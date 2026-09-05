package audit

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStoreContractMemory and TestStoreContractSQLite both run the
// same battery of Store invariants against the two implementations.
// Keeping them parallel ensures behavioural drift between the
// reference (memory) and production (sqlite) is caught at PR time.
func TestStoreContractMemory(t *testing.T) {
	runStoreContract(t, func(t *testing.T) Store { return NewMemoryStore() })
}

func TestStoreContractSQLite(t *testing.T) {
	runStoreContract(t, func(t *testing.T) Store {
		dir := t.TempDir()
		s, err := OpenSQLiteStore(filepath.Join(dir, "audit.db"))
		if err != nil {
			t.Fatalf("OpenSQLiteStore: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}

// runStoreContract is the shared invariant suite. Subtests cover
// each Store method's documented behaviour; failures here are bugs
// in either implementation.
func runStoreContract(t *testing.T, mk func(t *testing.T) Store) {
	t.Helper()

	t.Run("upsert-new-finding-is-open", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		f, err := s.UpsertFinding(ctx, sampleFinding())
		if err != nil {
			t.Fatalf("UpsertFinding: %v", err)
		}
		if f.State != FindingOpen {
			t.Errorf("new finding state = %s, want open", f.State)
		}
		if f.FindingID == "" {
			t.Error("new finding should have id")
		}
		if f.FirstSeen.IsZero() || f.LastSeen.IsZero() {
			t.Error("new finding should have timestamps")
		}
	})

	t.Run("upsert-existing-bumps-last-seen", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		first, _ := s.UpsertFinding(ctx, sampleFinding())
		// pause to ensure the second timestamp differs even on coarse clocks
		time.Sleep(2 * time.Millisecond)
		second, err := s.UpsertFinding(ctx, sampleFinding())
		if err != nil {
			t.Fatalf("second UpsertFinding: %v", err)
		}
		if second.FindingID != first.FindingID {
			t.Errorf("dedup should reuse id; got %s vs %s", first.FindingID, second.FindingID)
		}
		if !second.LastSeen.After(first.LastSeen) {
			t.Errorf("last_seen should advance: %v vs %v", first.LastSeen, second.LastSeen)
		}
	})

	t.Run("upsert-resolved-reopens", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		first, _ := s.UpsertFinding(ctx, sampleFinding())
		if _, err := s.SetFindingState(ctx, first.FindingID, FindingResolved, ""); err != nil {
			t.Fatalf("SetFindingState resolved: %v", err)
		}
		again, err := s.UpsertFinding(ctx, sampleFinding())
		if err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		if again.State != FindingReopened {
			t.Errorf("re-emit after resolve should reopen; got %s", again.State)
		}
	})

	t.Run("remediation-suggestions-round-trip", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		suggestions := []Remediation{
			{},
			{Strategy: StrategyAuto},
			{Strategy: StrategyAuto, Command: "go test ./..."},
			{Strategy: StrategyPR, Title: "Investigate audit finding", Body: "Details"},
			{Strategy: StrategyPR, Patch: "diff --git a/a b/a", Title: "Fix audit finding", Body: "Details"},
			{Strategy: StrategyIssue},
			{Strategy: StrategyIssue, Title: "Investigate audit finding", Body: "Details"},
			{Strategy: StrategyNoop},
		}
		for i, suggestion := range suggestions {
			finding := sampleFinding()
			finding.Fingerprint = fmt.Sprintf("suggestion-%d", i)
			finding.Suggested = suggestion
			stored, err := s.UpsertFinding(ctx, finding)
			if err != nil {
				t.Fatalf("UpsertFinding(%d): %v", i, err)
			}
			got, err := s.GetFinding(ctx, stored.FindingID)
			if err != nil {
				t.Fatalf("GetFinding(%d): %v", i, err)
			}
			if got.Suggested != suggestion {
				t.Errorf("suggestion %d = %+v, want %+v", i, got.Suggested, suggestion)
			}
		}
	})

	t.Run("unknown-remediation-strategy-is-rejected-before-mutation", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		finding := sampleFinding()
		finding.Fingerprint = "unknown-strategy"
		finding.Suggested.Strategy = Strategy("future")

		if _, err := s.UpsertFinding(ctx, finding); err == nil {
			t.Fatal("UpsertFinding accepted an unknown remediation strategy")
		} else if !strings.Contains(err.Error(), "future") {
			t.Fatalf("UpsertFinding error = %q, want strategy context", err)
		}
		got, err := s.ListFindings(ctx, FindingFilter{Repo: finding.Repo})
		if err != nil {
			t.Fatalf("ListFindings: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("ListFindings = %+v, want no mutation", got)
		}
	})

	t.Run("manual-transitions-illegal-paths-rejected", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		f, _ := s.UpsertFinding(ctx, sampleFinding())
		// open → reopened is the runner's job, not the operator's
		if _, err := s.SetFindingState(ctx, f.FindingID, FindingReopened, ""); err == nil {
			t.Error("manual open → reopened should be rejected")
		}
	})

	t.Run("count-buckets-correctly", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		_, _ = s.UpsertFinding(ctx, sampleFinding())
		alt := sampleFinding()
		alt.Fingerprint = "fp-2"
		alt.Title = "second"
		f2, _ := s.UpsertFinding(ctx, alt)
		_, _ = s.SetFindingState(ctx, f2.FindingID, FindingResolved, "")

		c, err := s.CountFindings(ctx, "demo")
		if err != nil {
			t.Fatalf("CountFindings: %v", err)
		}
		if c.Open != 1 || c.Resolved != 1 {
			t.Errorf("counts: %+v want Open=1 Resolved=1", c)
		}
	})

	t.Run("list-respects-filters-and-sort", func(t *testing.T) {
		s := mk(t)
		ctx := context.Background()
		f1 := sampleFinding()
		f1.Severity = SeverityP3
		f1.Fingerprint = "p3"
		_, _ = s.UpsertFinding(ctx, f1)
		f2 := sampleFinding()
		f2.Severity = SeverityP1
		f2.Fingerprint = "p1"
		_, _ = s.UpsertFinding(ctx, f2)

		list, err := s.ListFindings(ctx, FindingFilter{Repo: "demo"})
		if err != nil {
			t.Fatalf("ListFindings: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("ListFindings len = %d, want 2", len(list))
		}
		if list[0].Severity != SeverityP1 {
			t.Errorf("sort: most severe first; got %s then %s", list[0].Severity, list[1].Severity)
		}
	})
}

