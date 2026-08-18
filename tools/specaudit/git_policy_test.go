package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	gitPolicyProbeTimeout        = 30 * time.Second
	gitPolicyProbeCleanupReserve = time.Second
	gitPolicyProbeStartedMarker  = ".specaudit-git-policy-started"
)

func TestGitWallTimeIsBounded(t *testing.T) {
	requireLinuxCallerSelectedGit(t)
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := resolveTestExecutable(t, fakeGit)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := gitBytesWithContext(ctx, executable, t.TempDir(), 64, nil, "version")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("gitBytesWithContext() error = %v, want deterministic wall-time rejection", err)
	}
}

func TestGitCommandPolicyDisablesLazyFetchAndAmbientRouting(t *testing.T) {
	helper := filepath.Join(realTempDir(t), "record-git-policy")
	script := strings.Join([]string{
		"#!/bin/sh",
		": > " + gitPolicyProbeStartedMarker,
		"printf '%s\\n' \"$@\"",
		"printf 'GIT_NO_LAZY_FETCH=%s\\n' \"${GIT_NO_LAZY_FETCH-}\"",
		"printf 'GIT_NO_REPLACE_OBJECTS=%s\\n' \"${GIT_NO_REPLACE_OBJECTS-}\"",
		"printf 'GIT_DIR=%s\\n' \"${GIT_DIR-}\"",
		"printf 'GIT_WORK_TREE=%s\\n' \"${GIT_WORK_TREE-}\"",
	}, "\n") + "\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "attacker.git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	root := t.TempDir()
	startedMarker := filepath.Join(root, gitPolicyProbeStartedMarker)
	ctx, cancel, budget := newGitPolicyProbeContext(t)
	defer cancel()
	output, err := gitBytesWithContext(ctx, gitExecutable{path: helper}, root, 4096, nil, "rev-parse", "HEAD")
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Git command policy probe exceeded %s: %s: %v", budget, gitPolicyProbeState(startedMarker), err)
	}
	if err != nil {
		t.Fatal(err)
	}
	policy := string(output)
	for _, want := range []string{
		"--no-replace-objects\n--no-lazy-fetch\n-C\n",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_DIR=\n",
		"GIT_WORK_TREE=\n",
	} {
		if !strings.Contains(policy, want) {
			t.Fatalf("Git command policy output %q omitted %q", policy, want)
		}
	}
}

func TestGitPolicyProbeState(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, gitPolicyProbeStartedMarker)
	if got := gitPolicyProbeState(marker); got != "helper did not start; process-start or suite scheduling was delayed" {
		t.Fatalf("missing marker state = %q", got)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := gitPolicyProbeState(marker); got != "helper started but did not complete" {
		t.Fatalf("present marker state = %q", got)
	}
}

func newGitPolicyProbeContext(t *testing.T) (context.Context, context.CancelFunc, time.Duration) {
	t.Helper()
	budget := gitPolicyProbeTimeout
	if deadline, ok := t.Deadline(); ok {
		available := time.Until(deadline) - gitPolicyProbeCleanupReserve
		if available <= 0 {
			t.Fatalf("Git command policy probe has no time before the test cleanup reserve")
		}
		if available < budget {
			budget = available
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	return ctx, cancel, budget
}

func gitPolicyProbeState(marker string) string {
	_, err := os.Stat(marker)
	switch {
	case err == nil:
		return "helper started but did not complete"
	case errors.Is(err, os.ErrNotExist):
		return "helper did not start; process-start or suite scheduling was delayed"
	default:
		return fmt.Sprintf("helper start state is unreadable: %v", err)
	}
}
