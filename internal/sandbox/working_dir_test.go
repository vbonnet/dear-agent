package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchWorkingDirPreservesNestedRelativePath(t *testing.T) {
	repo := t.TempDir()
	workingDir := filepath.Join(repo, "agm", "cmd", "agm")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	match, err := MatchWorkingDir(workingDir, []string{repo})
	if err != nil {
		t.Fatalf("MatchWorkingDir() error = %v", err)
	}
	if match.LowerIndex != 0 || match.LowerDir != repo || match.RelativeDir != filepath.Join("agm", "cmd", "agm") {
		t.Fatalf("MatchWorkingDir() = %+v, want lower 0 and preserved relative directory", match)
	}
}

func TestMatchWorkingDirSelectsMostSpecificConfiguredRepository(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "nested-repo")
	workingDir := filepath.Join(inner, "pkg")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	match, err := MatchWorkingDir(workingDir, []string{outer, inner})
	if err != nil {
		t.Fatalf("MatchWorkingDir() error = %v", err)
	}
	if match.LowerIndex != 1 || match.LowerDir != inner || match.RelativeDir != "pkg" {
		t.Fatalf("MatchWorkingDir() = %+v, want most-specific lower directory", match)
	}
}

func TestMatchWorkingDirResolvesSymlinkAliases(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	workingDir := filepath.Join(repo, "wayfinder")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "repo-alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}

	match, err := MatchWorkingDir(filepath.Join(alias, "wayfinder"), []string{repo})
	if err != nil {
		t.Fatalf("MatchWorkingDir() error = %v", err)
	}
	if match.LowerIndex != 0 || match.RelativeDir != "wayfinder" {
		t.Fatalf("MatchWorkingDir() = %+v, want symlink-resolved relative directory", match)
	}
}

func TestMatchWorkingDirRejectsDirectoryOutsideConfiguredRepositories(t *testing.T) {
	_, err := MatchWorkingDir(t.TempDir(), []string{t.TempDir()})
	if err == nil {
		t.Fatal("MatchWorkingDir() error = nil, want fail-closed containment error")
	}
	var sandboxErr *Error
	if !errors.As(err, &sandboxErr) || sandboxErr.Code != ErrCodeInvalidConfig {
		t.Fatalf("MatchWorkingDir() error = %v, want ErrCodeInvalidConfig", err)
	}
}

func TestMapFlatWorkingDirReturnsMappedPathAndAuthoritativeRepository(t *testing.T) {
	firstRepo := t.TempDir()
	targetRepo := t.TempDir()
	requestedDir := filepath.Join(targetRepo, "agm", "internal")
	if err := os.MkdirAll(requestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mergedRoot := filepath.Join(t.TempDir(), "merged")

	mapped, matchedRepo, err := MapFlatWorkingDir(requestedDir, []string{firstRepo, targetRepo}, mergedRoot)
	if err != nil {
		t.Fatalf("MapFlatWorkingDir() error = %v", err)
	}
	if mapped != filepath.Join(mergedRoot, "agm", "internal") || matchedRepo != targetRepo {
		t.Fatalf("MapFlatWorkingDir() = %q, %q; want mapped path in %q", mapped, matchedRepo, targetRepo)
	}
}

func TestMapFlatWorkingDirDefaultsToMergedRootForLegacyRequest(t *testing.T) {
	mergedRoot := filepath.Join(t.TempDir(), "merged")
	mapped, matchedRepo, err := MapFlatWorkingDir("", nil, mergedRoot)
	if err != nil {
		t.Fatalf("MapFlatWorkingDir() error = %v", err)
	}
	if mapped != mergedRoot || matchedRepo != "" {
		t.Fatalf("MapFlatWorkingDir() = %q, %q; want legacy merged root", mapped, matchedRepo)
	}
}