func TestSQLiteStoreRejectsUnknownRemediationStrategy(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		fixture := newCorruptStrategyFixture(t, "corrupt-strategy-get")
		_, err := fixture.store.GetFinding(fixture.ctx, fixture.stored.FindingID)
		assertCorruptStrategyError(t, err, fixture.stored.FindingID)
	})
	t.Run("list", func(t *testing.T) {
		fixture := newCorruptStrategyFixture(t, "corrupt-strategy-list")
		_, err := fixture.store.ListFindings(fixture.ctx, FindingFilter{Repo: fixture.finding.Repo})
		assertCorruptStrategyError(t, err, fixture.stored.FindingID)
	})
	t.Run("state-transition", func(t *testing.T) {
		fixture := newCorruptStrategyFixture(t, "corrupt-strategy-state")
		before := readRawFindingSnapshot(t, fixture)

		_, err := fixture.store.SetFindingState(
			fixture.ctx, fixture.stored.FindingID, FindingResolved, "triaged",
		)
		assertCorruptStrategyError(t, err, fixture.stored.FindingID)

		after := readRawFindingSnapshot(t, fixture)
		if after != before {
			t.Fatalf("finding mutated after rejected state transition:\n before: %+v\n  after: %+v", before, after)
		}
	})

	t.Run("re-emission", func(t *testing.T) {
		fixture := newCorruptStrategyFixture(t, "corrupt-strategy-reemission")
		before := readRawFindingSnapshot(t, fixture)

		reemitted := sampleFinding()
		reemitted.Fingerprint = fixture.finding.Fingerprint
		reemitted.Severity = SeverityP3
		reemitted.Title = "replacement title"
		reemitted.Detail = "replacement detail"
		reemitted.Path = "replacement.go"
		reemitted.Line = 99
		reemitted.Suggested = Remediation{
			Strategy: StrategyPR,
			Command:  "go test ./pkg/audit/...",
			Patch:    "diff --git a/pkg/audit/a.go b/pkg/audit/a.go",
			Title:    "Replacement remediation",
			Body:     "Replacement body",
		}
		reemitted.Evidence = map[string]any{"replacement": true}
		if _, err := fixture.store.UpsertFinding(fixture.ctx, reemitted); err == nil {
			t.Fatal("UpsertFinding accepted an unknown stored remediation strategy")
		} else {
			assertCorruptStrategyError(t, err, fixture.stored.FindingID)
		}

		after := readRawFindingSnapshot(t, fixture)
		if after != before {
			t.Fatalf("finding mutated after rejected re-emission:\n before: %+v\n  after: %+v", before, after)
		}
	})
}

