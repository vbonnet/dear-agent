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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// stackProbeTimeout bounds the stack-membership query. It is a single
// point-read against the PR payload, so it is short by design.
const stackProbeTimeout = mergeConfirmationCommandTimeout

// stackProbeWaitDelay bounds pipe draining if a probe descendant outlives the
// direct gh process while retaining stdout or stderr.
const stackProbeWaitDelay = mergeConfirmationCommandWaitDelay

// stackMembership is the subset of the PR payload that decides the transport.
// Stack is a non-pointer RawMessage so an explicit null can be told apart from
// a field the host never sent: absent leaves it nil while "stack": null leaves
// the literal bytes. A *json.RawMessage cannot make that distinction, because
// encoding/json nils the pointer for an explicit null too, which would conflate
// "not stacked" with "this API version has no such concept". The second must
// not silently pick a transport the provider may refuse.
type stackMembership struct {
	Stack json.RawMessage `json:"stack"`
}

// errStackFieldAbsent reports a payload that carries no stack field at all, so
// membership is undetermined rather than negative.
var errStackFieldAbsent = errors.New("PR payload carries no stack field, so the merge transport is undetermined")

// parseStackMembership reports whether a PR payload places the PR in a stack.
// An explicit null means not stacked; an absent field is an error, because
// guessing the ordinary route for a stacked PR recreates the failure this
// routing exists to prevent.
func parseStackMembership(payload []byte) (bool, error) {
	var m stackMembership
	if err := json.Unmarshal(payload, &m); err != nil {
		return false, fmt.Errorf("parsing PR stack membership: %w", err)
	}
	if m.Stack == nil {
		return false, errStackFieldAbsent
	}
	trimmed := bytes.TrimSpace(m.Stack)
	if string(trimmed) == "null" {
		return false, nil
	}
	return true, nil
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
	// Bound pipe draining too. Cancelling the context kills only the direct gh
	// process; a credential helper that inherited stdout or stderr can keep the
	// read blocked past the timeout, and this probe is on the mandatory merge
	// path, so an unbounded wait here would stall every merge.
	cmd := exec.CommandContext(probeCtx, "gh", "api",
		fmt.Sprintf("repos/%s/pulls/%d", repoPath, prNum))
	cmd.WaitDelay = stackProbeWaitDelay
	out, err := runCommand(cmd)
	if err != nil {
		if ctxErr := probeCtx.Err(); ctxErr != nil {
			err = ctxErr
		}
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

// transportName labels the provider interface a merge attempt used, so a
// failure or a pending confirmation reports the operation that was actually
// attempted rather than one the attempt never made.
func transportName(stacked bool) string {
	if stacked {
		return "asynchronous REST merge"
	}
	return "gh pr merge"
}

// resolveMergeTransport determines which provider interface the merge must use.
//
// It runs before the base-freshness gate: GitHub refuses the GraphQL merge
// mutation for stacked pull requests, so the interface has to be known, but the
// probe is a provider round trip. Running it after the freshness proof would
// let the target branch advance unchecked for the probe's whole timeout.
func resolveMergeTransport(ctx context.Context, prNum int, repo string) (bool, error) {
	stacked, err := resolveStackMembership(ctx, prNum, repo)
	if err != nil {
		appendAuditEntry(repo, prNum, "error", "merge transport: "+err.Error())
		return false, fmt.Errorf("selecting merge transport: %w", err)
	}
	return stacked, nil
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

// remoteHeadSurvives reports whether the merged head branch still exists.
//
// The GraphQL route passes --delete-branch; the async REST route has no such
// flag, and safe-merge deliberately does not delete the branch itself. GitHub's
// ref deletion takes no compare-and-swap, so a read-then-delete would still
// erase a branch that advanced between the two calls, destroying commits the
// merge never contained. A pull request opened from a fork makes it worse: the
// head branch lives in another repository.
//
// So the system observes rather than acts, and it observes the ref itself
// rather than delete_branch_on_merge. That setting is a configured intention,
// not an outcome: this repository's own branch-reaper exists because GitHub
// missed 14 of 1032 merged-branch deletions. Reading the ref in the head
// repository answers the actual question, and it is correct for forks.
func remoteHeadSurvives(ctx context.Context, headRepo, branch string) (bool, error) {
	if headRepo == "" || branch == "" {
		return false, fmt.Errorf("resolving remote head: head repository and branch are required")
	}
	repoPath, err := escapedRepoPath(headRepo)
	if err != nil {
		return false, err
	}
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stackProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, "gh", "api", liveRefEndpoint(repoPath, branch))
	cmd.WaitDelay = stackProbeWaitDelay
	if _, err := runCommand(cmd); err != nil {
		if ctxErr := probeCtx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		if isMissingRefResponse([]byte(err.Error())) {
			// A 404 is ambiguous: the provider answers it both for a ref that is
			// gone and for one the credential may not read, which a private fork
			// head produces after the merge. Only proven ref-read access makes
			// the absence evidence of deletion.
			if accessErr := confirmRefReadAccess(probeCtx, repoPath); accessErr != nil {
				return false, fmt.Errorf(
					"remote head %q in %s is unreadable, so its absence is not proof of deletion: %w",
					branch, headRepo, accessErr)
			}
			return false, nil
		}
		return false, fmt.Errorf("reading remote head %q in %s: %w", branch, headRepo, err)
	}
	return true, nil
}

// confirmRefReadAccess establishes that the credential can read Git refs in the
// repository, so a missing ref inside it means the ref is gone rather than
// hidden. Repository metadata is not enough: a fine-grained token can read
// metadata while lacking Contents, in which case the ref request 404s for
// authorization reasons and the branch may well survive.
func confirmRefReadAccess(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/git/refs/heads?per_page=1", repoPath))
	cmd.WaitDelay = stackProbeWaitDelay
	out, err := runCommand(cmd)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("listing refs in %s returned nothing", repoPath)
	}
	return nil
}

// isMissingRefResponse reports whether the provider answered that the ref is
// gone, which is what a deleted merged branch looks like.
func isMissingRefResponse(out []byte) bool {
	lower := bytes.ToLower(out)
	return bytes.Contains(lower, []byte("not found")) ||
		bytes.Contains(lower, []byte("reference does not exist"))
}

// reportRemoteHeadRetention tells the caller whether the merged head branch
// will survive, so branch cleanup is an informed decision rather than a
// surprise. It never fails the merge: the merge is confirmed by this point.
func reportRemoteHeadRetention(ctx context.Context, headRepo, branch string) {
	survives, err := remoteHeadSurvives(ctx, headRepo, branch)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr,
			"safe-merge: could not determine whether remote branch %q remains: %v\n", branch, err)
	case survives:
		fmt.Fprintf(os.Stderr,
			"safe-merge: note: remote branch %q still exists in %s after the merge\n",
			branch, headRepo)
	}
}
