// Package safegit stack.go routes stacked pull requests to the merge transport
// GitHub accepts for them.
//
// GitHub refuses the GraphQL mergePullRequest mutation for any PR that belongs
// to a stack, answering "This pull request is part of a stack and must be
// merged using the asynchronous merge REST API". `gh pr merge` speaks that
// mutation, so every gate can pass and the merge still fails at the transport.
// The synchronous REST merge endpoint refuses stacked PRs as well, so the
// merge-async endpoint is the single transport that accepts them.
//
// The PR payload carries an authoritative `stack` object, non-null exactly when
// the PR belongs to a stack, so the transport is selected from provider truth
// rather than from matching an error string. The REST endpoint takes a `sha`
// parameter that anchors the merge to an exact head, preserving the TOCTOU
// protection that --match-head-commit provides on the GraphQL path.
package safegit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// stackProbeTimeout bounds the stack-membership query. It is a single
// point-read against the PR payload, so it is short by design.
const stackProbeTimeout = mergeConfirmationCommandTimeout

// stackMembership is the subset of the PR payload that decides the transport.
// A nil Stack means the PR merges through the ordinary GraphQL path.
type stackMembership struct {
	Stack *struct {
		ID       int `json:"id"`
		Number   int `json:"number"`
		Position int `json:"position"`
		Size     int `json:"size"`
	} `json:"stack"`
}

// parseStackMembership reports whether a PR payload places the PR in a stack.
// An absent or null `stack` key means it does not.
func parseStackMembership(payload []byte) (bool, error) {
	var m stackMembership
	if err := json.Unmarshal(payload, &m); err != nil {
		return false, fmt.Errorf("parsing PR stack membership: %w", err)
	}
	return m.Stack != nil, nil
}

// resolveStackMembership asks GitHub whether the PR belongs to a stack. A
// failure is reported rather than guessed: choosing the wrong transport either
// fails loudly (GraphQL on a stacked PR) or surprises the caller, and neither
// belongs behind a silent default.
func resolveStackMembership(ctx context.Context, prNum int, repo string) (bool, error) {
	repoPath, err := escapedRepoPath(repo)
	if err != nil {
		return false, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, stackProbeTimeout)
	defer cancel()
	out, err := runCommand(exec.CommandContext(probeCtx, "gh", "api",
		fmt.Sprintf("repos/%s/pulls/%d", repoPath, prNum)))
	if err != nil {
		return false, fmt.Errorf("resolving stack membership for PR #%d: %w", prNum, err)
	}
	return parseStackMembership(out)
}

// BuildAsyncMergeArgs returns the argv for the asynchronous REST merge, the
// only transport GitHub accepts for a stacked PR. The ordinary REST merge
// endpoint refuses them too, so this is specifically the merge-async endpoint,
// which answers 202 and completes out of band. The `sha` parameter is the
// REST equivalent of --match-head-commit and carries the same TOCTOU anchor, so
// it is mandatory here for the same reason. Branch deletion is left to the
// repository's delete_branch_on_merge setting, which the GraphQL path's
// --delete-branch flag duplicates rather than replaces.
//
// Panics if headSHA is empty — the caller must resolve it first.
func BuildAsyncMergeArgs(prNum int, repo, headSHA string) []string {
	if headSHA == "" {
		panic("BuildAsyncMergeArgs: headSHA must not be empty — TOCTOU anchor requires a resolved SHA")
	}
	return []string{
		"gh", "api",
		"-X", "PUT",
		fmt.Sprintf("repos/%s/pulls/%d/merge-async", repo, prNum),
		"-f", "merge_method=squash",
		"-f", "sha=" + headSHA,
	}
}

// mergeArgsForTransport builds the argv for an already-resolved transport.
// Membership is resolved before the base-freshness gate rather than here, so
// the freshness proof stays the last provider read before the mutation: a probe
// between that proof and the merge would let the base advance unchecked for the
// probe's whole timeout, defeating the gate's TOCTOU protection.
func mergeArgsForTransport(stacked bool, prNum int, repo, headSHA string) []string {
	if stacked {
		return BuildAsyncMergeArgs(prNum, repo, headSHA)
	}
	return BuildMergeArgs(prNum, repo, headSHA)
}
