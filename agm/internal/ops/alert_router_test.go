package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
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

// deliveryCandidates must fetch the active session list once, not once for
// the explicit-target lookup and again for supervisor discovery: the two
// were triggering the identical ListSessions query back to back on every
// routed alert.
func TestDeliveryCandidatesFetchesSessionListOnce(t *testing.T) {
	storage := &mockStorage{sessions: []*manifest.Manifest{
		testManifest("control-plane", manifest.StateReady, time.Now()),
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	}}
	router := NewAlertRouter(&OpContext{Storage: storage})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	router.sendMessage = func(context.Context, string, string) error { return nil }

	if _, err := router.Route(context.Background(), AlertRequest{
		Kind: "checker", Source: "c", Title: "t", Subject: "s",
		Actionability: AlertAgentActionable, Target: "control-plane",
		OccurredAt: time.Now(),
	}); err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if storage.listCalls != 1 {
		t.Fatalf("ListSessions called %d times, want exactly 1 (resolveTargetManifest and discoverSupervisors must share one fetch)", storage.listCalls)
	}
}

// The explicit-target search must see the complete active set: an artificial
// page limit here (rather than in discovery, which only ranks candidates)
// would let a workspace with more live sessions than that limit silently
// exclude an older-but-live pinned recipient from ever being found by name.
func TestDeliveryCandidatesSearchesUnboundedActiveSet(t *testing.T) {
	storage := &mockStorage{sessions: []*manifest.Manifest{
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	}}
	router := NewAlertRouter(&OpContext{Storage: storage})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))

	if _, err := router.deliveryCandidates("Dispatch"); err != nil {
		t.Fatalf("deliveryCandidates() error = %v", err)
	}
	if storage.lastFilter == nil {
		t.Fatal("ListSessions was not called")
	}
	if storage.lastFilter.Limit != 0 {
		t.Fatalf("ListSessions filter Limit = %d, want unbounded (0)", storage.lastFilter.Limit)
	}
}

// A ListSessions failure while searching for an explicit target is a lookup
// failure, not proof the target is gone. Treating the two the same would let
// a transient storage error silently drop the operator's configured relay
// target and reroute the alert elsewhere (or queue it) while the real
// recipient may have been live the whole time.
func TestDeliveryCandidatesKeepsExplicitTargetWhenListingFails(t *testing.T) {
	storage := &mockStorage{listErr: fmt.Errorf("dolt: connection reset")}
	router := NewAlertRouter(&OpContext{Storage: storage})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))

	candidates, err := router.deliveryCandidates("merge-non-dearagent-prs")
	if err == nil {
		t.Fatal("deliveryCandidates() error = nil, want the ListSessions failure surfaced")
	}
	if len(candidates) != 1 || candidates[0] != "merge-non-dearagent-prs" {
		t.Fatalf("candidates = %v, want the explicit target kept despite the lookup failure", candidates)
	}
}

// End to end: a storage error must not silently vanish into the generic "no
// live supervisor session discovered" — that message asserts something was
// checked and found absent, when in fact nothing could be checked at all.
func TestAlertRouterRecordsListErrorRatherThanFalseAbsence(t *testing.T) {
	storage := &mockStorage{listErr: fmt.Errorf("dolt: connection reset")}
	router := NewAlertRouter(&OpContext{Storage: storage})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind: "checker", Source: "c", Title: "t", Subject: "s",
		Actionability: AlertAgentActionable, OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Status != AlertStatusQueued {
		t.Fatalf("Status = %q, want queued", rec.Status)
	}
	if !strings.Contains(rec.Error, "connection reset") {
		t.Fatalf("Error = %q, want it to surface the underlying storage failure", rec.Error)
	}
}

// A target resolved only via the case-insensitive name match or an ID
// prefix must be delivered through the canonical manifest identity, not the
// raw string the operator or event supplied: SendMessage's own resolver
// (exact-case name via findActiveByName, or a full ID via GetSession) does
// not accept either alias, so delivering the raw string would fail despite
// this candidate having just been declared reachable.
func TestAlertRouterDeliversUsingCanonicalNameForCaseInsensitiveTarget(t *testing.T) {
	router, sent := testRouterIDOnly(t, testManifest("Dispatch", manifest.StateDone, time.Now()))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind: "completion", Source: "s", Title: "t", Subject: "s",
		Actionability: AlertAgentActionable, Target: "dispatch",
		OccurredAt: time.Now(), DedupeKey: "canonical-name",
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Status != AlertStatusDispatched {
		t.Fatalf("Status = %q (error %q), want dispatched", rec.Status, rec.Error)
	}
	if rec.Target != "Dispatch" {
		t.Fatalf("Target = %q, want the canonical name %q", rec.Target, "Dispatch")
	}
	if len(*sent) != 1 || (*sent)[0] != "Dispatch" {
		t.Fatalf("sent = %v, want delivery addressed to the canonical name, not the lowercase alias", *sent)
	}
}

