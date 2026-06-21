package supervisor

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// findInboxAlert returns the most recent overseer.session.inbox_alert record in
// the trail buffer, or nil if none was emitted.
func findInboxAlert(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "overseer.session.inbox_alert" {
			rec = r
		}
	}
	return rec
}

func newInboxOverseer(t *testing.T, counter SessionInboxCounter, threshold int) (*Overseer, *bytes.Buffer) {
	t.Helper()
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.3})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	o.WithSessionInboxAlert(counter, threshold)
	return o, buf
}

func TestCheckSessionInboxHealth_BelowThreshold(t *testing.T) {
	counter := NewInMemorySessionInboxCounter()
	counter.SetResult(10, nil)
	o, buf := newInboxOverseer(t, counter, 15)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if counter.Calls() != 1 {
		t.Fatalf("InboxCount called %d times, want 1", counter.Calls())
	}
	if rec := findInboxAlert(t, buf); rec != nil {
		t.Fatalf("below threshold should not alert, got %v", rec)
	}
}

func TestCheckSessionInboxHealth_AtThreshold(t *testing.T) {
	// At exactly the threshold the inbox is still considered healthy — the
	// alert fires only when the count strictly exceeds the threshold.
	counter := NewInMemorySessionInboxCounter()
	counter.SetResult(15, nil)
	o, buf := newInboxOverseer(t, counter, 15)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if rec := findInboxAlert(t, buf); rec != nil {
		t.Fatalf("at threshold should not alert, got %v", rec)
	}
}

func TestCheckSessionInboxHealth_AboveThreshold(t *testing.T) {
	counter := NewInMemorySessionInboxCounter()
	counter.SetResult(20, nil)
	o, buf := newInboxOverseer(t, counter, 15)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	rec := findInboxAlert(t, buf)
	if rec == nil {
		t.Fatal("above threshold should emit an inbox_alert record")
	}
	p := rec["payload"].(map[string]any)
	if int(p["count"].(float64)) != 20 {
		t.Errorf("count = %v, want 20", p["count"])
	}
	if int(p["threshold"].(float64)) != 15 {
		t.Errorf("threshold = %v, want 15", p["threshold"])
	}
	if _, ok := p["alert"].(string); !ok {
		t.Errorf("alert message missing or not a string: %v", p["alert"])
	}
	if _, hasErr := p["error"]; hasErr {
		t.Errorf("unexpected error in payload: %v", p["error"])
	}
}

func TestCheckSessionInboxHealth_DefaultThreshold(t *testing.T) {
	// A zero threshold falls back to defaultSessionInboxAlertThreshold (15).
	counter := NewInMemorySessionInboxCounter()
	counter.SetResult(defaultSessionInboxAlertThreshold+1, nil)
	o, buf := newInboxOverseer(t, counter, 0)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	rec := findInboxAlert(t, buf)
	if rec == nil {
		t.Fatal("count above default threshold should alert")
	}
	p := rec["payload"].(map[string]any)
	if int(p["threshold"].(float64)) != defaultSessionInboxAlertThreshold {
		t.Errorf("threshold = %v, want %d", p["threshold"], defaultSessionInboxAlertThreshold)
	}
}

func TestCheckSessionInboxHealth_CounterError(t *testing.T) {
	// A counter error is recorded so the failure is visible, but must not fail
	// the tick.
	counter := NewInMemorySessionInboxCounter()
	counter.SetResult(0, errors.New("agm not found"))
	o, buf := newInboxOverseer(t, counter, 15)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick must not fail on counter error: %v", err)
	}
	rec := findInboxAlert(t, buf)
	if rec == nil {
		t.Fatal("counter error should emit an inbox_alert record")
	}
	p := rec["payload"].(map[string]any)
	if p["error"] != "agm not found" {
		t.Errorf("error = %q, want %q", p["error"], "agm not found")
	}
}

func TestOverseer_NoInboxCounter_NoAlert(t *testing.T) {
	// With no counter wired the Overseer never alerts on inbox depth.
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.3})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if rec := findInboxAlert(t, buf); rec != nil {
		t.Fatalf("no counter wired should not alert, got %v", rec)
	}
}

// TestAGMSessionInboxCounter_ExcludesArchived verifies the production counter
// counts non-archived sessions and excludes archived ones from the --all set.
func TestAGMSessionInboxCounter_ExcludesArchived(t *testing.T) {
	const listJSON = `{
		"operation": "list_sessions",
		"sessions": [
			{"id": "1", "name": "a", "status": "active"},
			{"id": "2", "name": "b", "status": "stopped"},
			{"id": "3", "name": "c", "status": "stopped"},
			{"id": "4", "name": "d", "status": "archived"},
			{"id": "5", "name": "e", "status": "archived"}
		],
		"total": 5
	}`

	var gotArgs []string
	counter := &AGMSessionInboxCounter{
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			gotArgs = args
			return []byte(listJSON), nil
		},
	}

	count, err := counter.InboxCount(context.Background())
	if err != nil {
		t.Fatalf("InboxCount: %v", err)
	}
	// 1 active + 2 stopped = 3 non-archived; the 2 archived are excluded.
	if count != 3 {
		t.Errorf("count = %d, want 3 (active+stopped, excluding archived)", count)
	}

	// Must query the --all set, else stopped sessions are hidden.
	wantAll := false
	for _, a := range gotArgs {
		if a == "--all" {
			wantAll = true
		}
	}
	if !wantAll {
		t.Errorf("args = %v, want --all to include stopped sessions", gotArgs)
	}
}

func TestAGMSessionInboxCounter_EmptyInbox(t *testing.T) {
	counter := &AGMSessionInboxCounter{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"operation":"list_sessions","sessions":[],"total":0}`), nil
		},
	}
	count, err := counter.InboxCount(context.Background())
	if err != nil {
		t.Fatalf("InboxCount: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestAGMSessionInboxCounter_UnparseableOutput(t *testing.T) {
	counter := &AGMSessionInboxCounter{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("not json"), nil
		},
	}
	if _, err := counter.InboxCount(context.Background()); err == nil {
		t.Fatal("expected error on unparseable output")
	}
}

func TestAGMSessionInboxCounter_RunError(t *testing.T) {
	counter := &AGMSessionInboxCounter{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, errors.New("exec: \"agm\": not found")
		},
	}
	if _, err := counter.InboxCount(context.Background()); err == nil {
		t.Fatal("expected error when the command fails")
	}
}

func TestAGMSessionInboxCounter_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	counter := &AGMSessionInboxCounter{
		Timeout: time.Second,
		run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, ctx.Err()
		},
	}
	if _, err := counter.InboxCount(ctx); err == nil {
		t.Fatal("expected error when the context is cancelled")
	}
}
