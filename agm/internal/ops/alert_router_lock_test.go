package ops

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// A peer holding the queue lock must not be able to stall an alert
// indefinitely. Routing waits, then proceeds unserialized: losing dedupe is
// a far better failure than losing the alert, and an alert that blocks
// forever behind a wedged process is exactly the notification loss this
// router exists to prevent.
func TestRouteProceedsWhenTheQueueLockIsHeld(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "alerts.jsonl")

	// Hold the lock from a separate file descriptor, which is what a peer
	// process would do, and never release it during the test.
	held, err := lock.New(queue + ".lock")
	if err != nil {
		t.Fatalf("lock.New() error = %v", err)
	}
	if err := held.TryLock(); err != nil {
		t.Fatalf("TryLock() error = %v", err)
	}
	defer func() { _ = held.Unlock() }()

	router := NewAlertRouter(&OpContext{Storage: &mockStorage{sessions: []*manifest.Manifest{
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	}}})
	router.SetQueuePath(queue)
	router.lockTimeout = 100 * time.Millisecond
	delivered := false
	router.sendMessage = func(_ context.Context, _, _ string) error {
		delivered = true
		return nil
	}

	done := make(chan AlertRecord, 1)
	go func() {
		rec, routeErr := router.Route(context.Background(), AlertRequest{
			Kind: "checker", Source: "auth-checker", Title: "Auth at risk", Subject: "auth",
			Severity: AlertSeverityCritical, Actionability: AlertAgentActionable,
			OccurredAt: time.Now(),
		})
		if routeErr != nil {
			t.Errorf("Route() error = %v", routeErr)
		}
		done <- rec
	}()

	select {
	case rec := <-done:
		if rec.Status != AlertStatusDispatched {
			t.Fatalf("Status = %q, want dispatched despite the contended lock", rec.Status)
		}
		if !delivered {
			t.Fatal("alert was not delivered")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Route blocked indefinitely on a lock held by a peer")
	}
}