// Same regression, for the ID-prefix alias.
func TestAlertRouterDeliversUsingCanonicalNameForIDPrefixTarget(t *testing.T) {
	m := testManifest("worker", manifest.StateDone, time.Now())
	m.SessionID = "164eb3e7-6603-49d6-9c29-4ee88e9b7fe0"
	router, sent := testRouterIDOnly(t, m)

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind: "completion", Source: "s", Title: "t", Subject: "s",
		Actionability: AlertAgentActionable, Target: "164eb3e7",
		OccurredAt: time.Now(), DedupeKey: "canonical-id-prefix",
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Status != AlertStatusDispatched {
		t.Fatalf("Status = %q (error %q), want dispatched", rec.Status, rec.Error)
	}
	if rec.Target != "worker" {
		t.Fatalf("Target = %q, want the canonical name %q, not the raw ID prefix", rec.Target, "worker")
	}
	if len(*sent) != 1 || (*sent)[0] != "worker" {
		t.Fatalf("sent = %v, want delivery addressed to the canonical name", *sent)
	}
}

// SessionIsLiveConfirmed is the seam ce-6 (the state-detector DONE ambiguity)
// lives in: DONE covers both a completion watcher's confirmed-idle
// observation and the state detector's unconfirmed "safe default", and only
// a harness-liveness check can tell them apart.
func TestSessionIsLiveConfirmedVerifiesDoneAgainstHarnessLiveness(t *testing.T) {
	live := testManifest("dispatch", manifest.StateDone, time.Now())
	zombie := testManifest("zombie-dispatch", manifest.StateDone, time.Now())
	ready := testManifest("ready-dispatch", manifest.StateReady, time.Now())

	tm := &mockTmuxWithLiveness{
		mockTmux: newMockTmux(),
		liveness: map[string]session.LivenessInfo{
			"dispatch":        {SessionExists: true, HarnessAlive: true},
			"zombie-dispatch": {SessionExists: true, HarnessAlive: false},
			"ready-dispatch":  {SessionExists: true, HarnessAlive: false},
		},
	}

	if !SessionIsLiveConfirmed(live, tm) {
		t.Fatal("SessionIsLiveConfirmed() = false for a DONE session with a confirmed-live harness")
	}
	if SessionIsLiveConfirmed(zombie, tm) {
		t.Fatal("SessionIsLiveConfirmed() = true for a DONE session whose harness process is confirmed gone")
	}
	// READY already implies a confirmed-running harness upstream of the
	// manifest write (see state.DetectState), so it is not re-verified here
	// even though the fake liveness map says otherwise for this name.
	if !SessionIsLiveConfirmed(ready, tm) {
		t.Fatal("SessionIsLiveConfirmed() = false for a READY session; only DONE should be re-verified")
	}
}

// Without a HarnessLivenessChecker (nil tmux, or a TmuxInterface that does
// not implement the capability), verification is unavailable and the
// manifest state must be trusted, matching every routing surface's behavior
// before this capability existed.
func TestSessionIsLiveConfirmedTrustsStateWithoutALivenessChecker(t *testing.T) {
	done := testManifest("dispatch", manifest.StateDone, time.Now())
	if !SessionIsLiveConfirmed(done, nil) {
		t.Fatal("SessionIsLiveConfirmed() = false for a nil tmux; should fail open and trust the state")
	}
	if !SessionIsLiveConfirmed(done, newMockTmux()) {
		t.Fatal("SessionIsLiveConfirmed() = false for a tmux without the liveness capability; should fail open")
	}
}

// End to end: routing must not hand a completion alert to a DONE session
// whose harness process has actually exited, even though this PR's fix
// otherwise treats DONE as reachable.
func TestAlertRouterFallsThroughAZombieDoneExplicitTarget(t *testing.T) {
	zombieTarget := testManifest("zombie-dispatch", manifest.StateDone, time.Now())
	backupSupervisor := testManifest("vroom-orchestrator", manifest.StateReady, time.Now())
	tm := &mockTmuxWithLiveness{
		mockTmux: newMockTmux(),
		liveness: map[string]session.LivenessInfo{
			"zombie-dispatch": {SessionExists: true, HarnessAlive: false},
		},
	}
	router := NewAlertRouter(&OpContext{
		Storage: &mockStorage{sessions: []*manifest.Manifest{zombieTarget, backupSupervisor}},
		Tmux:    tm,
	})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	var sent []string
	router.sendMessage = func(_ context.Context, recipient, _ string) error {
		sent = append(sent, recipient)
		return nil
	}

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind: "completion", Source: "s", Title: "t", Subject: "s",
		Actionability: AlertAgentActionable, Target: "zombie-dispatch",
		OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Target == "zombie-dispatch" {
		t.Fatal("Target = zombie-dispatch, want routing to skip a DONE target with no confirmed-live harness")
	}
	if rec.Status != AlertStatusDispatched || rec.Target != "vroom-orchestrator" {
		t.Fatalf("Status/Target = %q/%q, want dispatched to the live fallback supervisor", rec.Status, rec.Target)
	}
	if len(sent) != 1 || sent[0] != "vroom-orchestrator" {
		t.Fatalf("sent = %v, want the zombie target skipped entirely", sent)
	}
}

