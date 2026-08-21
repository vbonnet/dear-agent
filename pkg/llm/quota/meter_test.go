package quota_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
	"github.com/vbonnet/dear-agent/pkg/workflow/roles"
)

// stubReader returns a canned snapshot and counts reads.
type stubReader struct {
	mu       sync.Mutex
	snapshot *quota.Snapshot
	err      error
	reads    int
}

func (s *stubReader) Read(context.Context) (*quota.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return s.snapshot, s.err
}

func (s *stubReader) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

// liveMeter builds a meter over the real captured CodexBar fixture, so
// the ordering tests exercise the same data the host produces.
func liveMeter(t *testing.T, policy quota.Policy) *quota.Meter {
	t.Helper()
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	now := snapshot.GeneratedAt.Add(time.Second)
	meter := quota.New(quota.Options{
		Reader:          &stubReader{snapshot: snapshot},
		Policy:          policy,
		RefreshInterval: -1,
		Now:             func() time.Time { return now },
	})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return meter
}

// A nil meter is the disabled case, and it must be indistinguishable
// from not having the package wired in at all.
func TestNilMeterLeavesRoutingUntouched(t *testing.T) {
	var meter *quota.Meter
	if meter.Enabled() {
		t.Error("a nil meter must not report itself enabled")
	}
	if !meter.HasCapacity("claude-opus-4-8") {
		t.Error("a nil meter must report capacity")
	}
	configured := []string{"claude-opus-4-8", "gpt-5.5-pro", "gemini-3.1-pro"}
	got, decisions := meter.OrderModels(configured)
	assertOrder(t, got, configured)
	for _, d := range decisions {
		if d.Known() {
			t.Errorf("a nil meter produced a known decision: %+v", d)
		}
	}
}

func TestMeterWithoutReaderLeavesRoutingUntouched(t *testing.T) {
	meter := quota.New(quota.Options{})
	if meter.Enabled() {
		t.Error("a meter with no reader must not report itself enabled")
	}
	configured := []string{"gpt-5.5-pro", "claude-opus-4-8"}
	got, _ := meter.OrderModels(configured)
	assertOrder(t, got, configured)
}

func TestMeterOrdersRealReadingByHeadroomExcludingUnimplementedGemini(t *testing.T) {
	meter := liveMeter(t, quota.Policy{})

	// Live capture: openai/codex is 52% used (48% left, band 1);
	// gemini/antigravity has 99.8% left (band 0, the best reading in this
	// snapshot) but gemini has no constructible provider yet
	// (provider.Factory's newGeminiProvider is a stub for both auth
	// methods), so it stays pinned to its configured last slot instead of
	// being promoted ahead of openai (review on #1218); anthropic/claude
	// is unreadable (unknown, also keeps its configured slot). With
	// gemini excluded, openai is the only candidate quota can actually
	// move, and it has nothing left to move past — the order comes out
	// unchanged from roles.yaml.
	configured := []string{"claude-opus-4-8", "gpt-5.5-pro", "gemini-3.1-pro"}
	got, decisions := meter.OrderModels(configured)
	assertOrder(t, got, configured)
	if decisions[2].Known() {
		t.Errorf("gemini decision = %+v, want ClassUnknown (excluded from quota promotion)", decisions[2])
	}
}

func TestMeterKeepsConfiguredOrderWithinABand(t *testing.T) {
	meter := liveMeter(t, quota.Policy{})
	// Both are unknown/healthy at band 0, so roles.yaml decides.
	configured := []string{"gemini-3.1-pro", "claude-opus-4-8"}
	got, _ := meter.OrderModels(configured)
	assertOrder(t, got, configured)

	reversed := []string{"claude-opus-4-8", "gemini-3.1-pro"}
	got, _ = meter.OrderModels(reversed)
	assertOrder(t, got, reversed)
}

func TestMeterNeverDropsACandidate(t *testing.T) {
	snapshot := snapshotAt(time.Now(),
		readable("openai", window("Weekly", 0)),
		readable("anthropic", window("Weekly", 0)),
		readable("gemini", window("Weekly", 0)),
	)
	meter := newTestMeter(t, snapshot, quota.Policy{})
	configured := []string{"claude-opus-4-8", "gpt-5.5-pro", "gemini-3.1-pro"}
	got, _ := meter.OrderModels(configured)
	if len(got) != len(configured) {
		t.Fatalf("got %d candidates, want %d — exhaustion must not shorten the chain", len(got), len(configured))
	}
}

