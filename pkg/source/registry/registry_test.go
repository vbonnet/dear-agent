package registry

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/source"
)

func TestBuiltins_Names(t *testing.T) {
	want := []string{"llm-wiki", "obsidian", "openviking", "sqlite"}
	got := Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i, n := range want {
		if got[i] != n {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], n)
		}
	}
}

func TestHas(t *testing.T) {
	if !Has("sqlite") {
		t.Error("Has(sqlite) = false")
	}
	if Has("nope") {
		t.Error("Has(nope) = true")
	}
}

func TestOpen_UnknownBackendError(t *testing.T) {
	_, err := Open("nope", "x")
	if err == nil || !errors.Is(err, err) {
		t.Fatalf("Open: %v", err)
	}
}

func TestOpen_SqliteRequiresPath(t *testing.T) {
	if _, err := Open("sqlite", ""); err == nil {
		t.Fatal("expected error for empty sqlite path")
	}
}

func TestOpen_ObsidianAndLLMWikiCreateDirs(t *testing.T) {
	dir := t.TempDir()
	a, err := Open("obsidian", filepath.Join(dir, "vault"))
	if err != nil {
		t.Fatalf("Open obsidian: %v", err)
	}
	defer a.Close()
	if a.Name() != "obsidian" {
		t.Errorf("Name = %q", a.Name())
	}

	b, err := Open("llm-wiki", filepath.Join(dir, "wiki"))
	if err != nil {
		t.Fatalf("Open llm-wiki: %v", err)
	}
	defer b.Close()
	if b.Name() != "llm-wiki" {
		t.Errorf("Name = %q", b.Name())
	}
}

func TestOpen_OpenVikingAcceptsURLOrJSON(t *testing.T) {
	a, err := Open("openviking", "bolt://host:7687")
	if err != nil {
		t.Fatalf("Open openviking url: %v", err)
	}
	defer a.Close()
	// Health check on the stub returns ErrNotImplemented — which is
	// what we want: the registry succeeds, the adapter signals "not
	// usable yet". This matches the Phase 5.3 design.
	if err := a.HealthCheck(context.Background()); err == nil {
		t.Error("expected stub HealthCheck to fail")
	}

	b, err := Open("openviking", `{"URL":"bolt://h","User":"u"}`)
	if err != nil {
		t.Fatalf("Open openviking json: %v", err)
	}
	defer b.Close()
}

func TestRegister_OverwriteAndReset(t *testing.T) {
	calls := 0
	custom := func(string) (source.Adapter, error) {
		calls++
		return nil, errors.New("test factory")
	}
	defer func() {
		// Restore real registry by running the package init body.
		Reset()
		Register("sqlite", openSQLite)
		Register("obsidian", openObsidian)
		Register("llm-wiki", openLLMWiki)
		Register("openviking", openOpenViking)
	}()
	Register("custom", custom)
	if !Has("custom") {
		t.Fatal("Has(custom) = false after Register")
	}
	if _, err := Open("custom", "x"); err == nil {
		t.Fatal("expected error from custom factory")
	}
	if calls != 1 {
		t.Errorf("custom factory called %d times, want 1", calls)
	}
	Reset()
	if Has("custom") || Has("sqlite") {
		t.Error("Reset did not clear factories")
	}
}

func TestRegister_RejectsEmptyOrNil(t *testing.T) {
	Register("", openSQLite)
	if Has("") {
		t.Error("registered empty name")
	}
	Register("not-nil", nil)
	if Has("not-nil") {
		t.Error("registered nil factory")
	}
}