func jsonLine(rec AlertRecord) ([]byte, error) {
	data, err := jsonMarshalAlert(rec)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// The real watcher builds an OpContext with no Config, so workspace scoping
// has to work without one or the dedupe fix is inert in production.
func TestAlertRouterDerivesWorkspaceWithoutAConfig(t *testing.T) {
	t.Setenv("ENGRAM_WORKSPACE", "/tmp/workspace-alpha")
	router := NewAlertRouter(&OpContext{})
	if router.workspace != "/tmp/workspace-alpha" {
		t.Fatalf("workspace = %q, want the detected workspace", router.workspace)
	}
}

// The regression this fix exists for. The completion watcher stamps DONE
// on every session that finishes a unit of work. When DONE counted as
// terminal, that session immediately disqualified itself as a delivery
// target, so an operator's explicitly configured relay target was
// rejected and the alert queued behind "no live supervisor session
// discovered" while the recipient sat idle at its composer.
func TestAlertRouterDeliversToExplicitTargetThatHasCompletedWork(t *testing.T) {
	router, sent := testRouter(t, testManifest("merge-non-dearagent-prs", manifest.StateDone, time.Now()))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind:          "completion",
		Source:        "agm-completion-watcher",
		Title:         "AGM session idle: worker-1",
		Body:          "worker-1 finished working and is idle.",
		Subject:       "worker-1",
		Severity:      AlertSeverityInfo,
		Actionability: AlertAgentActionable,
		Target:        "merge-non-dearagent-prs",
		OccurredAt:    time.Now(),
		DedupeKey:     "worker-1:1",
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Status != AlertStatusDispatched {
		t.Fatalf("Status = %q (error %q), want dispatched to a DONE-but-live target",
			rec.Status, rec.Error)
	}
	if rec.Target != "merge-non-dearagent-prs" {
		t.Fatalf("Target = %q, want the explicitly configured relay target", rec.Target)
	}
	if len(*sent) != 1 || (*sent)[0] != "merge-non-dearagent-prs" {
		t.Fatalf("sent = %v, want one delivery to merge-non-dearagent-prs", *sent)
	}
	if !rec.Delivered() {
		t.Fatal("Delivered() = false for a dispatched alert")
	}
}

// Discovery has to see completed supervisors too, or a Dispatch session
// becomes unreachable as soon as it finishes its own first turn.
func TestDiscoverSupervisorsIncludesSessionThatHasCompletedWork(t *testing.T) {
	router, _ := testRouter(t,
		testManifest("worker-1", manifest.StateWorking, time.Now()),
		testManifest("vroom-dispatch", manifest.StateDone, time.Now()),
	)
	got := router.discoverSupervisors()
	if len(got) != 1 || got[0] != "vroom-dispatch" {
		t.Fatalf("discoverSupervisors() = %v, want the DONE-but-live vroom-dispatch", got)
	}
}

// The reachability predicate is the seam the regression lived in, so it
// is asserted directly: DONE is reachable, gone is not.
func TestSessionIsReachableTreatsDoneAsLiveAndGoneAsNot(t *testing.T) {
	archived := testManifest("archived-dispatch", manifest.StateReady, time.Now())
	archived.Lifecycle = manifest.LifecycleArchived

	for _, tc := range []struct {
		name string
		m    *manifest.Manifest
		want bool
	}{
		{"done is reachable", testManifest("done-dispatch", manifest.StateDone, time.Now()), true},
		{"ready is reachable", testManifest("ready-dispatch", manifest.StateReady, time.Now()), true},
		{"working is reachable", testManifest("busy-dispatch", manifest.StateWorking, time.Now()), true},
		{"offline is not", testManifest("offline-dispatch", manifest.StateOffline, time.Now()), false},
		{"archived lifecycle is not", archived, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, _ := testRouter(t, tc.m)
			if got := router.sessionIsReachable(tc.m.Name); got != tc.want {
				t.Fatalf("sessionIsReachable(%q with state %q) = %v, want %v",
					tc.m.Name, tc.m.State, got, tc.want)
			}
		})
	}
}

