package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// testRouter builds a router with an isolated queue and a delivery seam
// that records recipients instead of touching tmux.
func testRouter(t *testing.T, sessions ...*manifest.Manifest) (*AlertRouter, *[]string) {
	t.Helper()
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{sessions: sessions}})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	var sent []string
	router.sendMessage = func(_ context.Context, recipient, _ string) error {
		sent = append(sent, recipient)
		return nil
	}
	return router, &sent
}

func TestAlertRouterClassifiesCriticalAgentActionable(t *testing.T) {
	dir := t.TempDir()
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{}})
	router.SetQueuePath(filepath.Join(dir, "alerts.jsonl"))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind:       "checker",
		Source:     "auth-checker",
		Title:      "Claude Auth At Risk",
		Body:       "token family may expire",
		Subject:    "claude-auth",
		OccurredAt: time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Severity != AlertSeverityCritical {
		t.Fatalf("Severity = %q, want %q", rec.Severity, AlertSeverityCritical)
	}
	if rec.Actionability != AlertAgentActionable {
		t.Fatalf("Actionability = %q, want %q", rec.Actionability, AlertAgentActionable)
	}
	if rec.Status != AlertStatusQueued {
		t.Fatalf("Status = %q, want queued fallback with no live supervisor", rec.Status)
	}
	if rec.Delivered() {
		t.Fatal("Delivered() = true for a queued alert")
	}
}

func TestAlertRouterDedupesDeliveredChecker(t *testing.T) {
	router, sent := testRouter(t, testManifest("Dispatch", manifest.StateReady, time.Now()))
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)
	req := AlertRequest{Kind: "checker", Source: "auth-checker", Title: "Claude Auth At Risk", Subject: "claude-auth"}

	req.OccurredAt = when
	first, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("first Route() error = %v", err)
	}
	if first.Status != AlertStatusDispatched {
		t.Fatalf("first Status = %q, want dispatched", first.Status)
	}

	req.OccurredAt = when.Add(2 * time.Minute)
	second, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("second Route() error = %v", err)
	}
	if second.Status != AlertStatusSuppressed {
		t.Fatalf("second Status = %q, want suppressed", second.Status)
	}
	if !second.Delivered() {
		t.Fatal("Delivered() = false for a duplicate of a dispatched alert")
	}
	if second.RepeatCount != 1 {
		t.Fatalf("RepeatCount = %d, want 1", second.RepeatCount)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want the duplicate suppressed", len(*sent))
	}

	// The suppression must be persisted, or repeats are invisible to
	// `agm alerts list` and RepeatCount can never grow.
	records, err := ReadAlertRecords(router.queuePath, 10)
	if err != nil {
		t.Fatalf("ReadAlertRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want the dispatch and its suppressed repeat", len(records))
	}
}

// A queued alert reached nobody, so a repeat is the retry that finally
// delivers it, not a duplicate to swallow.
func TestAlertRouterRetriesAfterAQueuedAttempt(t *testing.T) {
	router, sent := testRouter(t)
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)
	req := AlertRequest{
		Kind: "checker", Source: "disk-checker", Title: "Disk floor",
		Subject: "disk", Severity: AlertSeverityCritical, Actionability: AlertAgentActionable,
		OccurredAt: when,
	}
	first, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("first Route() error = %v", err)
	}
	if first.Status != AlertStatusQueued {
		t.Fatalf("first Status = %q, want queued with no supervisor", first.Status)
	}

	// A supervisor appears.
	router.ctx.Storage = &mockStorage{sessions: []*manifest.Manifest{
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	}}
	req.OccurredAt = when.Add(2 * time.Minute)
	second, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("second Route() error = %v", err)
	}
	if second.Status != AlertStatusDispatched {
		t.Fatalf("second Status = %q, want dispatched; a queued predecessor must not suppress the retry", second.Status)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent = %v, want one delivery", *sent)
	}
}

