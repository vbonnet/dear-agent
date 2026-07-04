package beadstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyJSONL mirrors the real ~/.beads/issues.jsonl orphan store from the
// 2026-07-03 incident: beads acknowledged by the legacy MCP tool that never
// reached the bd store.
const legacyJSONL = `{"id": "98fe2fea", "title": "Implement sandbox directory GC", "description": "orphaned write", "priority": 0, "labels": ["gc"], "estimated_minutes": 480, "status": "open", "created_at": "2026-07-03T08:19:25+00:00"}
{"id": "8b539465", "title": "Add disk-free monitoring", "description": "orphaned write", "priority": 5, "labels": [], "estimated_minutes": 240, "status": "open", "created_at": "2026-07-03T08:19:38+00:00"}
{"id": "deadbeef", "title": "Already closed legacy bead", "description": "done", "priority": 2, "labels": [], "estimated_minutes": 10, "status": "closed", "created_at": "2026-07-01T00:00:00+00:00"}
`

func writeLegacy(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReconcile_BackfillsMissingOpenBeads(t *testing.T) {
	// Store already contains a bead previously backfilled from 98fe2fea
	// (identified by its backfill-src label), so only 8b539465 is missing.
	calls := []fakeCall{
		{stdout: `[{"id":"ce-old","title":"Implement sandbox directory GC","status":"open","labels":["backfill-src:98fe2fea"]}]`}, // list
		{stdout: `{"id":"ce-new","title":"Add disk-free monitoring","status":"open"}`},                                            // create
		{stdout: `[{"id":"ce-new","title":"Add disk-free monitoring","status":"open"}]`},                                          // show (verify)
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	res, err := s.Reconcile(context.Background(), writeLegacy(t, legacyJSONL), false)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Created) != 1 || res.Created[0].LegacyID != "8b539465" {
		t.Fatalf("Created = %+v, want exactly 8b539465", res.Created)
	}
	if res.Created[0].BeadID != "ce-new" {
		t.Errorf("BeadID = %q, want ce-new", res.Created[0].BeadID)
	}
	// 98fe2fea skipped (already backfilled), deadbeef skipped (not open).
	if len(res.Skipped) != 2 {
		t.Errorf("Skipped = %+v, want 2 entries", res.Skipped)
	}

	// The backfilled bead must carry provenance and be findable next run.
	createArgs := strings.Join(calls[1].args, " ")
	if !strings.Contains(createArgs, "backfill-src:8b539465") {
		t.Errorf("backfill create must label the legacy source ID, got: %v", calls[1].args)
	}
	// Legacy priority 5 must clamp into bd's 0-4 range.
	if !strings.Contains(createArgs, "--priority 4") {
		t.Errorf("legacy priority 5 should clamp to 4, got: %v", calls[1].args)
	}
}

func TestReconcile_SkipsTitleMatches(t *testing.T) {
	// The four lost ce-ctsi beads were re-filed manually; reconcile must not
	// duplicate a bead whose title already exists in the store.
	calls := []fakeCall{
		{stdout: `[
			{"id":"ce-m1","title":"Implement sandbox directory GC","status":"open"},
			{"id":"ce-m2","title":"add disk-free monitoring","status":"open"}
		]`},
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	res, err := s.Reconcile(context.Background(), writeLegacy(t, legacyJSONL), false)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Created) != 0 {
		t.Errorf("Created = %+v, want none (titles already present, case-insensitive)", res.Created)
	}
}

func TestReconcile_DryRunWritesNothing(t *testing.T) {
	calls := []fakeCall{
		{stdout: `[]`}, // list only; any further invocation fails the test
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	res, err := s.Reconcile(context.Background(), writeLegacy(t, legacyJSONL), true)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !res.DryRun {
		t.Error("DryRun flag not set on result")
	}
	if len(res.Created) != 2 {
		t.Errorf("dry run should report both open orphans as would-create, got %+v", res.Created)
	}
}

func TestReconcile_MissingJSONLIsError(t *testing.T) {
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &[]fakeCall{})}
	if _, err := s.Reconcile(context.Background(), "/nonexistent/issues.jsonl", false); err == nil {
		t.Fatal("want error for missing legacy store")
	}
}

func TestLoadLegacyJSONL_SkipsMalformedLines(t *testing.T) {
	path := writeLegacy(t, "not json\n"+`{"id":"aa","title":"ok","status":"open"}`+"\n")
	beads, err := LoadLegacyJSONL(path)
	if err != nil {
		t.Fatalf("LoadLegacyJSONL: %v", err)
	}
	if len(beads) != 1 || beads[0].ID != "aa" {
		t.Errorf("beads = %+v, want single 'aa'", beads)
	}
}
