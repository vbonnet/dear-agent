package dev

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func TestLoadFixtures_MissingFileReturnsEmptySet(t *testing.T) {
	fs, err := LoadFixtures("/no/such/path")
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if len(fs.Responses) != 0 {
		t.Errorf("expected empty fixture set, got %d entries", len(fs.Responses))
	}
}

func TestLoadFixtures_ParsesYAMLAndDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fix.yaml")
	if err := os.WriteFile(p, []byte("a: hello\nb: world\n_default: fallback\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs, err := LoadFixtures(p)
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if v, ok := fs.Get("a"); !ok || v != "hello" {
		t.Errorf("Get(a) = %q,%v", v, ok)
	}
	if v, ok := fs.Get("c"); ok || v != "fallback" {
		t.Errorf("Get(c) should fall back to default; got %q,%v", v, ok)
	}
}

func TestFixtureSet_ReloadPicksUpChanges(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fix.yaml")
	if err := os.WriteFile(p, []byte("a: first\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs, err := LoadFixtures(p)
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if v, _ := fs.Get("a"); v != "first" {
		t.Errorf("Get(a) before reload = %q", v)
	}
	if err := os.WriteFile(p, []byte("a: second\nb: added\n"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	n, err := fs.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if n != 2 {
		t.Errorf("Reload count = %d, want 2", n)
	}
	if v, _ := fs.Get("a"); v != "second" {
		t.Errorf("Get(a) after reload = %q", v)
	}
}

func TestMockAIExecutor_UsesFixtureKeyWhenPresent(t *testing.T) {
	fs := &FixtureSet{Responses: map[string]string{"research": "ok"}}
	m := &MockAIExecutor{Fixtures: fs}
	out, err := m.Generate(context.Background(), &workflow.AINode{Role: "research", Prompt: "ignored"}, nil, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "ok" {
		t.Errorf("Generate = %q, want ok", out)
	}
}

func TestMockAIExecutor_FailOnGap(t *testing.T) {
	fs := &FixtureSet{Responses: map[string]string{}}
	m := &MockAIExecutor{Fixtures: fs, FailOnGap: true}
	if _, err := m.Generate(context.Background(), &workflow.AINode{Role: "missing", Prompt: "x"}, nil, nil); err == nil {
		t.Fatal("expected error on missing fixture")
	}
}

func TestMockAIExecutor_PlaceholderWhenLenient(t *testing.T) {
	m := &MockAIExecutor{}
	out, err := m.Generate(context.Background(), &workflow.AINode{Role: "x", Prompt: "p"}, nil, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out == "" {
		t.Errorf("expected placeholder, got empty")
	}
}

func TestFixtureFile_ConventionalCompanion(t *testing.T) {
	cases := map[string]string{
		"hello.yaml":           "hello.fixtures.yaml",
		"a/b/c.yml":            "a/b/c.fixtures.yaml",
		"workflow":             "workflow.fixtures.yaml",
	}
	for in, want := range cases {
		got := FixtureFile(in)
		if got != want {
			t.Errorf("FixtureFile(%q) = %q, want %q", in, got, want)
		}
	}
}