// An escalation inside the dedupe window is new information, not a repeat.
func TestAlertRouterDeliversSeverityEscalationInsideWindow(t *testing.T) {
	router, sent := testRouter(t, testManifest("Dispatch", manifest.StateReady, time.Now()))
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)
	req := AlertRequest{
		Kind: "checker", Source: "quota-checker", Title: "Quota pressure", Subject: "quota",
		Actionability: AlertAgentActionable, Severity: AlertSeverityWarning, OccurredAt: when,
	}
	if _, err := router.Route(context.Background(), req); err != nil {
		t.Fatalf("warning Route() error = %v", err)
	}
	req.Severity = AlertSeverityCritical
	req.OccurredAt = when.Add(time.Minute)
	escalated, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("critical Route() error = %v", err)
	}
	if escalated.Status != AlertStatusDispatched {
		t.Fatalf("escalated Status = %q, want dispatched", escalated.Status)
	}
	if len(*sent) != 2 {
		t.Fatalf("sent %d messages, want the escalation delivered too", len(*sent))
	}
}

// A persistent worker that finishes two tasks produces the same kind,
// source, subject, and title both times. Without an occurrence identity the
// second result would be discarded as a duplicate and never relayed.
func TestAlertRouterDeliversDistinctCompletionsFromOneSession(t *testing.T) {
	router, sent := testRouter(t, testManifest("Dispatch", manifest.StateReady, time.Now()))
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)
	base := AlertRequest{
		Kind: "completion", Source: "agm-completion-watcher", Title: "AGM session idle: worker-7",
		Subject: "worker-7", Actionability: AlertAgentActionable, Severity: AlertSeverityInfo,
	}
	first := base
	first.OccurredAt = when
	first.DedupeKey = "worker-7:1"
	second := base
	second.OccurredAt = when.Add(5 * time.Minute)
	second.DedupeKey = "worker-7:2"

	for _, req := range []AlertRequest{first, second} {
		rec, err := router.Route(context.Background(), req)
		if err != nil {
			t.Fatalf("Route() error = %v", err)
		}
		if rec.Status != AlertStatusDispatched {
			t.Fatalf("Status = %q for %s, want dispatched", rec.Status, req.DedupeKey)
		}
	}
	if len(*sent) != 2 {
		t.Fatalf("sent %d completions, want both relayed", len(*sent))
	}
}

// Keyword inference may fill an unstated actionability, never overrule a
// stated one: a worker result mentioning an OAuth fix is still the
// agent-actionable completion its caller typed.
func TestClassifyAlertKeepsExplicitActionability(t *testing.T) {
	got := classifyAlert(AlertRequest{
		Kind: "completion", Title: "OAuth credential refresh fixed",
		Body: "rotated the oauth credential", Actionability: AlertAgentActionable,
	})
	if got.Actionability != AlertAgentActionable {
		t.Fatalf("Actionability = %q, want the caller's agent_actionable preserved", got.Actionability)
	}
}

func TestClassifyAlertStillInfersUnstatedActionability(t *testing.T) {
	got := classifyAlert(AlertRequest{
		Kind: "credential", Title: "Credential decision needed",
		Body: "needs Valentin to approve a new oauth consent screen",
	})
	if got.Actionability != AlertHumanOnly {
		t.Fatalf("Actionability = %q, want inferred human_only", got.Actionability)
	}
}

func TestAlertRouterClassifiesHumanOnlyWithoutPagingAgent(t *testing.T) {
	dir := t.TempDir()
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{}})
	router.SetQueuePath(filepath.Join(dir, "alerts.jsonl"))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind:       "credential",
		Source:     "oauth-refresh",
		Title:      "Credential decision needed",
		Body:       "needs Valentin to approve a new OAuth consent screen",
		Subject:    "oauth",
		OccurredAt: time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Actionability != AlertHumanOnly {
		t.Fatalf("Actionability = %q, want %q", rec.Actionability, AlertHumanOnly)
	}
	if rec.Status != AlertStatusQueued {
		t.Fatalf("Status = %q, want queued because no human recipient is configured in test", rec.Status)
	}
}

func TestAlertRouterDiscoversDispatchFirst(t *testing.T) {
	router, _ := testRouter(t,
		testManifest("worker-1", manifest.StateWorking, time.Now()),
		testManifest("vroom-orchestrator", manifest.StateReady, time.Now()),
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	)
	got := router.discoverSupervisors()
	if len(got) < 2 || got[0] != "Dispatch" {
		t.Fatalf("discoverSupervisors() = %v, want Dispatch ranked first", got)
	}
}

// A live supervisor tagged role:supervisor must be reachable even when its
// name carries no conventional keyword.
func TestAlertRouterDiscoversRoleTaggedSupervisor(t *testing.T) {
	tagged := testManifest("control-plane", manifest.StateReady, time.Now())
	tagged.Context.Tags = []string{"role:supervisor"}
	router, _ := testRouter(t, tagged)
	if got := router.discoverSupervisors(); len(got) != 1 || got[0] != "control-plane" {
		t.Fatalf("discoverSupervisors() = %v, want the role-tagged session", got)
	}
}

