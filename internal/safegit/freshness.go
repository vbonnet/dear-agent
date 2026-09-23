// Package safegit freshness.go implements the base-freshness gate.
//
// A green check proves something about the base incorporated into the tested
// head, not automatically about the target branch's current tip. GitHub's PR
// snapshot fields can lag that live tip, so this gate resolves the target ref
// independently and proves ancestry between immutable commit OIDs.
//
// The gate runs last, after CI, threads, soak, and the expected-reviewer gates
// have all passed, so a branch update and subsequent CI cycle are only
// requested for a PR that already met every earlier merge bar.
package safegit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// Provider compare states are accepted only when their merge-base witness
// agrees with the requested live base. The state is useful diagnostics; the
// merge-base SHA is the ancestry proof.
const (
	compareAhead       = "ahead"
	compareBehind      = "behind"
	compareDiverged    = "diverged"
	compareIdentical   = "identical"
	mergeStateBehind   = "BEHIND"
	mergeStateClean    = "CLEAN"
	mergeStateDirty    = "DIRTY"
	mergeStateUnstable = "UNSTABLE"
)

// ErrBranchUpdateRequested reports that the provider accepted a request to update the
// head to the live base tip. It is a blocking outcome, not proof that the
// asynchronous update has completed; the next attempt re-proves every gate.
var ErrBranchUpdateRequested = errors.New("branch update to live base requested; wait for the provider and required checks, then retry")

type liveBaseSnapshot struct {
	Branch  string
	OID     string
	HeadOID string
}

type compareResult struct {
	Status     string `json:"status"`
	BaseCommit struct {
		SHA string `json:"sha"`
	} `json:"base_commit"`
	MergeBaseCommit struct {
		SHA string `json:"sha"`
	} `json:"merge_base_commit"`
}

func escapedRepoPath(repo string) (string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("repo must contain exactly owner/name, got %q", repo)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part ||
			strings.IndexFunc(part, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
			return "", fmt.Errorf("repo contains invalid owner or name segment %q", part)
		}
	}
	return url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}

func liveRefEndpoint(repoPath, branch string) string {
	return fmt.Sprintf("repos/%s/git/ref/%s", repoPath, url.PathEscape("heads/"+branch))
}

