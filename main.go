package main

// Attention-only Tokyo Night rendering. The focused pane is excluded from the
// bar (you're already looking at it). All counts are collapsed: ● N, ✋ N, ✓ N.
// Max 3 segments, always fits.

import (
	"fmt"
	"os"
	"os/exec"
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

// Truecolor ANSI escapes for fzf (Tokyo Night Storm). tmux's #[...] syntax is
// meaningless to fzf, which expects real ESC[38;2;r;g;bm sequences.
const (
	ansiBlue   = "\x1b[38;2;122;162;247m" // #7aa2f7
	ansiOrange = "\x1b[38;2;224;175;104m" // #e0af68
	ansiGreen  = "\x1b[38;2;158;206;106m" // #9ece6a
	ansiGrey   = "\x1b[38;2;86;95;137m"   // #565f89
	ansiReset  = "\x1b[0m"
)

// renderStatus prints the status-right segment.
func renderStatus() {
	st := loadStore()
	panes, err := discoverPanes()
	if err != nil || len(panes) == 0 {
		fmt.Print("#[fg=colour240]· crush idle#[default]")
		return
	}

	focusedCwd, _ := focusedCwd()

	working := 0
	blocked := 0
	done := 0
	changed := false

	for _, p := range panes {
		if p.Cwd == focusedCwd {
			continue
		}
		before := st.Lanes[p.Cwd]
		state := st.classify(p)
		after := st.Lanes[p.Cwd]
		if before == nil || *before != *after {
			changed = true
		}
		switch state {
		case StateWorking:
			working++
		case StateBlocked:
			blocked++
		case StateDone:
			done++
		}
	}
	if changed {
		st.save()
	}

	var b strings.Builder
	if working > 0 {
		fmt.Fprintf(&b, "%s%s %d%s  ", colWorking, glyphWorking, working, colReset)
	}
	if blocked > 0 {
		fmt.Fprintf(&b, "%s%s %d%s  ", colBlocked, glyphBlocked, blocked, colReset)
	}
	if done > 0 {
		fmt.Fprintf(&b, "%s%s %d%s  ", colDone, glyphDone, done, colReset)
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		// Nothing needs attention: bar stays clean.
		fmt.Print("#[fg=colour240]· crush idle#[default]")
		return
	}
	fmt.Print(out)
}


// focusedCwd returns the working dir of the truly-focused pane.
func focusedCwd() (string, error) {
	out, err := exec.Command("tmux", "list-panes", "-a",
		"-f", "#{&&:#{pane_active},#{&&:#{window_active},#{session_attached}}}",
		"-F", "#{pane_current_path}").Output()
	if err != nil {
		return "", err
	}
	line := strings.Split(strings.TrimRight(string(out), "\n"), "\n")[0]
	return strings.TrimSpace(line), nil
}

// markViewedCmd collapses done->seen for the focused pane's project lane.
func markViewedCmd() {
	cwd, err := focusedCwd()
	if err != nil || cwd == "" {
		return
	}
	st := loadStore()
	st.markViewed(cwd)
	st.save()
}

// renderSwitch prints fzf rows: paneID<TAB>glyph label location project.
// renderSwitch prints fzf rows: paneID<TAB>glyph label location project.
// NOTE: fzf renders ANSI escape codes (real \x1b[... sequences), NOT tmux's
// #[...] syntax — that only works inside tmux's own status bar. So this uses
// truecolor ANSI escapes matched to the Tokyo Night palette.
func renderSwitch() {
	st := loadStore()
	panes, err := discoverPanes()
	if err != nil || len(panes) == 0 {
		return
	}
	changed := false
	for _, p := range panes {
		before := st.Lanes[p.Cwd]
		state := st.classify(p)
		after := st.Lanes[p.Cwd]
		if before == nil || *before != *after {
			changed = true
		}
		var glyph, ansi string
		switch state {
		case StateWorking:
			glyph, ansi = glyphWorking, ansiBlue
		case StateBlocked:
			glyph, ansi = glyphBlocked, ansiOrange
		case StateDone:
			glyph, ansi = glyphDone, ansiGreen
		case StateSeen:
			glyph, ansi = glyphSeen, ansiGrey
		default:
			glyph, ansi = "?", ansiGrey
		}
		proj := baseName(p.Cwd)
		loc := windowLoc(p.ID)
		fmt.Printf("%s\t%s%s%s %s  %s  %s\n", p.ID, ansi, glyph, ansiReset, p.Session, loc, proj)
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
	case "config":
		configCmd()
	default:
		fmt.Fprintf(os.Stderr, "usage: crush-tmux [status|switch|mark-viewed|config]\n")
		os.Exit(2)
	}
}