// An explicitly pinned target is checked for liveness only. The
// supervisor-name heuristic belongs to discovery; applying it here would
// discard a perfectly good pinned recipient.
func TestAlertRouterHonorsExplicitNonKeywordTarget(t *testing.T) {
	router, sent := testRouter(t,
		testManifest("control-plane", manifest.StateReady, time.Now()),
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	)
	rec, err := router.Route(context.Background(), AlertRequest{
		Kind: "checker", Source: "c", Title: "t", Subject: "s",
		Actionability: AlertAgentActionable, Target: "control-plane",
		OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Target != "control-plane" {
		t.Fatalf("Target = %q, want the explicitly pinned session", rec.Target)
	}
	if len(*sent) != 1 || (*sent)[0] != "control-plane" {
		t.Fatalf("sent = %v, want delivery to the pinned session", *sent)
	}
}

// When the preferred candidate cannot accept input, routing must try the
// next live supervisor before falling back to the queue.
func TestAlertRouterTriesNextCandidateBeforeQueueing(t *testing.T) {
	router, _ := testRouter(t,
		testManifest("Dispatch", manifest.StateReady, time.Now()),
		testManifest("vroom-orchestrator", manifest.StateReady, time.Now()),
	)
	var attempts []string
	router.sendMessage = func(_ context.Context, recipient, _ string) error {
		attempts = append(attempts, recipient)
		if recipient == "Dispatch" {
			return fmt.Errorf("session at a permission prompt")
		}
		return nil
	}
	rec, err := router.Route(context.Background(), AlertRequest{
		Kind: "stall.permission_prompt", Source: "watcher", Title: "stall", Subject: "worker-3",
		Severity: AlertSeverityCritical, Actionability: AlertAgentActionable, OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Status != AlertStatusDispatched {
		t.Fatalf("Status = %q, want dispatched to the second candidate", rec.Status)
	}
	if rec.Target != "vroom-orchestrator" {
		t.Fatalf("Target = %q, want vroom-orchestrator", rec.Target)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %v, want both candidates tried", attempts)
	}
}

// Persistence is not delivery. An alert queued while no supervisor was
// reachable must be delivered once one appears.
func TestDrainQueuedDeliversOnceASupervisorAppears(t *testing.T) {
	router, sent := testRouter(t)
	if _, err := router.Route(context.Background(), AlertRequest{
		Kind: "checker", Source: "spawn-checker", Title: "Spawn failure", Subject: "spawn",
		Severity: AlertSeverityCritical, Actionability: AlertAgentActionable, OccurredAt: time.Now(),
	}); err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if len(*sent) != 0 {
		t.Fatalf("sent = %v, want nothing delivered yet", *sent)
	}

	router.ctx.Storage = &mockStorage{sessions: []*manifest.Manifest{
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	}}
	drained, err := router.DrainQueued(context.Background())
	if err != nil {
		t.Fatalf("DrainQueued() error = %v", err)
	}
	if drained != 1 {
		t.Fatalf("drained = %d, want 1", drained)
	}
	if len(*sent) != 1 || (*sent)[0] != "Dispatch" {
		t.Fatalf("sent = %v, want the queued alert delivered to Dispatch", *sent)
	}

	// A second drain must not redeliver what it already delivered.
	again, err := router.DrainQueued(context.Background())
	if err != nil {
		t.Fatalf("second DrainQueued() error = %v", err)
	}
	if again != 0 {
		t.Fatalf("second drain = %d, want 0", again)
	}
}

// Two workspaces share the host-wide queue; neither may silence the other.
func TestAlertRouterScopesDedupeByWorkspace(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "alerts.jsonl")
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)
	req := AlertRequest{
		Kind: "checker", Source: "auth-checker", Title: "Auth at risk", Subject: "auth",
		Actionability: AlertAgentActionable, OccurredAt: when,
	}
	var delivered []string
	newRouter := func(workspace string) *AlertRouter {
		r := NewAlertRouter(&OpContext{Storage: &mockStorage{sessions: []*manifest.Manifest{
			testManifest("Dispatch", manifest.StateReady, time.Now()),
		}}})
		r.SetQueuePath(queue)
		r.SetWorkspace(workspace)
		r.sendMessage = func(_ context.Context, recipient, _ string) error {
			delivered = append(delivered, workspace+"->"+recipient)
			return nil
		}
		return r
	}
	for _, workspace := range []string{"alpha", "beta"} {
		rec, err := newRouter(workspace).Route(context.Background(), req)
		if err != nil {
			t.Fatalf("Route(%s) error = %v", workspace, err)
		}
		if rec.Status != AlertStatusDispatched {
			t.Fatalf("Route(%s) Status = %q, want dispatched", workspace, rec.Status)
		}
		if rec.Workspace != workspace {
			t.Fatalf("Workspace = %q, want %q", rec.Workspace, workspace)
		}
	}
	if len(delivered) != 2 {
		t.Fatalf("delivered = %v, want one alert per workspace", delivered)
	}
}

// Two watcher processes routing the same alert must not both dispatch it.
// The dedupe read, the send, and the append are one critical section held
// across processes; without the lock both can pass the read before either
// appends. Run under -race.
func TestAlertRouterSerializesDedupeAcrossConcurrentRouters(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "alerts.jsonl")
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	var delivered int
	newRouter := func() *AlertRouter {
		r := NewAlertRouter(&OpContext{Storage: &mockStorage{sessions: []*manifest.Manifest{
			testManifest("Dispatch", manifest.StateReady, time.Now()),
		}}})
		r.SetQueuePath(queue)
		r.sendMessage = func(_ context.Context, _, _ string) error {
			mu.Lock()
			delivered++
			mu.Unlock()
			// Widen the window a real tmux write would occupy.
			time.Sleep(2 * time.Millisecond)
			return nil
		}
		return r
	}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			_, err := newRouter().Route(context.Background(), AlertRequest{
				Kind: "checker", Source: "auth-checker", Title: "Auth at risk", Subject: "auth",
				Actionability: AlertAgentActionable,
				OccurredAt:    when.Add(time.Duration(i) * time.Second),
			})
			if err != nil {
				t.Errorf("Route() error = %v", err)
			}
		})
	}
	wg.Wait()

	mu.Lock()
	got := delivered
	mu.Unlock()
	if got != 1 {
		t.Fatalf("delivered %d copies of one deduplicated alert, want exactly 1", got)
	}
}

