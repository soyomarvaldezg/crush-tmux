package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const switcherScriptContent = `#!/usr/bin/env bash
# crush-switcher.sh — fzf popup: jump to any live crush instance.
# Rows come from the crush-tmux Go binary (read-only DB watermarking).
set -u
BIN="${CRUSH_TMUX_BIN:-%s}"
FZF="${FZF_BIN:-fzf}"

rows="$("$BIN" switch 2>/dev/null)"
[ -n "$rows" ] || { tmux display-message "crush: no instances"; exit 0; }

sel="$(printf '%%s\n' "$rows" | "$FZF" \
    --ansi --delimiter='\t' --with-nth=2.. --prompt='crush ▸ ' \
    --preview 'tmux capture-pane -pe -J -t {1}' --preview-window=right:60%%)"
[ -n "$sel" ] || exit 0

pane="$(printf '%%s' "$sel" | cut -f1)"
[ -n "$pane" ] || exit 0

sess="$(tmux display-message -p -t "$pane" '#{session_name}' 2>/dev/null)"
tmux switch-client -t "$sess" \; select-window -t "$pane" \; select-pane -t "$pane"
`

const tmuxConfigTemplate = `# crush-tmux — add this line to your tmux.conf:
#   run-shell "tmux source-file $(crush-tmux config)"

set -g focus-events on
set -g status-right "#(%s status)"
set -g status-right-length 120
set -g status-interval 2

set-hook -g after-select-pane      'run-shell -b "%s mark-viewed"'
set-hook -g after-select-window    'run-shell -b "%s mark-viewed"'
set-hook -g client-session-changed 'run-shell -b "%s mark-viewed"'
set-hook -g client-focus-in        'run-shell -b "%s mark-viewed"'

bind-key a display-popup -E -w 80%% -h 60%% "%s"
`

func configCmd() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "crush-tmux config: cannot determine binary path: %v\n", err)
		os.Exit(1)
	}

	switcherDir := filepath.Join(homeDir(), ".config", "tmux", "crush")
	switcherPath := filepath.Join(switcherDir, "crush-switcher.sh")

	if err := ensureSwitcherScript(switcherPath); err != nil {
		fmt.Fprintf(os.Stderr, "crush-tmux config: failed to install switcher script: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(tmuxConfigTemplate, exe, exe, exe, exe, exe, switcherPath)
}

func ensureSwitcherScript(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determining binary path: %w", err)
	}

	content := fmt.Sprintf(switcherScriptContent, shellQuote(exe))
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return fmt.Errorf("writing switcher script: %w", err)
	}

	return nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	dir, _ := os.UserHomeDir()
	return dir
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
