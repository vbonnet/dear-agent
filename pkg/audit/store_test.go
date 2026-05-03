package audit

import (
	"context"
	"path/filepath"
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