type corruptStrategyFixture struct {
	ctx     context.Context
	store   *SQLiteStore
	finding Finding
	stored  Finding
}

func newCorruptStrategyFixture(t *testing.T, fingerprint string) corruptStrategyFixture {
	t.Helper()
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	finding := sampleFinding()
	finding.Fingerprint = fingerprint
	stored, err := store.UpsertFinding(ctx, finding)
	if err != nil {
		t.Fatalf("UpsertFinding: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"UPDATE audit_findings SET suggested_strategy = ? WHERE finding_id = ?",
		"future", stored.FindingID,
	); err != nil {
		t.Fatalf("corrupt suggested_strategy: %v", err)
	}
	return corruptStrategyFixture{ctx: ctx, store: store, finding: finding, stored: stored}
}

func assertCorruptStrategyError(t *testing.T, err error, findingID string) {
	t.Helper()
	if err == nil {
		t.Fatal("operation accepted an unknown stored remediation strategy")
	}
	for _, want := range []string{findingID, "future"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("operation error = %q, want %q context", err, want)
		}
	}
}

// rawFindingSnapshot includes every audit_findings column. In particular, it
// covers the union of columns that UpsertFinding and SetFindingState may write,
// so rejected lifecycle operations must leave the entire row field-for-field
// identical at the typed SQL boundary.
type rawFindingSnapshot struct {
	findingID         string
	repo              string
	fingerprint       string
	checkID           string
	severity          string
	state             string
	title             string
	detail            string
	path              string
	line              int
	firstSeen         time.Time
	lastSeen          time.Time
	resolvedAt        sql.NullTime
	stateNote         string
	suggestedStrategy string
	suggestedCommand  string
	suggestedPatch    string
	suggestedTitle    string
	suggestedBody     string
	evidenceJSON      string
}

func readRawFindingSnapshot(t *testing.T, fixture corruptStrategyFixture) rawFindingSnapshot {
	t.Helper()
	var got rawFindingSnapshot
	if err := fixture.store.DB().QueryRowContext(fixture.ctx, `
		SELECT finding_id, repo, fingerprint, check_id, severity, state,
		       title, detail, path, line, first_seen, last_seen, resolved_at,
		       state_note, suggested_strategy, suggested_command,
		       suggested_patch, suggested_title, suggested_body, evidence_json
		  FROM audit_findings WHERE finding_id = ?`, fixture.stored.FindingID).Scan(
		&got.findingID, &got.repo, &got.fingerprint, &got.checkID,
		&got.severity, &got.state, &got.title, &got.detail, &got.path, &got.line,
		&got.firstSeen, &got.lastSeen, &got.resolvedAt, &got.stateNote,
		&got.suggestedStrategy, &got.suggestedCommand, &got.suggestedPatch,
		&got.suggestedTitle, &got.suggestedBody, &got.evidenceJSON,
	); err != nil {
		t.Fatalf("read raw finding snapshot: %v", err)
	}
	return got
}

func sampleFinding() Finding {
	return Finding{
		Repo:        "demo",
		CheckID:     "test-check",
		Fingerprint: "fp-1",
		Severity:    SeverityP1,
		Title:       "broke a thing",
		Detail:      "details",
		Path:        "main.go",
		Line:        42,
	}
}
