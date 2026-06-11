// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a_test

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/a2a"
)

// TestShutdown_BeforeStart verifies that calling Shutdown on a server that
// was never Started returns promptly (closing the listener) instead of
// blocking forever on serveCh — nothing will ever write to it because the
// serve goroutine was never launched.
func TestShutdown_BeforeStart(t *testing.T) {
	t.Parallel()

	card := a2a.SessionCard{
		SessionID:   "test-" + t.Name(),
		Description: "shutdown-before-start",
	}.Build()
	srv, err := a2a.NewServer(context.Background(), a2a.ServerConfig{
		Card:    card,
		Handler: a2a.HandlerFunc(func(ctx context.Context, _ string, io a2a.SessionIO) error { return nil }),
		Addr:    "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- srv.Shutdown(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown before Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown before Start blocked instead of returning promptly")
	}
}

// TestShutdown_AfterStart confirms the normal path still works.
func TestShutdown_AfterStart(t *testing.T) {
	t.Parallel()

	card := a2a.SessionCard{
		SessionID:   "test-" + t.Name(),
		Description: "shutdown-after-start",
	}.Build()
	srv, err := a2a.NewServer(context.Background(), a2a.ServerConfig{
		Card:    card,
		Handler: a2a.HandlerFunc(func(ctx context.Context, _ string, io a2a.SessionIO) error { return nil }),
		Addr:    "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown after Start: %v", err)
	}
}
