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

func (d *TmuxDispatcher) Name() string { return "tmux" }

func (d *TmuxDispatcher) Dispatch(ctx context.Context, n *Notification) error {
	msg := n.Title
	if n.Body != "" {
		msg = n.Title + ": " + n.Body
	}
	// Truncate long messages for tmux display.
	if len(msg) > 200 {
		msg = msg[:197] + "..."
	}
	// Escape single quotes for shell safety.
	msg = strings.ReplaceAll(msg, "'", "'\\''")

	args := []string{"display-message"}
	if d.Target != "" {
		args = append(args, "-t", d.Target)
	}
	args = append(args, msg)

	cmd := exec.CommandContext(ctx, "tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux display-message: %w: %s", err, out)
	}
	return nil
}

func (d *TmuxDispatcher) Close() error { return nil }