func validGitOID(oid string) bool {
	if len(oid) != 40 && len(oid) != 64 {
		return false
	}
	for _, c := range []byte(oid) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func parseLiveRefOID(out []byte, expectedRef string) (string, error) {
	var raw struct {
		Ref    string `json:"ref"`
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", err
	}
	if raw.Ref != expectedRef {
		return "", fmt.Errorf("response reported ref %q, want %q", raw.Ref, expectedRef)
	}
	if raw.Object.Type != "commit" {
		return "", fmt.Errorf("response reported object.type %q, want commit", raw.Object.Type)
	}
	if !validGitOID(raw.Object.SHA) {
		return "", fmt.Errorf("response reported invalid commit object.sha %q", raw.Object.SHA)
	}
	return raw.Object.SHA, nil
}

type prRefSnapshot struct {
	BaseRefName string `json:"baseRefName"`
	HeadRefOID  string `json:"headRefOid"`
}

func fetchPRRefSnapshot(ctx context.Context, prNum int, repo string) (prRefSnapshot, error) {
	out, err := runCommand(exec.CommandContext(ctx, "gh", "pr", "view",
		strconv.Itoa(prNum), "--repo", repo, "--json", "baseRefName,headRefOid"))
	if err != nil {
		return prRefSnapshot{}, err
	}
	var snapshot prRefSnapshot
	if err := json.Unmarshal(out, &snapshot); err != nil {
		return prRefSnapshot{}, err
	}
	if snapshot.BaseRefName == "" || snapshot.HeadRefOID == "" {
		return prRefSnapshot{}, fmt.Errorf("PR #%d response omitted baseRefName or headRefOid", prNum)
	}
	if !validGitOID(snapshot.HeadRefOID) {
		return prRefSnapshot{}, fmt.Errorf("PR #%d response reported invalid headRefOid %q", prNum, snapshot.HeadRefOID)
	}
	return snapshot, nil
}

func parseMergeStateStatus(out []byte) (string, error) {
	var raw struct {
		MergeStateStatus string `json:"mergeStateStatus"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", err
	}
	if raw.MergeStateStatus == "" {
		return "", fmt.Errorf("response reported no mergeStateStatus")
	}
	return raw.MergeStateStatus, nil
}

// prMergeStateStatus is remediation input only. It may authorize an automatic
// update after independent ancestry proof has already found the head stale; it
// is never accepted as evidence that the head contains the live base.
func prMergeStateStatus(ctx context.Context, prNum int, repo string) (string, error) {
	out, err := runCommand(exec.CommandContext(ctx, "gh", "pr", "view",
		strconv.Itoa(prNum), "--repo", repo, "--json", "mergeStateStatus"))
	if err != nil {
		return "", err
	}
	state, err := parseMergeStateStatus(out)
	if err != nil {
		return "", fmt.Errorf("PR #%d: %w", prNum, err)
	}
	return state, nil
}

// allowsAutomaticUpdate is deliberately closed. These states say GitHub can
// merge the PR content mechanically; UNSTABLE differs from CLEAN only because
// an informational check is failing. DIRTY and unknown/future states require a
// human or a separately reviewed policy change.
func allowsAutomaticUpdate(state string) bool {
	switch state {
	case mergeStateBehind, mergeStateClean, mergeStateUnstable:
		return true
	default:
		return false
	}
}

// fetchLiveBaseSnapshot resolves the PR's base branch name and then resolves
// that repository ref independently. PR baseRefOid and mergeStateStatus are
// intentionally not used: both can describe the PR's last synchronized view
// rather than the current branch tip.
func fetchLiveBaseSnapshot(ctx context.Context, prNum int, repo string) (liveBaseSnapshot, error) {
	pr, err := fetchPRRefSnapshot(ctx, prNum, repo)
	if err != nil {
		return liveBaseSnapshot{}, err
	}
	repoPath, err := escapedRepoPath(repo)
	if err != nil {
		return liveBaseSnapshot{}, err
	}
	expectedRef := "refs/heads/" + pr.BaseRefName
	out, err := runCommand(exec.CommandContext(ctx, "gh", "api",
		liveRefEndpoint(repoPath, pr.BaseRefName)))
	if err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("resolving live %s ref: %w", pr.BaseRefName, err)
	}
	oid, err := parseLiveRefOID(out, expectedRef)
	if err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("parsing live %s ref: %w", pr.BaseRefName, err)
	}
	return liveBaseSnapshot{Branch: pr.BaseRefName, OID: oid, HeadOID: pr.HeadRefOID}, nil
}

// compareLiveBaseToHead asks GitHub for an explicit merge-base witness. A
// coarse PR mergeStateStatus can be UNSTABLE because of an advisory check even
// while the base advanced, so it cannot prove ancestry.
func compareLiveBaseToHead(ctx context.Context, repo, baseOID, headOID string) (bool, string, error) {
	if !validGitOID(baseOID) || !validGitOID(headOID) {
		return false, "", fmt.Errorf("canonical base and head OIDs are required for ancestry proof")
	}
	repoPath, err := escapedRepoPath(repo)
	if err != nil {
		return false, "", err
	}
	out, err := runCommand(exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/compare/%s...%s", repoPath, url.PathEscape(baseOID), url.PathEscape(headOID))))
	if err != nil {
		return false, "", err
	}
	result, err := parseCompareResult(out, baseOID)
	if err != nil {
		return false, "", err
	}
	return interpretCompareResult(result, baseOID)
}

func parseCompareResult(out []byte, expectedBaseOID string) (compareResult, error) {
	var result compareResult
	if err := json.Unmarshal(out, &result); err != nil {
		return compareResult{}, err
	}
	if result.Status == "" || result.BaseCommit.SHA == "" || result.MergeBaseCommit.SHA == "" {
		return compareResult{}, fmt.Errorf("compare response omitted status, base_commit.sha, or merge_base_commit.sha")
	}
	if !validGitOID(result.BaseCommit.SHA) || !validGitOID(result.MergeBaseCommit.SHA) {
		return compareResult{}, fmt.Errorf("compare response reported a non-canonical base_commit.sha or merge_base_commit.sha")
	}
	if result.BaseCommit.SHA != expectedBaseOID {
		return compareResult{}, fmt.Errorf("compare response resolved base_commit.sha=%s, want requested live base %s",
			result.BaseCommit.SHA, expectedBaseOID)
	}
	return result, nil
}

func interpretCompareResult(result compareResult, baseOID string) (bool, string, error) {
	containsBase := result.MergeBaseCommit.SHA == baseOID
	switch result.Status {
	case compareAhead, compareIdentical:
		if !containsBase {
			return false, "", fmt.Errorf("compare response is inconsistent: status %q but merge base %s is not live base %s",
				result.Status, result.MergeBaseCommit.SHA, baseOID)
		}
		return true, result.Status, nil
	case compareBehind, compareDiverged:
		if containsBase {
			return false, "", fmt.Errorf("compare response is inconsistent: status %q but merge base equals live base %s",
				result.Status, baseOID)
		}
		return false, result.Status, nil
	default:
		return false, "", fmt.Errorf("compare response reported unknown status %q", result.Status)
	}
}

// updatePRBranch asks the provider to advance the PR head to the base tip. It
// is anchored to expectedHeadSHA so a branch that moved under us since the
// gates ran is rejected by the provider (422) rather than silently rewritten —
// the same TOCTOU anchor BuildMergeArgs uses for the merge itself. A successful
// response means the request was accepted; the next attempt proves completion.
func updatePRBranch(ctx context.Context, prNum int, repo, expectedHeadSHA string) error {
	if expectedHeadSHA == "" {
		return fmt.Errorf("updatePRBranch: expectedHeadSHA must not be empty — TOCTOU anchor required")
	}
	repoPath, err := escapedRepoPath(repo)
	if err != nil {
		return err
	}
	_, err = runCommand(exec.CommandContext(ctx, "gh", "api", "-X", "PUT",
		fmt.Sprintf("repos/%s/pulls/%d/update-branch", repoPath, prNum),
		"-f", "expected_head_sha="+expectedHeadSHA))
	return err
}

// checkBranchFreshness returns the exact stable live-base snapshot contained
// by the head. A DIRTY branch is never touched: conflict resolution changes
// content, which is a decision the gates cannot make. A stale branch is
// eligible for the provider's update operation only when its merge state is
// explicitly allowlisted as mechanically mergeable.
func checkBranchFreshness(
	ctx context.Context,
	prNum int,
	repo string,
	expectedBase string,
	headSHA string,
	dryRun bool,
) (liveBaseSnapshot, error) {
	before, err := fetchLiveBaseSnapshot(ctx, prNum, repo)
	if err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("cannot resolve live base before ancestry proof: %w", err)
	}
	if before.HeadOID != headSHA {
		return liveBaseSnapshot{}, fmt.Errorf("PR #%d head changed before freshness proof (expected %s, got %s); retry",
			prNum, headSHA, before.HeadOID)
	}
	if expectedBase != "" && before.Branch != expectedBase {
		return liveBaseSnapshot{}, fmt.Errorf("PR #%d base changed before freshness proof (expected %s, got %s); retry",
			prNum, expectedBase, before.Branch)
	}
	containsBase, status, err := compareLiveBaseToHead(ctx, repo, before.OID, before.HeadOID)
	if err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("cannot prove PR #%d head %s contains live base %s@%s: %w",
			prNum, headSHA, before.Branch, before.OID, err)
	}
	after, err := fetchLiveBaseSnapshot(ctx, prNum, repo)
	if err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("cannot re-resolve live base after ancestry proof: %w", err)
	}
	if before != after {
		return liveBaseSnapshot{}, fmt.Errorf("PR #%d base or head changed during freshness proof "+
			"(base %s@%s -> %s@%s; head %s -> %s); retry",
			prNum, before.Branch, before.OID, after.Branch, after.OID, before.HeadOID, after.HeadOID)
	}
	if containsBase {
		return before, nil
	}

	stale := fmt.Sprintf("PR #%d head %s does not contain live base %s@%s (compare status %s)",
		prNum, headSHA, before.Branch, before.OID, status)
	mergeState, err := prMergeStateStatus(ctx, prNum, repo)
	if err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("%s; cannot read provider merge state needed to choose a safe remediation: %w", stale, err)
	}
	if mergeState == mergeStateDirty {
		return liveBaseSnapshot{}, fmt.Errorf("%s and conflicts with its base branch; resolve the conflict and push — "+
			"safe-merge will not rewrite branch content", stale)
	}
	if dryRun {
		if allowsAutomaticUpdate(mergeState) {
			return liveBaseSnapshot{}, fmt.Errorf("%s; a real run would request an update to the live base tip and wait for the re-run", stale)
		}
		return liveBaseSnapshot{}, fmt.Errorf("%s; provider merge state %s does not authorize an automatic update — update or rebase the branch and retry",
			stale, mergeState)
	}
	if !allowsAutomaticUpdate(mergeState) {
		return liveBaseSnapshot{}, fmt.Errorf("%s; provider merge state %s does not authorize an automatic update — update or rebase the branch and retry",
			stale, mergeState)
	}
	if err := updatePRBranch(ctx, prNum, repo, headSHA); err != nil {
		return liveBaseSnapshot{}, fmt.Errorf("%s and the branch update failed (head or base may have moved since the gates ran): %w",
			stale, err)
	}
	return liveBaseSnapshot{}, ErrBranchUpdateRequested
}
