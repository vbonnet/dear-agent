package freshness

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Result holds the outcome of a binary freshness check.
type Result struct {
	Stale        bool
	BinaryCommit string
	RepoHEAD     string
	RepoPath     string
	Error        error
}

// Check compares the running binary's embedded commit against the repo HEAD.
// It fails open: if staleness cannot be determined, Stale is false and Error is set.
func Check(repoPath string, binaryCommit string) Result {
	r := Result{
		BinaryCommit: binaryCommit,
		RepoPath:     repoPath,
	}

	// If binary was built without version info, it's definitely stale
	if binaryCommit == "" || binaryCommit == "unknown" {
		r.Stale = true
		r.RepoHEAD = "(unknown - run git rev-parse HEAD in repo)"
		return r
	}

	// Get repo HEAD with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		r.Error = err
		return r // fail open
	}

	r.RepoHEAD = strings.TrimSpace(string(out))

	// Strip -dirty suffix for comparison
	cleanCommit := strings.TrimSuffix(binaryCommit, "-dirty")

	// Compare using prefix match (short vs long hash)
	minLen := len(cleanCommit)
	if len(r.RepoHEAD) < minLen {
		minLen = len(r.RepoHEAD)
	}
	if minLen > 0 && cleanCommit[:minLen] != r.RepoHEAD[:minLen] {
		r.Stale = true
	}

	return r
}
