package main

// Discovery: map live crush CLI processes to tmux panes via TTY.

import (
	"os/exec"
	"sort"
	"strings"
)

// Pane describes a tmux pane running a live crush process.
type Pane struct {
	ID      string // tmux pane id, e.g. %24
	Session string // tmux session name (the label)
	Cwd     string // pane_current_path
	TTY     string // pane tty
}

// liveCrushTTYs returns the set of TTYs (without /dev/) that have a live crush CLI.
func liveCrushTTYs() (map[string]bool, error) {
	out, err := exec.Command("ps", "-Ao", "tty,command").Output()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "??" {
			continue
		}
		cmd := fields[1]
		if i := strings.LastIndex(cmd, "/"); i >= 0 {
			cmd = cmd[i+1:]
		}
		if cmd == "crush" || strings.HasPrefix(cmd, "crush-") {
			set[fields[0]] = true
		}
	}
	return set, nil
}

// discoverPanes returns every tmux pane running a live crush process.
func discoverPanes() ([]Pane, error) {
	ttys, err := liveCrushTTYs()
	if err != nil {
		return nil, err
	}
	if len(ttys) == 0 {
		return nil, nil
	}
	out, err := exec.Command("tmux", "list-panes", "-a",
		"-F", "#{pane_id}\t#{session_name}\t#{pane_current_path}\t#{pane_tty}").Output()
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		tty := strings.TrimPrefix(f[3], "/dev/")
		if ttys[tty] {
			panes = append(panes, Pane{ID: f[0], Session: f[1], Cwd: f[2], TTY: tty})
		}
	}
	sort.Slice(panes, func(i, j int) bool { return panes[i].Session < panes[j].Session })
	return panes, nil
}
