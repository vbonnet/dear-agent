package main

import (
	"testing"
)

func TestDetermineJobs_GoFiles(t *testing.T) {
	t.Parallel()
	// determineJobs calls git, so we pass a non-existent repoRoot —
	// the git diff will fail and the fallback ["lint"] is returned.
	jobs := determineJobs("/nonexistent-repo-path")
	if len(jobs) == 0 {
		t.Error("determineJobs(invalid dir): expected fallback [\"lint\"], got empty slice")
	}
	found := false
	for _, j := range jobs {
		if j == "lint" {
			found = true
		}
	}
	if !found {
		t.Errorf("determineJobs fallback: expected \"lint\" in %v", jobs)
	}
}
