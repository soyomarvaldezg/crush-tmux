# crush-tmux

A single Go binary that surfaces every live [Crush](https://github.com/charmbracelet/crush)
CLI instance in your tmux status bar, with an fzf switcher to jump to the one that
needs you. Built for a dotbar-themed tmux: it owns only the (otherwise empty)
`status-right` and never touches the theme.

## Why

Multi-agent attention management tools (agent-deck, ccmux, tmux-agent-status,
tmux-agent-indicator) are all driven by Claude Code / Codex lifecycle hooks
(`Stop`, `Notification`, `PermissionRequest`). Crush v0.88 exposes only
`PreToolUse` hooks, so it cannot feed them — they would show Crush as forever
"running". agent-deck additionally runs agents in an isolated `tmux -L
agent-deck` server, which stomps a themed status bar in your default server.

This instead reads what Crush actually is:

1. **Which instances exist** — process discovery: map each `crush` process TTY
   to a tmux pane (`ps` + `tmux list-panes`).
2. **How far along it is** — **read-only DB watermarking** against the
   per-project `<cwd>/.crush/crush.db`. Watermark = `MAX(messages.created_at)`.
   `wm > stored` → working; `wm == stored` → done/seen.
3. **Is it blocked** — a footer scan for the `Allow/Deny` permission barrier,
   the one thing the DB cannot see. This is the *only* footer dependency.

## States (attention-only)

The bar shows only what needs you. When nothing does, it reads `· crush idle`.

| glyph | color | state | meaning |
|---|---|---|---|
| `●` N | blue | working | N agents mid-task (watermark advancing) |
| `` name | orange | blocked | waiting on your Allow/Deny |
| `` name | green | done | finished, you haven't looked yet |

In the switcher, seen instances show `○` (grey). `done` collapses to seen the
moment you focus its pane.

## Read-only guarantee

The DB is opened with `file:<path>?mode=ro&_query_only=true` (see `db.go`).
`mode=ro` makes writes fail at the driver; `_query_only=true` makes SQLite
itself refuse any write or state-changing pragma. The binary therefore
**cannot** write to a crush database — read-only is enforced at open, not by
convention. Readers never block Crush's writer, so there is no concurrency or
latency impact.

## Install

```sh
go build -o crush-tmux .
```

Then in `~/.config/tmux/tmux.conf` (after TPM + theme overrides):

```tmux
set -g focus-events on
set -g status-right "#(/path/to/crush-tmux status)"
set -g status-right-length 120
set -g status-interval 5

set-hook -g after-select-pane      'run-shell -b "/path/to/crush-tmux mark-viewed"'
set-hook -g after-select-window    'run-shell -b "/path/to/crush-tmux mark-viewed"'
set-hook -g client-session-changed 'run-shell -b "/path/to/crush-tmux mark-viewed"'
set-hook -g client-focus-in        'run-shell -b "/path/to/crush-tmux mark-viewed"'

bind-key a display-popup -E -w 80% -h 60% "/path/to/crush-switcher.sh"
```

## Subcommands

- `crush-tmux status` — render the status-right segment (called by tmux every
  `status-interval`).
- `crush-tmux switch` — print fzf rows (`paneID<TAB>glyph label loc project`);
  consumed by `crush-switcher.sh`.
- `crush-tmux mark-viewed` — focus hook; collapses done→seen for the focused
  session.

State persists in `~/.config/tmux/crush/crush-tmux-state.json` so the done/seen
distinction survives across ticks.

## Maintenance

- **tmux updates**: safe. Uses only stable, long-standing tmux surface
  (`list-panes`, `capture-pane`, `display-message`, `set-hook`, `status-right`).
- **Crush updates**: the only fragile point is the footer barrier strings in
  `permissionBarrier()` (`state.go`). If a Crush release rewords the Allow/Deny
  prompt, update that one function. The done/working distinction is watermark
  based and does not depend on footer wording.
