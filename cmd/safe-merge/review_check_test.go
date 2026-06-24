package main

import (
	"strings"
	"testing"
)

func TestRun_SkipReviewCheckFlag(t *testing.T) {
	// --skip-review-check must be accepted by the flag parser.
	// The run will fail at the CI gate (no real gh), but the error
	// must not be "flag provided but not defined: -skip-review-check".
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	err := run([]string{"--pr", "1", "--skip-review-check"})
	if err != nil && strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("--skip-review-check not registered: %v", err)
	}
}

func TestRun_SkipReviewCheckFlagDefault(t *testing.T) {
	// Without --skip-review-check, run should fail at CI gate,
	// not at an unknown flag error.
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	err := run([]string{"--pr", "1"})
	if err != nil && strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected flag error: %v", err)
	}
}

func TestUsageContainsSkipReviewCheck(t *testing.T) {
	if !strings.Contains(usage, "--skip-review-check") {
		t.Error("usage string must document --skip-review-check flag")
	}
}
