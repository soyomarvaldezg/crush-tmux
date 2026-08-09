package main

// Attention-only Tokyo Night rendering. Only states that need you are shown:
// working (blue), blocked (orange), done (green). Seen/hidden stay out of the
// bar so it reads clean when nothing needs attention.

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const (
	colWorking = "#[fg=colour39]"  // blue
	colBlocked = "#[fg=colour214]" // orange
	colDone    = "#[fg=colour114]" // green
	colReset   = "#[default]"
)

const (
	glyphWorking = "●"          // U+25CF e2 97 8f
	glyphBlocked = "\U0000f256" // hand  ef 89 96
	glyphDone    = "\U0000f00c" // check ef 80 8c
	glyphSeen    = "○"          // U+25CB e2 97 8b
)

// renderStatus prints the status-right segment.
func renderStatus() {
	st := loadStore()
	panes, err := discoverPanes()
	if err != nil || len(panes) == 0 {
		fmt.Print("#[fg=colour240]· crush idle#[default]")
		return
	}

	working := 0
	blocked := map[string]bool{}
	done := map[string]bool{}
	changed := false

	for _, p := range panes {
		before := st.Lanes[p.Session]
		state := st.classify(p)
		after := st.Lanes[p.Session]
		if before == nil || *before != *after {
			changed = true
		}
		switch state {
		case StateWorking:
			working++
		case StateBlocked:
			blocked[p.Session] = true
		case StateDone:
			done[p.Session] = true
		}
	}
	if changed {
		st.save()
	}

	var b strings.Builder
	if working > 0 {
		fmt.Fprintf(&b, "%s%s%d%s ", colWorking, glyphWorking, working, colReset)
	}
	for _, name := range sortedKeys(blocked) {
		fmt.Fprintf(&b, "%s%s%s%s ", colBlocked, glyphBlocked, name, colReset)
	}
	for _, name := range sortedKeys(done) {
		fmt.Fprintf(&b, "%s%s%s%s ", colDone, glyphDone, name, colReset)
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		// Nothing needs attention: bar stays clean.
		fmt.Print("#[fg=colour240]· crush idle#[default]")
		return
	}
	fmt.Print(out)
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// focusedSession returns the tmux session of the truly-focused pane.
func focusedSession() (string, error) {
	out, err := exec.Command("tmux", "list-panes", "-a",
		"-f", "#{&&:#{pane_active},#{&&:#{window_active},#{session_attached}}}",
		"-F", "#{session_name}").Output()
	if err != nil {
		return "", err
	}
	line := strings.Split(strings.TrimRight(string(out), "\n"), "\n")[0]
	return strings.TrimSpace(line), nil
}

// markViewedCmd collapses done->seen for the focused session.
func markViewedCmd() {
	sess, err := focusedSession()
	if err != nil || sess == "" {
		return
	}
	st := loadStore()
	st.markViewed(sess)
	st.save()
}

// renderSwitch prints fzf rows: paneID<TAB>glyph label location project.
func renderSwitch() {
	st := loadStore()
	panes, err := discoverPanes()
	if err != nil || len(panes) == 0 {
		return
	}
	changed := false
	for _, p := range panes {
		before := st.Lanes[p.Session]
		state := st.classify(p)
		after := st.Lanes[p.Session]
		if before == nil || *before != *after {
			changed = true
		}
		var glyph, color string
		switch state {
		case StateWorking:
			glyph, color = glyphWorking, colWorking
		case StateBlocked:
			glyph, color = glyphBlocked, colBlocked
		case StateDone:
			glyph, color = glyphDone, colDone
		case StateSeen:
			glyph, color = glyphSeen, "#[fg=colour244]"
		default:
			glyph, color = "?", "#[fg=colour244]"
		}
		proj := baseName(p.Cwd)
		loc := windowLoc(p.ID)
		fmt.Printf("%s\t%s%s%s %s  %s  %s\n", p.ID, color, glyph, colReset, p.Session, loc, proj)
	}
	if changed {
		st.save()
	}
}

func windowLoc(paneID string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{window_index}.#{pane_index}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func main() {
	cmd := "status"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "status":
		renderStatus()
	case "switch":
		renderSwitch()
	case "mark-viewed":
		markViewedCmd()
	default:
		fmt.Fprintf(os.Stderr, "usage: crush-tmux [status|switch|mark-viewed]\n")
		os.Exit(2)
	}
}
