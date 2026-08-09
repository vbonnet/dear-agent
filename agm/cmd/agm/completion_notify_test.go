package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/pkg/notify"
)

func TestNotifySessionCompletionUsesNeutralDefaultLogDispatcher(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	cfg := &config.Config{
		Notify: config.NotifyConfig{
			Enabled:   true,
			Recipient: "the operator",
			Dispatchers: []notify.DispatcherConfig{
				{Type: "log"},
			},
		},
	}

	notifySessionCompletion(context.Background(), cfg, completionNotification{
		SessionID: "session-1",
		Name:      "worker-1",
		Harness:   "codex-cli",
		Model:     "gpt-5",
		Outcome:   "completed",
		Source:    "test",
	})

	got := buf.String()
	for _, want := range []string{
		"AGM session completed",
		"worker-1 finished with outcome completed",
		"agm.session.complete",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("logged notification missing %q:\n%s", want, got)
		}
	}
}

func TestNotifySessionCompletionGracefulWhenDisabled(t *testing.T) {
	notifySessionCompletion(context.Background(), &config.Config{
		Notify: config.NotifyConfig{Enabled: false},
	}, completionNotification{SessionID: "session-1", Name: "worker-1"})
}
