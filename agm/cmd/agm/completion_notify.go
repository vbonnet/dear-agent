package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/pkg/notify"
)

const completionNotifyTimeout = 5 * time.Second

type completionNotification struct {
	SessionID string
	Name      string
	Harness   string
	Model     string
	Outcome   string
	Source    string
}

func notifySessionCompletion(ctx context.Context, cfg *config.Config, c completionNotification) {
	if cfg == nil || !cfg.Notify.Enabled {
		return
	}
	dispatchers, err := notify.BuildDispatchers(&notify.Config{Dispatchers: cfg.Notify.Dispatchers}, slog.Default())
	if err != nil {
		slog.Default().Warn("completion notification dispatchers unavailable", "error", err)
		return
	}
	if len(dispatchers) == 0 {
		dispatchers = append(dispatchers, notify.NewLogDispatcher(slog.Default()))
	}
	defer func() {
		for _, d := range dispatchers {
			if err := d.Close(); err != nil {
				slog.Default().Warn("completion notification dispatcher close failed", "dispatcher", d.Name(), "error", err)
			}
		}
	}()

	recipient := cfg.Notify.Recipient
	if recipient == "" {
		recipient = "the operator"
	}
	outcome := c.Outcome
	if outcome == "" {
		outcome = string(manifest.OutcomeCompleted)
	}

	n := &notify.Notification{
		ID:        fmt.Sprintf("agm-session-complete-%s-%d", c.SessionID, time.Now().UnixNano()),
		Title:     "AGM session completed",
		Body:      fmt.Sprintf("%s finished with outcome %s", c.Name, outcome),
		Level:     slog.LevelInfo,
		Source:    "agm.session.complete",
		Timestamp: time.Now(),
		Meta: map[string]any{
			"recipient":  recipient,
			"session_id": c.SessionID,
			"name":       c.Name,
			"harness":    c.Harness,
			"model":      c.Model,
			"outcome":    outcome,
			"source":     c.Source,
		},
	}

	notifyCtx, cancel := context.WithTimeout(ctx, completionNotifyTimeout)
	defer cancel()
	for _, d := range dispatchers {
		if err := d.Dispatch(notifyCtx, n); err != nil {
			slog.Default().Warn("completion notification dispatch failed",
				"dispatcher", d.Name(),
				"session_id", c.SessionID,
				"error", err,
			)
		}
	}
}
