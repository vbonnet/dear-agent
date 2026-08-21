package prconcern

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// collectTimeout bounds the diff walk so a wedged repository cannot hold a CI
// job open for its whole timeout budget.
const collectTimeout = 60 * time.Second

// Collect runs the diff between base and head and returns its parsed records.
//
// The range is `base...head` (three dots), matching pr-size-scope.yml: it
// diffs head against the merge base, so commits landed on the base branch
// since the PR was opened are not attributed to this PR.
//
// -M enables rename detection, without which every move arrives as an
// unrelated delete plus add and the move-only signal can never fire.
func Collect(ctx context.Context, repoDir, base, head string) ([]Change, error) {
	if base == "" || head == "" {
		return nil, fmt.Errorf("both a base and a head revision are required")
	}
	ctx, cancel := context.WithTimeout(ctx, collectTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	argv = append(argv, "diff", "-M", "--numstat", "-z", base+"..."+head)
	// No shell: argv is compile-time literals plus the caller's revisions,
	// which git treats as revision arguments rather than options.
	cmd := exec.CommandContext(ctx, "git", argv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s...%s failed: %w: %s", base, head, err, strings.TrimSpace(stderr.String()))
	}
	return ParseNumstatZ(string(out))
}
