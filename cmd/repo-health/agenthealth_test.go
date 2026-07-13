package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBDDCoverageUsesCanonicalExecutableDirectory(t *testing.T) {
	root := t.TempDir()
	writeFeature := func(rel string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("Feature: Test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFeature("agm/test/bdd/features/executable.feature")
	writeFeature("docs/aspirational.feature")

	metric, total, executable := bddCoverage(&scanCtx{root: root})
	if !metric.Available {
		t.Fatalf("BDD metric unavailable: %s", metric.Note)
	}
	if total != 2 || executable != 1 {
		t.Fatalf("BDD coverage = %d/%d, want 1/2", executable, total)
	}
}

func TestEARSCoverageIncludesCrossLanguageImplementationDirs(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("go/pkg/main.go", 0o644)
	write("go/pkg/SPEC.md", 0o644)
	write("web/app/index.ts", 0o644)
	write("web/app/SPEC.md", 0o644)
	write("ops/hooks/pre-commit", 0o755)
	write("ops/hooks/SPEC.md", 0o644)
	write("containers/image/Dockerfile", 0o644)
	write("containers/image/SPEC.md", 0o644)
	write("automation/Makefile", 0o644)
	write("automation/SPEC.md", 0o644)
	write("docs/readme.md", 0o644)

	metric, total, withSpec := earsCoverage(&scanCtx{root: root})
	if !metric.Available {
		t.Fatalf("EARS metric unavailable: %s", metric.Note)
	}
	if total != 5 || withSpec != 5 {
		t.Fatalf("EARS coverage = %d/%d, want 5/5", withSpec, total)
	}
}
