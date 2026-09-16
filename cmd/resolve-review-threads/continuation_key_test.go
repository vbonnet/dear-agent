package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestEnsurePrivateContinuationDirectoryRetriesEveryParentSync(t *testing.T) {
	testCases := []struct {
		name      string
		failIndex int
	}{
		{name: "first component", failIndex: 0},
		{name: "middle component", failIndex: 1},
		{name: "final component", failIndex: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			anchor := t.TempDir()
			first := filepath.Join(anchor, "first")
			middle := filepath.Join(first, "middle")
			target := filepath.Join(middle, "target")
			expectedParents := []string{anchor, first, middle}
			failedSync := errors.New("injected parent sync failure")
			firstAttemptFailed := false
			firstSync := func(path string) error {
				if path == expectedParents[tc.failIndex] && !firstAttemptFailed {
					firstAttemptFailed = true
					return failedSync
				}
				return nil
			}

			err := ensurePrivateContinuationDirectoryWithSync(anchor, target, firstSync)
			if !errors.Is(err, failedSync) {
				t.Fatalf("first preflight error = %v, want injected sync failure", err)
			}
			if !firstAttemptFailed {
				t.Fatal("first preflight did not reach selected parent sync")
			}

			retryCalls := make([]string, 0, len(expectedParents))
			err = ensurePrivateContinuationDirectoryWithSync(anchor, target, func(path string) error {
				retryCalls = append(retryCalls, path)
				return nil
			})
			if err != nil {
				t.Fatalf("retry preflight: %v", err)
			}
			if !reflect.DeepEqual(retryCalls, expectedParents) {
				t.Fatalf("retry parent syncs = %q, want %q", retryCalls, expectedParents)
			}
			if runtime.GOOS != "windows" {
				for _, path := range []string{first, middle, target} {
					info, statErr := os.Lstat(path)
					if statErr != nil {
						t.Fatalf("inspect created directory %s: %v", path, statErr)
					}
					if got := info.Mode().Perm(); got != 0o700 {
						t.Errorf("created directory %s mode = %04o, want 0700", path, got)
					}
				}
			}
		})
	}
}

func TestExplicitXDGDirectoryPlanSyncsOnlyManagedDescendants(t *testing.T) {
	base := t.TempDir()
	externalAncestor := filepath.Join(base, "external-ancestor")
	if err := os.Mkdir(externalAncestor, 0o700); err != nil {
		t.Fatalf("create external ancestor: %v", err)
	}
	stateRoot := filepath.Join(externalAncestor, "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatalf("create explicit XDG state root: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateRoot)
	target := filepath.Join(stateRoot, "dear-agent", "resolve-review-threads")

	plan, err := buildContinuationReceiptDirectoryPlan(target)
	if err != nil {
		t.Fatalf("build explicit XDG directory plan: %v", err)
	}
	if plan.anchor != stateRoot {
		t.Fatalf("explicit XDG durability anchor = %q, want %q", plan.anchor, stateRoot)
	}

	expectedParents := []string{stateRoot, filepath.Join(stateRoot, "dear-agent")}
	var syncCalls []string
	err = ensurePrivateContinuationDirectoryWithSync(plan.anchor, plan.target, func(path string) error {
		if path == externalAncestor || path == base || path == filepath.Dir(base) {
			return fmt.Errorf("synchronized above explicit XDG root: %s", path)
		}
		syncCalls = append(syncCalls, path)
		return nil
	})
	if err != nil {
		t.Fatalf("prepare explicit XDG state: %v", err)
	}
	if !reflect.DeepEqual(syncCalls, expectedParents) {
		t.Fatalf("explicit XDG parent syncs = %q, want %q", syncCalls, expectedParents)
	}
}

func TestHomeFallbackDirectoryPlanPreservesAnchorAndSyncOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX continuation state is not supported on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	sharedStateDirectory := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(sharedStateDirectory, 0o755); err != nil {
		t.Fatalf("create fallback shared state directory: %v", err)
	}
	if err := os.Chmod(sharedStateDirectory, 0o755); err != nil {
		t.Fatalf("set fallback shared state directory mode: %v", err)
	}
	target := filepath.Join(sharedStateDirectory, "resolve-review-threads")

	plan, err := buildContinuationReceiptDirectoryPlan(target)
	if err != nil {
		t.Fatalf("build fallback directory plan: %v", err)
	}
	if plan.anchor != home {
		t.Fatalf("fallback durability anchor = %q, want %q", plan.anchor, home)
	}
	expectedParents := []string{
		home,
		filepath.Join(home, ".local"),
		filepath.Join(home, ".local", "state"),
		sharedStateDirectory,
	}
	var syncCalls []string
	if err := ensurePrivateContinuationDirectoryWithSync(plan.anchor, plan.target, func(path string) error {
		syncCalls = append(syncCalls, path)
		return nil
	}); err != nil {
		t.Fatalf("prepare fallback state: %v", err)
	}
	if !reflect.DeepEqual(syncCalls, expectedParents) {
		t.Fatalf("fallback parent syncs = %q, want %q", syncCalls, expectedParents)
	}
}

func TestExplicitXDGDirectoryPlanRequiresExistingBoundary(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "missing-state-root")
	t.Setenv("XDG_STATE_HOME", stateRoot)
	target := filepath.Join(stateRoot, "dear-agent", "resolve-review-threads")

	plan, err := buildContinuationReceiptDirectoryPlan(target)
	if err != nil {
		t.Fatalf("build explicit XDG directory plan: %v", err)
	}
	err = ensurePrivateContinuationDirectoryWithSync(plan.anchor, plan.target, func(string) error {
		t.Fatal("missing explicit XDG boundary attempted a directory sync")
		return nil
	})
	if err == nil {
		t.Fatal("missing explicit XDG state root was created by the command")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing explicit XDG state root error = %v, want os.ErrNotExist", err)
	}
	if _, statErr := os.Lstat(stateRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing explicit XDG state root was mutated: %v", statErr)
	}
}

func TestEnsurePrivateContinuationDirectoryRejectsPublicFinalDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions are not available")
	}
	anchor := t.TempDir()
	target := filepath.Join(anchor, "public")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("create public-directory fixture: %v", err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatalf("make directory fixture public: %v", err)
	}

	err := ensurePrivateContinuationDirectoryWithSync(anchor, target, func(string) error { return nil })
	if err == nil {
		t.Fatal("public final continuation directory passed private-directory validation")
	}
}
