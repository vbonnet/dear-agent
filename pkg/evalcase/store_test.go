package evalcase

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleCase(id string) EvalCase {
	return EvalCase{
		SchemaVersion:    SchemaVersion,
		ID:               id,
		SourceTraceID:    id,
		Task:             "do the thing",
		ExpectedBehavior: "thing done",
		ActualBehavior:   "thing not done",
		Classification:   ClassToolError,
		GeneratedAt:      time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
	}
}

func TestStore_SaveAndList(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)

	path, existed, err := s.Save(sampleCase("abc"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if existed {
		t.Fatal("first save reported existed")
	}
	if filepath.Dir(path) != s.CasesDir() {
		t.Errorf("case written outside cases dir: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("case file missing: %v", err)
	}

	cases, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cases) != 1 || cases[0].ID != "abc" {
		t.Fatalf("list = %+v", cases)
	}
}

func TestStore_SaveIsIdempotentAndNonDestructive(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)

	if _, existed, _ := s.Save(sampleCase("dup")); existed {
		t.Fatal("first save existed")
	}

	// Mutate and re-save under the same ID: the original must survive untouched.
	modified := sampleCase("dup")
	modified.ActualBehavior = "EDITED BY HUMAN"
	path, existed, err := s.Save(modified)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if !existed {
		t.Fatal("second save did not report existed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) == "" {
		t.Fatal("empty case file")
	}
	cases, _ := s.List()
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].ActualBehavior == "EDITED BY HUMAN" {
		t.Fatal("existing case was clobbered by re-save")
	}
}

func TestStore_HasReportsPresence(t *testing.T) {
	s := NewFileStore(t.TempDir())
	if s.Has("nope") {
		t.Fatal("Has true for missing case")
	}
	_, _, _ = s.Save(sampleCase("yes"))
	if !s.Has("yes") {
		t.Fatal("Has false for present case")
	}
}

func TestStore_ListMissingDirIsEmpty(t *testing.T) {
	s := NewFileStore(filepath.Join(t.TempDir(), "does-not-exist"))
	cases, err := s.List()
	if err != nil {
		t.Fatalf("list missing dir errored: %v", err)
	}
	if len(cases) != 0 {
		t.Fatalf("expected empty, got %d", len(cases))
	}
}
