package main

import (
	"errors"
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

func TestExplicitXDGDirectoryPlanRetriesCreatedAncestorSync(t *testing.T) {
	base := t.TempDir()
	createdAncestor := filepath.Join(base, "created-ancestor")
	stateRoot := filepath.Join(createdAncestor, "state")
	t.Setenv("XDG_STATE_HOME", stateRoot)
	target := filepath.Join(stateRoot, "dear-agent", "resolve-review-threads")

	plan, err := buildContinuationReceiptDirectoryPlan(target)
	if err != nil {
		t.Fatalf("build explicit XDG directory plan: %v", err)
	}
	volume := filepath.VolumeName(target)
	wantAnchor := volume + string(filepath.Separator)
	if volume == "" {
		wantAnchor = string(filepath.Separator)
	}
	if plan.anchor != wantAnchor {
		t.Fatalf("explicit XDG durability anchor = %q, want %q", plan.anchor, wantAnchor)
	}

	failedSync := errors.New("injected XDG ancestor sync failure")
	failParent := filepath.Dir(createdAncestor)
	failed := false
	err = ensurePrivateContinuationDirectoryWithSync(plan.anchor, plan.target, func(path string) error {
		if path == failParent && !failed {
			failed = true
			return failedSync
		}
		return nil
	})
	if !errors.Is(err, failedSync) {
		t.Fatalf("first explicit XDG preflight error = %v, want injected sync failure", err)
	}
	if _, statErr := os.Lstat(createdAncestor); statErr != nil {
		t.Fatalf("created XDG ancestor was not left for retry: %v", statErr)
	}

	plan, err = buildContinuationReceiptDirectoryPlan(target)
	if err != nil {
		t.Fatalf("rebuild explicit XDG directory plan: %v", err)
	}
	retriedFailedParent := false
	err = ensurePrivateContinuationDirectoryWithSync(plan.anchor, plan.target, func(path string) error {
		if path == failParent {
			retriedFailedParent = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry explicit XDG preflight: %v", err)
	}
	if !retriedFailedParent {
		t.Fatalf("retry did not re-sync parent %s for the already-created XDG ancestor", failParent)
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