// Reading the queue must not scan the whole file; it must still return the
// newest records.
func TestReadAlertRecordsReadsABoundedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	padding := make([]byte, 4096)
	for i := range padding {
		padding[i] = 'x'
	}
	for i := range 600 {
		rec := AlertRecord{ID: fmt.Sprintf("alert-%d", i), Fingerprint: "fp", Body: string(padding)}
		line, marshalErr := jsonLine(rec)
		if marshalErr != nil {
			t.Fatalf("marshal error = %v", marshalErr)
		}
		if _, err := f.Write(line); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() <= alertQueueTailBytes {
		t.Fatalf("test file is %d bytes, need more than the %d-byte tail bound", info.Size(), alertQueueTailBytes)
	}
	records, err := ReadAlertRecords(path, 5)
	if err != nil {
		t.Fatalf("ReadAlertRecords() error = %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %d, want 5", len(records))
	}
	if records[4].ID != "alert-599" {
		t.Fatalf("newest record = %q, want alert-599", records[4].ID)
	}
}

// Filtering has to happen before the limit: a queued critical alert behind
// 50 dispatched completions must still be findable.
func TestReadAlertRecordsWithStatusFiltersBeforeLimiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.jsonl")
	if err := appendAlertRecord(path, AlertRecord{ID: "stranded", Status: AlertStatusQueued}); err != nil {
		t.Fatalf("appendAlertRecord() error = %v", err)
	}
	for i := range 60 {
		if err := appendAlertRecord(path, AlertRecord{ID: fmt.Sprintf("c-%d", i), Status: AlertStatusDispatched}); err != nil {
			t.Fatalf("appendAlertRecord() error = %v", err)
		}
	}
	got, err := ReadAlertRecordsWithStatus(path, 50, AlertStatusQueued)
	if err != nil {
		t.Fatalf("ReadAlertRecordsWithStatus() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "stranded" {
		t.Fatalf("got %v, want the stranded queued alert", got)
	}
}

func jsonLine(rec AlertRecord) ([]byte, error) {
	data, err := jsonMarshalAlert(rec)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