// An unknown session name is not reachable, so a stale relay target
// cannot make the router type into a session that does not exist.
func TestSessionIsReachableRejectsUnknownSession(t *testing.T) {
	router, _ := testRouter(t, testManifest("dispatch", manifest.StateDone, time.Now()))
	if router.sessionIsReachable("no-such-session") {
		t.Fatal("sessionIsReachable() = true for an unknown session")
	}
}

// idOnlyStorage mirrors the real Dolt adapter, whose GetSession runs
// `WHERE id = ?` and so never matches a session name. The shared
// mockStorage resolves by name as well, which is why the name-resolution
// bug passed every unit test while failing on every real host.
type idOnlyStorage struct{ *mockStorage }

func (s *idOnlyStorage) GetSession(identifier string) (*manifest.Manifest, error) {
	for _, m := range s.sessions {
		if m.SessionID == identifier {
			return m, nil
		}
	}
	return nil, ErrSessionNotFound(identifier)
}

// testRouterIDOnly builds a router over storage with real ID-only
// GetSession semantics.
func testRouterIDOnly(t *testing.T, sessions ...*manifest.Manifest) (*AlertRouter, *[]string) {
	t.Helper()
	router := NewAlertRouter(&OpContext{Storage: &idOnlyStorage{&mockStorage{sessions: sessions}}})
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	var sent []string
	router.sendMessage = func(_ context.Context, recipient, _ string) error {
		sent = append(sent, recipient)
		return nil
	}
	return router, &sent
}

// `agm completion relay-target set` takes a session name, so a target
// named that way has to resolve against storage that keys on ID.
func TestSessionIsReachableResolvesTargetByName(t *testing.T) {
	m := testManifest("merge-non-dearagent-prs", manifest.StateDone, time.Now())
	router, _ := testRouterIDOnly(t, m)

	if !router.sessionIsReachable("merge-non-dearagent-prs") {
		t.Fatal("sessionIsReachable() = false for a live session named by name")
	}
}

// Name matching is case-insensitive, matching how AGM accepts recipients
// elsewhere.
func TestSessionIsReachableResolvesTargetByNameCaseInsensitively(t *testing.T) {
	m := testManifest("Dispatch", manifest.StateDone, time.Now())
	router, _ := testRouterIDOnly(t, m)

	if !router.sessionIsReachable("dispatch") {
		t.Fatal("sessionIsReachable() = false for a name differing only in case")
	}
}

// A UUID prefix is a valid recipient identifier, and must resolve.
func TestSessionIsReachableResolvesTargetByIDPrefix(t *testing.T) {
	m := testManifest("worker", manifest.StateDone, time.Now())
	m.SessionID = "164eb3e7-6603-49d6-9c29-4ee88e9b7fe0"
	router, _ := testRouterIDOnly(t, m)

	if !router.sessionIsReachable("164eb3e7") {
		t.Fatal("sessionIsReachable() = false for an 8-character session-ID prefix")
	}
	if router.sessionIsReachable("164e") {
		t.Fatal("sessionIsReachable() = true for a prefix too short to be unambiguous")
	}
}

// The end-to-end shape of the outage: an operator sets a relay target by
// name, that session has completed work, and the completion must be
// delivered to it rather than queued behind a discovery miss.
func TestAlertRouterDeliversToNamedRelayTargetThatHasCompletedWork(t *testing.T) {
	router, sent := testRouterIDOnly(t,
		testManifest("continue-dearagent-drain", manifest.StateDone, time.Now()),
		testManifest("merge-non-dearagent-prs", manifest.StateDone, time.Now()),
	)

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind:          "completion",
		Source:        "agm-completion-watcher",
		Title:         "AGM session idle: continue-dearagent-drain",
		Body:          "continue-dearagent-drain finished working and is idle.",
		Subject:       "continue-dearagent-drain",
		Severity:      AlertSeverityInfo,
		Actionability: AlertAgentActionable,
		Target:        "merge-non-dearagent-prs",
		OccurredAt:    time.Now(),
		DedupeKey:     "continue-dearagent-drain:e2e",
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Status != AlertStatusDispatched {
		t.Fatalf("Status = %q (error %q), want dispatched to the named relay target",
			rec.Status, rec.Error)
	}
	if len(*sent) != 1 || (*sent)[0] != "merge-non-dearagent-prs" {
		t.Fatalf("sent = %v, want one delivery to merge-non-dearagent-prs", *sent)
	}
}
