// Package safegit freshness.go implements the base-freshness gate.
//
// Under a strict required-status-checks policy the provider refuses to merge a
// PR whose head is not built on the current base tip: a green check proves
// something about the base the branch was cut from, not about the tip it would
// land on. Every merge path in this repo therefore needs the branch advanced to
// the base tip before the merge can be accepted — and nothing else in the
// pipeline does that. `gh pr merge --auto` only queues the merge; it never
// advances the branch, so an out-of-date PR sits queued forever.
//
// This gate closes that hole. It runs last, after CI, threads, soak, and the
// expected-reviewer gates have all passed, so a CI cycle is only ever spent on
// a PR that already met the full merge bar.
package safegit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
)

// Provider merge-state values this gate reacts to. Any other value (CLEAN,
// BLOCKED, UNSTABLE, …) is left to the gates that own it.
const (
	mergeStateBehind = "BEHIND"
	mergeStateDirty  = "DIRTY"
)

// ErrBranchUpdated reports that the gate advanced the head to the base tip.
// It is a blocking outcome, not a failure: the merge cannot proceed on this
// attempt because the push invalidated the checks that had just passed. Watch
// mode retries and merges once the re-run goes green.
var ErrBranchUpdated = errors.New("branch updated to base tip; required checks are re-running")

// freshnessAction is what the gate does about a given provider merge state.
type freshnessAction uint8

const (
	// freshnessProceed: the head is on the base tip, or the reason it is not
	// mergeable belongs to a gate that already ran. Either way, not this
	// gate's call to make.
	freshnessProceed freshnessAction = iota
	// freshnessUpdate: the head trails the base tip and can be advanced
	// mechanically, with no content decision.
	freshnessUpdate
	// freshnessConflict: advancing would mean resolving conflicting content,
	// which is a decision no gate is entitled to make.
	freshnessConflict
)

// classifyFreshness maps a provider merge state to this gate's action. Unknown
// and future states proceed: this gate owns base freshness only, and must not
// become a second opinion on checks or reviews.
func classifyFreshness(state string) freshnessAction {
	switch state {
	case mergeStateBehind:
		return freshnessUpdate
	case mergeStateDirty:
		return freshnessConflict
	default:
		return freshnessProceed
	}
}

// parseMergeStateStatus extracts the provider's mergeability verdict from a
// `gh pr view --json mergeStateStatus` payload. An absent verdict is an error
// rather than an empty string, because classifyFreshness reads "" as proceed —
// a silently-empty parse would wave through the branch this gate exists to catch.
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

// prMergeStateStatus reads the provider's mergeability verdict for a PR.
func prMergeStateStatus(prNum int, repo string) (string, error) {
	out, err := runCommand(exec.Command("gh", "pr", "view",
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

// updatePRBranch advances the PR head to the base tip. It is anchored to
// expectedHeadSHA so a branch that moved under us since the gates ran is
// rejected by the provider (422) rather than silently rewritten — the same
// TOCTOU anchor BuildMergeArgs uses for the merge itself.
func updatePRBranch(prNum int, repo, expectedHeadSHA string) error {
	if expectedHeadSHA == "" {
		return fmt.Errorf("updatePRBranch: expectedHeadSHA must not be empty — TOCTOU anchor required")
	}
	_, err := runCommand(exec.Command("gh", "api", "-X", "PUT",
		fmt.Sprintf("repos/%s/pulls/%d/update-branch", repo, prNum),
		"-f", "expected_head_sha="+expectedHeadSHA))
	return err
}

// checkBranchFreshness blocks the merge unless the head is built on the current
// base tip, advancing the branch itself when it can do so safely.
//
// A DIRTY branch is never touched: conflict resolution changes content, which
// is a decision the gates cannot make. A BEHIND branch is advanced, because
// that is a mechanical fast-forward of the base with no content decision.
func checkBranchFreshness(prNum int, repo, headSHA string, dryRun bool) error {
	state, err := prMergeStateStatus(prNum, repo)
	if err != nil {
		return fmt.Errorf("cannot read merge state (needed to prove the head is on the base tip): %w", err)
	}

	switch classifyFreshness(state) {
	case freshnessConflict:
		return fmt.Errorf("PR #%d conflicts with its base branch; resolve the conflict and push — "+
			"safe-merge will not rewrite branch content", prNum)

	case freshnessUpdate:
		if dryRun {
			return fmt.Errorf("PR #%d is behind its base branch; a real run would advance it to the base tip "+
				"and wait for the re-run", prNum)
		}
		if err := updatePRBranch(prNum, repo, headSHA); err != nil {
			return fmt.Errorf("PR #%d is behind its base branch and the branch update failed "+
				"(head may have moved since the gates ran): %w", prNum, err)
		}
		return ErrBranchUpdated

	case freshnessProceed:
		return nil

	default:
		return nil
	}
}
