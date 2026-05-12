package benchmarks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeTaskFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestJSONFileLoader_LoadArrayShape(t *testing.T) {
	path := writeTaskFile(t, "tasks.json", `[
		{"id":"a","suite":"swe-bench-lite","prompt":"p1"},
		{"id":"b","suite":"swe-bench-lite","prompt":"p2"},
		{"id":"c","suite":"swe-bench-lite","prompt":"p3"}
	]`)
	loader := NewJSONFileLoader(path)
	got, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d tasks, want 3", len(got))
	}
	if got[0].ID != "a" || got[2].ID != "c" {
		t.Fatalf("unexpected task order: %+v", got)
	}
}

func TestJSONFileLoader_LoadNDJSONShape(t *testing.T) {
	path := writeTaskFile(t, "tasks.ndjson", `// header comment
{"id":"a","suite":"swe-bench-lite","prompt":"p1"}

// gap comment
{"id":"b","suite":"swe-bench-lite","prompt":"p2"}
{"id":"c","suite":"swe-bench-lite","prompt":"p3"}
`)
	loader := NewJSONFileLoader(path)
	got, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d tasks, want 3 (blank lines and // comments should be skipped)", len(got))
	}
}

func TestJSONFileLoader_LimitTruncates(t *testing.T) {
	path := writeTaskFile(t, "tasks.json", `[
		{"id":"a","suite":"swe-bench-lite","prompt":"p1"},
		{"id":"b","suite":"swe-bench-lite","prompt":"p2"},
		{"id":"c","suite":"swe-bench-lite","prompt":"p3"}
	]`)
	loader := NewJSONFileLoader(path)
	got, err := loader.Load(context.Background(), SuiteSWEBenchLite, 2)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tasks, want 2", len(got))
	}
}

func TestJSONFileLoader_FilterBySuite(t *testing.T) {
	path := writeTaskFile(t, "tasks.json", `[
		{"id":"a","suite":"swe-bench-lite","prompt":"p1"},
		{"id":"b","suite":"swe-bench-verified","prompt":"p2"},
		{"id":"c","suite":"swe-bench-lite","prompt":"p3"}
	]`)
	loader := &JSONFileLoader{Path: path, FilterBySuite: true}

	lite, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(lite) != 2 || lite[0].ID != "a" || lite[1].ID != "c" {
		t.Fatalf("filter-by-suite produced %+v, want a + c", lite)
	}

	verified, err := loader.Load(context.Background(), SuiteSWEBenchVerified, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(verified) != 1 || verified[0].ID != "b" {
		t.Fatalf("filter-by-suite produced %+v, want b", verified)
	}
}

func TestJSONFileLoader_RejectsEmptyPath(t *testing.T) {
	loader := &JSONFileLoader{Path: ""}
	if _, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0); err == nil {
		t.Fatal("expected error on empty Path, got nil")
	}
}

func TestJSONFileLoader_MissingFile(t *testing.T) {
	loader := NewJSONFileLoader(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if _, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0); err == nil {
		t.Fatal("expected error on missing file, got nil")
	}
}

func TestJSONFileLoader_EmptyFile(t *testing.T) {
	path := writeTaskFile(t, "empty.json", "")
	loader := NewJSONFileLoader(path)
	if _, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0); err == nil {
		t.Fatal("expected error on empty file, got nil")
	}
}

func TestJSONFileLoader_MalformedJSONReportsLine(t *testing.T) {
	path := writeTaskFile(t, "tasks.ndjson", `{"id":"a","suite":"swe-bench-lite","prompt":"p1"}
{not valid json}
{"id":"b","suite":"swe-bench-lite","prompt":"p3"}
`)
	loader := NewJSONFileLoader(path)
	_, err := loader.Load(context.Background(), SuiteSWEBenchLite, 0)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestJSONFileLoader_ContextCancelled(t *testing.T) {
	path := writeTaskFile(t, "tasks.json", `[{"id":"a","suite":"swe-bench-lite","prompt":"p1"}]`)
	loader := NewJSONFileLoader(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loader.Load(ctx, SuiteSWEBenchLite, 0); err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestJSONFileLoader_ImplementsTaskLoader(t *testing.T) {
	var _ TaskLoader = (*JSONFileLoader)(nil)
}
