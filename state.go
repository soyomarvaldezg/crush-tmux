package main

// State machine. Signals, in priority order:
//   1. footer "Allow/Deny" / "Requesting permission..." -> blocked
//   2. footer spinner line ("> " prefix + "..." suffix) -> working
//   3. watermark moved + idle footer  -> done (finished, unseen)
//      watermark moved + ambiguous footer -> working
//   4. watermark stable -> done if unseen, seen if viewed
//
// The footer tells us busy-vs-idle (watermark alone cannot: a working agent
// pauses between tool calls and looks identical to a done one). The watermark
// is only used to (a) notice activity and (b) reset the viewed flag when new
// output arrives after you've looked.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type State string

const (
	StateWorking State = "working" // a turn is actively running
	StateDone    State = "done"    // finished, you haven't looked yet
	StateSeen    State = "seen"    // finished and you already looked
	StateBlocked State = "blocked" // Allow/Deny barrier is up
)

// laneState is the persisted per-lane record. Lanes are keyed by project path
// (not tmux session name) so several panes in one tmux session that work on
// different projects don't collide on a single watermark.
type laneState struct {
	Watermark int64 `json:"watermark"`
	Viewed    bool  `json:"viewed"`
}

type store struct {
	path  string
	Lanes map[string]*laneState `json:"lanes"`
}

func storePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tmux", "crush", "crush-tmux-state.json")
}

func loadStore() *store {
	s := &store{path: storePath(), Lanes: map[string]*laneState{}}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, s)
	}
	if s.Lanes == nil {
		s.Lanes = map[string]*laneState{}
	}
	return s
}

func (s *store) save() {
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err == nil {
		_ = os.Rename(tmp, s.path)
	}
}

// scanFooter captures the pane's last non-empty lines once.
func scanFooter(paneID string) string {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-J", "-t", paneID).Output()
	if err != nil {
		return ""
	}
	return lastLines(string(out), 15)
}

func isBlocked(footer string) bool {
	return strings.Contains(footer, "Allow for Session") ||
		(strings.Contains(footer, "Allow") && strings.Contains(footer, "Deny") && strings.Contains(footer, "choose")) ||
		strings.Contains(footer, "Requesting permission...")
}

func isWorkingFooter(footer string) bool {
	// Only the animated spinner line ("> Brrrrr...", "> Working!", etc.) is a
	// reliable working signal. "esc cancel" and "Waiting for tool response" are
	// always present in the idle/footer area or linger in scrollback, so they
	// would misclassify idle and blocked panes as working.
	for _, line := range strings.Split(footer, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "> ") && strings.HasSuffix(trimmed, "...") {
			return true
		}
		if strings.Contains(trimmed, "> Working!") {
			return true
		}
	}
	return false
}

func isIdleFooter(footer string) bool {
	return strings.Contains(footer, "tab focus chat") ||
		strings.Contains(footer, "tab focus editor")
}

func lastLines(s string, n int) string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var nonEmpty []string
	for _, l := range all {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}
	if len(nonEmpty) > n {
		nonEmpty = nonEmpty[len(nonEmpty)-n:]
	}
	return strings.Join(nonEmpty, "\n")
}

// classify updates the lane for p and returns its current State.
func (s *store) classify(p Pane) State {
	lane := s.Lanes[p.Cwd]
	if lane == nil {
		lane = &laneState{Watermark: -1}
		s.Lanes[p.Cwd] = lane
	}

	footer := scanFooter(p.ID)

	// 1. Blocked overrides everything.
	if isBlocked(footer) {
		lane.Viewed = false
		return StateBlocked
	}

	// 2. Footer explicitly busy.
	if isWorkingFooter(footer) {
		lane.Viewed = false
		if wm, err := latestWatermark(p.Cwd); err == nil && wm > lane.Watermark {
			lane.Watermark = wm
		}
		return StateWorking
	}

	// 3 & 4. Footer idle (or unreadable): use the watermark for done/seen.
	// If the footer is clearly idle, trust it over the watermark — a higher
	// watermark just means output arrived earlier, not that the agent is busy now.
	wm, err := latestWatermark(p.Cwd)
	if err != nil {
		wm = lane.Watermark // DB hiccup: keep last known
	}
	if lane.Watermark < 0 {
		lane.Watermark = wm
		lane.Viewed = true
		return StateSeen
	}
	if wm > lane.Watermark {
		lane.Watermark = wm
		if isIdleFooter(footer) {
			// Watermark moved but footer says idle: agent finished, mark done/unseen.
			lane.Viewed = false
			return StateDone
		}
		// Footer ambiguous (not idle, not working, not blocked): treat as working.
		lane.Viewed = false
		return StateWorking
	}
	if lane.Viewed {
		return StateSeen
	}
	return StateDone
}

// markViewed collapses done->seen for the focused pane's project lane.
func (s *store) markViewed(cwd string) {
	if lane, ok := s.Lanes[cwd]; ok {
		lane.Viewed = true
	}
}
