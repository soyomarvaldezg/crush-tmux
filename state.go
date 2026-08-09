package main

// State machine: classify each live crush pane using its project DB watermark
// (primary) plus the viewed flag. Footer scanning is intentionally NOT the
// primary signal; it only distinguishes "blocked on a permission barrier",
// which the DB cannot see.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type State string

const (
	StateWorking State = "working" // watermark still advancing / mid-turn
	StateDone    State = "done"    // watermark stable, not yet viewed
	StateSeen    State = "seen"    // done and already looked at
	StateBlocked State = "blocked" // Allow/Deny barrier up (footer signal)
	StateHidden  State = "hidden"  // footer unreadable (alt-screen app)
)

// laneState is the persisted per-label record.
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

// permissionBarrier reports whether the pane footer shows an Allow/Deny prompt.
func permissionBarrier(paneID string) bool {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-J", "-t", paneID).Output()
	if err != nil {
		return false
	}
	tail := lastLines(string(out), 8)
	return strings.Contains(tail, "Allow for Session") ||
		(strings.Contains(tail, "Allow") && strings.Contains(tail, "Deny") && strings.Contains(tail, "choose"))
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// classify updates the store lane for p and returns its current State.
func (s *store) classify(p Pane) State {
	lane := s.Lanes[p.Session]
	if lane == nil {
		lane = &laneState{Watermark: -1}
		s.Lanes[p.Session] = lane
	}

	// Permission barrier overrides everything (attention you must give now).
	if permissionBarrier(p.ID) {
		lane.Viewed = false
		return StateBlocked
	}

	wm, err := latestWatermark(p.Cwd)
	if err != nil {
		wm = lane.Watermark // DB hiccup: keep last known
	}

	switch {
	case lane.Watermark < 0:
		// First sighting: seed at current watermark, treat as working.
		lane.Watermark = wm
		lane.Viewed = false
		return StateWorking
	case wm > lane.Watermark:
		// Activity: the agent produced new messages.
		lane.Watermark = wm
		lane.Viewed = false
		return StateWorking
	case lane.Viewed:
		// Stable watermark, already looked: seen.
		return StateSeen
	default:
		// Stable watermark, not yet looked: done, needs your cursor.
		return StateDone
	}
}

// markViewed collapses done->seen for the focused pane's session.
func (s *store) markViewed(session string) {
	if lane, ok := s.Lanes[session]; ok {
		lane.Viewed = true
	}
}
