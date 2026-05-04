package notify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// DesktopDispatcher sends native desktop notifications.
// On Linux it uses notify-send; on macOS it uses osascript.
type DesktopDispatcher struct{}

// NewDesktopDispatcher creates a dispatcher for native desktop notifications.
func NewDesktopDispatcher() *DesktopDispatcher {
	return &DesktopDispatcher{}
}

// Name returns the dispatcher name "desktop".
func (d *DesktopDispatcher) Name() string { return "desktop" }

// Dispatch shows the notification via the platform's native desktop notifier.
func (d *DesktopDispatcher) Dispatch(ctx context.Context, n *Notification) error {
	switch runtime.GOOS {
	case "linux":
		return d.notifySend(ctx, n)
	case "darwin":
		return d.osascript(ctx, n)
	default:
		return fmt.Errorf("desktop notifications not supported on %s", runtime.GOOS)
	}
}

func (d *DesktopDispatcher) notifySend(ctx context.Context, n *Notification) error {
	args := []string{n.Title}
	if n.Body != "" {
		args = append(args, n.Body)
	}
	cmd := exec.CommandContext(ctx, "notify-send", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notify-send: %w: %s", err, out)
	}
	return nil
}

func (d *DesktopDispatcher) osascript(ctx context.Context, n *Notification) error {
	script := fmt.Sprintf(`display notification %q with title %q`, n.Body, n.Title)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %w: %s", err, out)
	}
	return nil
}

// Close is a no-op for DesktopDispatcher.
func (d *DesktopDispatcher) Close() error { return nil }
