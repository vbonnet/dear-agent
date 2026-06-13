package main

import (
	"strings"
	"testing"
)

func TestRun_MissingPRFlag(t *testing.T) {
	err := run([]string{})
	if err == nil {
		t.Fatal("expected error when --pr not provided, got nil")
	}
	if !strings.Contains(err.Error(), "--pr") {
		t.Errorf("error should mention --pr flag, got: %v", err)
	}
}

func TestRun_MissingRepo(t *testing.T) {
	// No --repo and no GITHUB_REPOSITORY env var set.
	t.Setenv("GITHUB_REPOSITORY", "")
	err := run([]string{"--pr", "1"})
	if err == nil {
		t.Fatal("expected error when repo is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Errorf("error should mention --repo, got: %v", err)
	}
}

func TestRun_RepoFromEnv(t *testing.T) {
	// Repo resolved from env; will still fail at the CI-gate stage (no gh CLI),
	// but the error should not be about missing repo.
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	err := run([]string{"--pr", "1"})
	// The error may be anything (gh not found, etc.), but NOT "repo is required".
	if err != nil && strings.Contains(err.Error(), "--repo is required") {
		t.Errorf("should resolve repo from env, got: %v", err)
	}
}

func TestUsageContainsBinaryName(t *testing.T) {
	if !strings.Contains(usage, "safe-merge") {
		t.Errorf("usage string should mention the binary name, got: %q", usage)
	}
}
