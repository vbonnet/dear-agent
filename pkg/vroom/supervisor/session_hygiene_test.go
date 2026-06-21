package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOverseer_Tick_SessionHygiene_EmitsTrail(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.3})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	gardener := NewInMemorySessionGardener()
	gardener.SetResult(SessionGCStats{Scanned: 12, Archived: 3, Skipped: 9}, nil)
	o.WithSessionHygiene(gardener, 30*time.Second)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if gardener.Calls() != 1 {
		t.Fatalf("GC called %d times, want 1", gardener.Calls())
	}

	var rec map[string]any
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "overseer.session.hygiene" {
			rec = r
		}
	}
	if rec == nil {
		t.Fatal("no overseer.session.hygiene trail record")
	}
	p := rec["payload"].(map[string]any)
	if int(p["scanned"].(float64)) != 12 {
		t.Errorf("scanned = %v, want 12", p["scanned"])
	}
	if int(p["archived"].(float64)) != 3 {
		t.Errorf("archived = %v, want 3", p["archived"])
	}
	if int(p["skipped"].(float64)) != 9 {
		t.Errorf("skipped = %v, want 9", p["skipped"])
	}
	if p["note"] != "archived 3 sessions" {
		t.Errorf("note = %q, want %q", p["note"], "archived 3 sessions")
	}
	if _, hasErr := p["error"]; hasErr {
		t.Errorf("unexpected error in payload: %v", p["error"])
	}
}

func TestOverseer_Tick_SessionHygiene_RateLimited(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.3})
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	gardener := NewInMemorySessionGardener()
	gardener.SetResult(SessionGCStats{Scanned: 5, Archived: 1}, nil)
	o.WithSessionHygiene(gardener, 30*time.Second)

	// Drive a fake clock so rate-limiting is deterministic.
	now := time.Unix(1_700_000_000, 0)
	o.clock = func() time.Time { return now }

	// First tick runs the pass.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	if gardener.Calls() != 1 {
		t.Fatalf("after tick 1: GC called %d times, want 1", gardener.Calls())
	}

	// Second tick 10s later is within the 30s interval — skipped.
	now = now.Add(10 * time.Second)
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	if gardener.Calls() != 1 {
		t.Fatalf("after tick 2 (rate-limited): GC called %d times, want 1", gardener.Calls())
	}

	// Third tick past the interval — runs again.
	now = now.Add(25 * time.Second)
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 3: %v", err)
	}
	if gardener.Calls() != 2 {
		t.Fatalf("after tick 3 (interval elapsed): GC called %d times, want 2", gardener.Calls())
	}
}

func TestOverseer_Tick_SessionHygiene_ErrorRecordedNotFatal(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.3})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	gardener := NewInMemorySessionGardener()
	gardener.SetResult(SessionGCStats{}, errors.New("gc boom"))
	o.WithSessionHygiene(gardener, 30*time.Second)

	// A gardener failure must not fail the tick.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick should not fail on gardener error: %v", err)
	}

	var rec map[string]any
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "overseer.session.hygiene" {
			rec = r
		}
	}
	if rec == nil {
		t.Fatal("no overseer.session.hygiene trail record")
	}
	p := rec["payload"].(map[string]any)
	if p["error"] != "gc boom" {
		t.Errorf("error = %q, want %q", p["error"], "gc boom")
	}
}

func TestOverseer_Tick_NoHygieneWhenUnwired(t *testing.T) {
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

	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "overseer.session.hygiene" {
			t.Errorf("hygiene record present with no gardener wired: %+v", r)
		}
	}
}

func TestWithSessionHygiene_DefaultsZeroInterval(t *testing.T) {
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, NewInMemoryResourceProbe(), EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	o.WithSessionHygiene(NewInMemorySessionGardener(), 0)
	if o.hygieneInterval != defaultHygieneInterval {
		t.Errorf("hygieneInterval = %v, want default %v", o.hygieneInterval, defaultHygieneInterval)
	}
}

func TestGCSessionGardener_ParsesJSON(t *testing.T) {
	var gotArgs []string
	g := &GCSessionGardener{
		OlderThan: 10 * time.Minute,
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			gotArgs = args
			return []byte(`{"operation":"session_gc","scanned":7,"archived":2,"skipped":5,"errors":0}`), nil
		},
	}

	stats, err := g.GC(context.Background())
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if stats.Scanned != 7 || stats.Archived != 2 || stats.Skipped != 5 || stats.Errors != 0 {
		t.Errorf("stats = %+v, want {7 2 5 0}", stats)
	}

	// Confirm the gc flags are passed through.
	joined := ""
	for _, a := range gotArgs {
		joined += a + " "
	}
	wantContains := []string{"gc", "--json", "--older-than", "10m0s"}
	for _, w := range wantContains {
		found := false
		for _, a := range gotArgs {
			if a == w {
				found = true
			}
		}
		if !found {
			t.Errorf("args %v missing %q", gotArgs, w)
		}
	}
}

func TestGCSessionGardener_UnparseableJSONErrors(t *testing.T) {
	g := &GCSessionGardener{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("not json"), nil
		},
	}
	if _, err := g.GC(context.Background()); err == nil {
		t.Error("expected error on unparseable JSON, got nil")
	}
}

func TestGCSessionGardener_PartialExitStillParses(t *testing.T) {
	// A non-zero exit that still emits valid JSON (some archives failed) must
	// yield the parsed counts, not an error.
	g := &GCSessionGardener{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"scanned":4,"archived":1,"skipped":2,"errors":1}`), errors.New("exit status 1")
		},
	}
	stats, err := g.GC(context.Background())
	if err != nil {
		t.Fatalf("GC should parse JSON despite non-zero exit: %v", err)
	}
	if stats.Archived != 1 || stats.Errors != 1 {
		t.Errorf("stats = %+v, want archived=1 errors=1", stats)
	}
}
