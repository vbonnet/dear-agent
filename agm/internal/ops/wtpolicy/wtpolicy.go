// Package wtpolicy holds the reusable worktree-safety policy: the
// squash-safe merge oracle, the dirty-tree check, and the
// "session is waiting for the user" transcript heuristic.
//
// These are the predicates that decide whether a worktree's session is
// safe to retire. They are deliberately split out of `agm worktree sweep`
// so any command that retires sessions — the sweep today, `agm session
// archive-ui` tomorrow — applies exactly the same rules. Drift between two
// copies of "is this merged?" is precisely the class of bug the
// 2026-05-{01,15,17} sprawl retros keep re-finding.
//
// Every function here is fail-safe: an unprovable answer resolves to the
// conservative side (not-merged / dirty-unknown / not-awaiting-but-kept),
// never to "safe to delete".
package wtpolicy

// GitProbe is the read-only git/gh surface the merge and dirty oracles
// need. It is the exact subset of the sweep's dependency interface that
// concerns safety, so callers can pass their existing deps object
// unchanged. Every method must be fail-safe (see each contract).
type GitProbe interface {
	// IsDirty reports whether the worktree has uncommitted or untracked
	// changes. On any error it MUST return (true, err) so an unprovable
	// status is treated as dirty.
	IsDirty(worktreePath string) (bool, error)
	// IsAncestor reports whether branch's tip is reachable from base. On
	// any error it MUST return (false, err) so unprovable ancestry is
	// treated as "not merged".
	IsAncestor(repoPath, branch, base string) (bool, error)
	// PRState returns "MERGED"/"OPEN"/"CLOSED"; known=false whenever the
	// state cannot be positively established (gh missing, no PR, ambiguous
	// head, timeout, …). Callers MUST treat known=false as "do not act".
	PRState(repoPath, branch string) (state string, known bool)
}

// Dirty reports whether a worktree has uncommitted/untracked changes.
// known=false means the status could not be determined — callers MUST
// treat that as "not safe to retire" exactly like dirty=true. (The
// underlying probe already returns dirty=true on error; the explicit
// known flag lets callers label "status-check-failed" distinctly from a
// genuinely dirty tree.)
func Dirty(g GitProbe, worktreePath string) (dirty bool, known bool) {
	d, err := g.IsDirty(worktreePath)
	if err != nil {
		return true, false
	}
	return d, true
}

// MergeVerdict is the squash-safe merge-oracle result.
type MergeVerdict struct {
	// Merged is true only when a merge was POSITIVELY proven.
	Merged bool
	// Reason is "ancestor-of-base" or "pr-merged" when Merged, else "".
	Reason string
	// PRState is the resolved display state: the real gh state, "UNKNOWN"
	// when the oracle ran but could not establish it, or "(pr-check-off)"
	// / "(ancestor)" sentinels.
	PRState string
	// Pushed is true when the merge proof also implies the branch reached
	// the remote (ancestor-of-base or a MERGED PR).
	Pushed bool
	// AncestorErr is non-nil when the ancestry probe itself failed; the
	// verdict is then conservatively not-merged and the caller should
	// surface an "indeterminate" state rather than reaping.
	AncestorErr error
}

// ProvablyMerged is THE merge oracle. It never uses ahead/behind (a
// squash-merge inflates that to "ahead>0", the bug that lets husks
// accumulate). A worktree's branch is merged only when one of two
// positive proofs holds:
//
//  1. branch is an ancestor of base — a fast-forward / merge-commit
//     landing (squash-safe: this is false for a squashed branch, which is
//     why proof 2 exists);
//  2. checkPR is set and gh authoritatively reports the branch's PR head
//     state == MERGED — the only signal that survives a squash.
//
// mainRepo/base must already be resolved by the caller (resolution and
// its own failure modes are caller-specific). Any failure of the ancestry
// probe is reported via AncestorErr and yields Merged=false.
func ProvablyMerged(g GitProbe, mainRepo, branch, base string, checkPR bool) MergeVerdict {
	anc, err := g.IsAncestor(mainRepo, branch, base)
	if err != nil {
		return MergeVerdict{AncestorErr: err, PRState: resolvePRState(g, mainRepo, branch, checkPR)}
	}
	if anc {
		return MergeVerdict{Merged: true, Reason: "ancestor-of-base", PRState: "(ancestor)", Pushed: true}
	}

	state := resolvePRState(g, mainRepo, branch, checkPR)
	if state == "MERGED" {
		return MergeVerdict{Merged: true, Reason: "pr-merged", PRState: state, Pushed: true}
	}
	return MergeVerdict{PRState: state}
}

// resolvePRState collapses the gh oracle to a single display/decision
// string: the real state when known, "UNKNOWN" when gh could not answer,
// or "(pr-check-off)" when the oracle is disabled.
func resolvePRState(g GitProbe, mainRepo, branch string, checkPR bool) string {
	if !checkPR {
		return "(pr-check-off)"
	}
	if state, known := g.PRState(mainRepo, branch); known {
		return state
	}
	return "UNKNOWN"
}