// TestMeterOrderModelsPinsUnknownDecisionsToTheirOriginalIndex guards a
// transitivity bug: an earlier comparator returned "equivalent" for any
// pair involving an unknown decision, which is not a valid strict weak
// order once two unknowns sit between known decisions of different bands
// (gemini review on #1325). OrderModels now pins every unknown decision
// to its original index and only reorders the known decisions among the
// remaining positions.
func TestMeterOrderModelsPinsUnknownDecisionsToTheirOriginalIndex(t *testing.T) {
	snapshot := snapshotAt(time.Now(),
		readable("anthropic", window("Weekly", 90)), // band 0, best
		readable("openai", window("Weekly", 5)),     // band 3, worst
	)
	meter := newTestMeter(t, snapshot, quota.Policy{})

	// gemini has no reading at all, so it decides unknown. Configured
	// between the best and worst known candidate, it must stay exactly
	// there — never pulled ahead by its synthetic band-0 tie, never
	// pushed back by the known reordering around it.
	configured := []string{"gpt-5.5-pro", "gemini-3.1-pro", "claude-opus-4-8"}
	got, decisions := meter.OrderModels(configured)
	assertOrder(t, got, []string{"claude-opus-4-8", "gemini-3.1-pro", "gpt-5.5-pro"})
	if decisions[1].Known() {
		t.Fatalf("gemini decision = %+v, want unknown", decisions[1])
	}
}

func TestMeterOrderModelsDoesNotMutateInput(t *testing.T) {
	meter := liveMeter(t, quota.Policy{})
	configured := []string{"claude-opus-4-8", "gpt-5.5-pro", "gemini-3.1-pro"}
	original := append([]string(nil), configured...)
	if _, _ = meter.OrderModels(configured); true {
		assertOrder(t, configured, original)
	}
}

func TestMeterOrderModelsPairsDecisionsWithTheirModels(t *testing.T) {
	meter := liveMeter(t, quota.Policy{})
	models, decisions := meter.OrderModels([]string{"claude-opus-4-8", "gpt-5.5-pro", "gemini-3.1-pro"})
	if len(models) != len(decisions) {
		t.Fatalf("got %d models and %d decisions", len(models), len(decisions))
	}
	for i, model := range models {
		want := meter.DecisionForModel(model)
		if decisions[i].Family != want.Family || decisions[i].RemainingPercent != want.RemainingPercent {
			t.Errorf("decision[%d] for %s = %+v, want %+v", i, model, decisions[i], want)
		}
	}
}

// The workflow role resolver takes a capacity filter. Asserting the
// contract here means a signature drift on either side fails the build
// instead of silently unwiring quota from role resolution.
func TestMeterSatisfiesTheRoleCapacityChecker(t *testing.T) {
	var checker roles.CapacityChecker = quota.New(quota.Options{})
	if !checker.HasCapacity("claude-opus-4-8") {
		t.Error("a disabled meter must report capacity through the role resolver too")
	}

	resolver := roles.Resolver{Capacity: checker}
	got, err := resolver.Resolve(roles.Request{Role: "implementer"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Model == "" {
		t.Error("want a resolved model when the meter reports capacity")
	}
}

func TestMeterHasCapacityOnlyRejectsAPositiveExhaustionReading(t *testing.T) {
	snapshot := snapshotAt(time.Now(),
		readable("openai", window("Weekly", 1)),
		quota.ProviderQuota{Family: "anthropic", Availability: quota.AvailabilityAuthRequired, Note: "sign in"},
		readable("gemini", window("Weekly", 80)),
	)
	meter := newTestMeter(t, snapshot, quota.Policy{})

	if meter.HasCapacity("gpt-5.5-pro") {
		t.Error("a provider at 1% left must not report capacity")
	}
	if !meter.HasCapacity("claude-opus-4-8") {
		t.Error("an unreadable provider must report capacity, not exhaustion")
	}
	if !meter.HasCapacity("gemini-3.1-pro") {
		t.Error("a healthy provider must report capacity")
	}
}

func TestMeterUnresolvableModelRoutesUnchanged(t *testing.T) {
	meter := liveMeter(t, quota.Policy{})
	decision := meter.DecisionForModel("some-house-model-9000")
	if decision.Known() {
		t.Errorf("an unmapped model produced a known decision: %+v", decision)
	}
	if !meter.HasCapacity("some-house-model-9000") {
		t.Error("an unmapped model must report capacity")
	}
	if !strings.Contains(decision.Reason, "provider family") {
		t.Errorf("Reason = %q, want it to explain the mapping gap", decision.Reason)
	}
}

func TestMeterRefreshKeepsThePreviousReadingOnFailure(t *testing.T) {
	good := snapshotAt(time.Now(), readable("openai", window("Weekly", 80)))
	reader := &stubReader{snapshot: good}
	meter := quota.New(quota.Options{Reader: reader, RefreshInterval: -1})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	reader.mu.Lock()
	reader.snapshot, reader.err = nil, errors.New("codexbar vanished")
	reader.mu.Unlock()

	kept, err := meter.Refresh(context.Background())
	if err == nil {
		t.Fatal("want the read error surfaced")
	}
	if kept == nil {
		t.Fatal("a transient failure must not discard a usable reading")
	}
	if got := meter.DecisionFor("openai"); got.Class != quota.ClassHealthy {
		t.Errorf("Class = %q, want healthy from the retained reading", got.Class)
	}
}

// syncClock is a mutable, goroutine-safe clock for a test that advances
// time across a background refresh running on another goroutine.
type syncClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *syncClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *syncClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// TestMeterSnapshotBacksOffBetweenFailedRefreshesOfAnEmptyCache guards
// against a spawn storm: an empty cache with a reader that fails instantly
// (codexbar missing) never sets readAt, so every Snapshot call saw the
// cache as stale and, once the previous background refresh finished,
// kicked off another one immediately — with no floor on how often that
// could happen (gemini review on #1325).
func TestMeterSnapshotBacksOffBetweenFailedRefreshesOfAnEmptyCache(t *testing.T) {
	reader := &stubReader{err: errors.New("codexbar not installed")}
	clock := &syncClock{now: time.Now()}
	meter := quota.New(quota.Options{
		Reader:          reader,
		RefreshInterval: time.Millisecond, // the cache is always "stale" on its own
		Now:             clock.Now,
	})

	waitForReads := func(want int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if reader.readCount() >= want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("reader was called %d times, want at least %d", reader.readCount(), want)
	}

	_, _, _ = snapshotOf(meter)
	waitForReads(1)

	// The cache is still empty and the failed attempt just landed. A
	// burst of callers checking quota right after must not retrigger a
	// read for each one.
	for range 20 {
		_, _, _ = snapshotOf(meter)
	}
	time.Sleep(20 * time.Millisecond)
	if got := reader.readCount(); got != 1 {
		t.Fatalf("reader was called %d times right after a failed empty-cache read, want 1 (backoff should suppress the retry storm)", got)
	}

	// Once the backoff window has passed, a Snapshot call may retry.
	clock.Advance(31 * time.Second)
	_, _, _ = snapshotOf(meter)
	waitForReads(2)
}

func TestMeterSnapshotServesTheCacheWithoutReReading(t *testing.T) {
	reader := &stubReader{snapshot: snapshotAt(time.Now(), readable("openai", window("Weekly", 80)))}
	now := time.Now()
	meter := quota.New(quota.Options{
		Reader:          reader,
		RefreshInterval: time.Hour,
		Now:             func() time.Time { return now },
	})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	for range 10 {
		if _, _, err := snapshotOf(meter); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	}
	if got := reader.readCount(); got != 1 {
		t.Errorf("reader was called %d times, want 1 — the routing path must serve the cache", got)
	}
}

func TestMeterSnapshotDoesNotBlockWhenTheCacheIsCold(t *testing.T) {
	// A reader that would block for longer than any test should wait.
	// Snapshot must return immediately with no reading rather than
	// stalling a model call behind a multi-second meter refresh.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	meter := quota.New(quota.Options{
		Reader:          blockingReader{release: release},
		RefreshInterval: time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = snapshotOf(meter)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Snapshot blocked on the underlying reader")
	}
}

type blockingReader struct{ release <-chan struct{} }

func (b blockingReader) Read(ctx context.Context) (*quota.Snapshot, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil, errors.New("released")
}

func snapshotOf(m *quota.Meter) (*quota.Snapshot, bool, error) {
	snap, err := m.Snapshot()
	return snap, snap != nil, err
}

func newTestMeter(t *testing.T, snapshot *quota.Snapshot, policy quota.Policy) *quota.Meter {
	t.Helper()
	meter := quota.New(quota.Options{
		Reader:          &stubReader{snapshot: snapshot},
		Policy:          policy,
		RefreshInterval: -1,
	})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return meter
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
