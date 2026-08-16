// Command agm-worktree-audit scans every git repository under a root directory
// (default ~/src) and reports worktrees and branches that look abandoned,
// merged-but-not-cleaned, or otherwise reclaimable.
//
// It is a read-only diagnostic: it never removes a worktree or deletes a
// branch. Its job is to produce a clear report a human (or a follow-up
// cleanup task) can act on. For linked worktrees beneath its configured base,
// the dry-run-default `agm worktree sweep` can reclaim only worktrees
// positively classified as clean and merged after fail-closed active-session
// checks. After successful removal, the sweep attempts to force-delete the
// selected local branch; a failed branch deletion warns and leaves that branch
// in place. Findings outside that base remain report-only for separately
// reviewed, repository-scoped cleanup.
//
// Four finding categories are reported (see [FindingKind]):
//
//	abandoned-worktree   worktree whose HEAD has no commit within --worktree-days
//	merged-not-deleted   local branch already merged into the base ref
//	stale-unmerged       unmerged local branch untouched for >= --branch-days
//	worktree-no-remote   worktree branch with no matching origin/<branch>
//
// Usage:
//
//	agm-worktree-audit [--root DIR] [--json] [--worktree-days N] [--branch-days N]
//
// Exit codes: 0 success, 1 runtime error, 2 usage error.
package main

import (
	"sort"
	"time"
)

// FindingKind categorises a single audit finding. The string values are stable
// and used as-is in JSON output and as text-report section keys.
type FindingKind string

const (
	// KindAbandonedWorktree marks a worktree whose HEAD commit is older than
	// the worktree staleness threshold — a candidate for review/removal.
	KindAbandonedWorktree FindingKind = "abandoned-worktree"
	// KindMergedNotDeleted marks a local branch that is already merged into
	// the base ref but still exists — safe to delete once confirmed.
	KindMergedNotDeleted FindingKind = "merged-not-deleted"
	// KindStaleUnmerged marks an unmerged local branch that has not been
	// touched within the branch staleness threshold — work that may be
	// abandoned and is worth confirming before deletion.
	KindStaleUnmerged FindingKind = "stale-unmerged"
	// KindWorktreeNoRemote marks a worktree whose branch has no corresponding
	// origin/<branch> — its work exists only locally.
	KindWorktreeNoRemote FindingKind = "worktree-no-remote"
)

// Branch is the per-branch data the audit needs, already extracted from git so
// that [Categorize] can run with no I/O (and therefore be unit-tested).
type Branch struct {
	Name       string    `json:"name"`
	LastCommit time.Time `json:"last_commit"`
	// Ahead is the number of commits on the branch but not on the base ref.
	// A value < 0 signals "could not determine" and is treated conservatively
	// (the branch is never reported as merged).
	Ahead int `json:"ahead"`
	// Behind is the number of commits on the base ref but not on the branch.
	Behind int `json:"behind"`
	// Merged reports membership in `git branch --merged <base>` (true merges
	// and fast-forwards). Squash-merged branches are NOT detected here — they
	// report Merged=false with Ahead>0, by design, to avoid false positives.
	Merged bool `json:"merged"`
	// HasRemote reports whether origin/<Name> exists.
	HasRemote bool `json:"has_remote"`
	// IsBase reports whether this branch is the repo's base ref (main/master).
	IsBase bool `json:"is_base"`
	// CheckedOut reports whether the branch is checked out in some worktree.
	CheckedOut bool `json:"checked_out"`
}

// Worktree is the per-worktree data the audit needs.
type Worktree struct {
	Path       string    `json:"path"`
	Branch     string    `json:"branch"` // "" when detached
	IsMain     bool      `json:"is_main"`
	LastCommit time.Time `json:"last_commit"`
	HasRemote  bool      `json:"has_remote"` // origin/<Branch> exists
}

// RepoData is everything [Categorize] needs about one repository.
type RepoData struct {
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	BaseRef   string     `json:"base_ref"` // e.g. "origin/main"; "" if unresolved
	Branches  []Branch   `json:"branches"`
	Worktrees []Worktree `json:"worktrees"`
}

// Finding is one reportable item: a worktree or branch worth cleaning up.
type Finding struct {
	Repo       string      `json:"repo"`
	Kind       FindingKind `json:"kind"`
	Branch     string      `json:"branch"`
	Path       string      `json:"path,omitempty"` // set for worktree findings
	LastCommit time.Time   `json:"last_commit"`
	Ahead      int         `json:"ahead"`
	Behind     int         `json:"behind"`
	Merged     bool        `json:"merged"`
	// Reason is a short human-readable justification for the finding.
	Reason string `json:"reason"`
}

// Thresholds controls how old something must be before it is flagged.
type Thresholds struct {
	// WorktreeStale is the max HEAD age before a worktree is "abandoned".
	WorktreeStale time.Duration
	// BranchStale is the max age before an unmerged branch is "stale".
	BranchStale time.Duration
}

// DefaultThresholds matches the audit's documented defaults: 7-day worktrees,
// 14-day unmerged branches.
func DefaultThresholds() Thresholds {
	return Thresholds{
		WorktreeStale: 7 * 24 * time.Hour,
		BranchStale:   14 * 24 * time.Hour,
	}
}

// Categorize is the pure heart of the audit: given already-collected repo data,
// a reference "now", and the staleness thresholds, it returns the findings.
// It performs no I/O so it is fully unit-testable.
//
// Findings are sorted deterministically by (repo, kind, branch) so output is
// stable across runs.
func Categorize(repos []RepoData, now time.Time, th Thresholds) []Finding {
	var findings []Finding
	for _, repo := range repos {
		findings = append(findings, worktreeFindings(repo, now, th)...)
		findings = append(findings, branchFindings(repo, now, th)...)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Branch < b.Branch
	})
	return findings
}

// worktreeFindings reports the two worktree-level categories (abandoned,
// no-remote) for a single repo. Ahead/behind/merged are borrowed from the
// matching local branch where one exists.
func worktreeFindings(repo RepoData, now time.Time, th Thresholds) []Finding {
	branchByName := make(map[string]Branch, len(repo.Branches))
	for _, br := range repo.Branches {
		branchByName[br.Name] = br
	}
	enrich := func(f Finding, wt Worktree) Finding {
		if br, ok := branchByName[wt.Branch]; ok && wt.Branch != "" {
			f.Ahead, f.Behind, f.Merged = br.Ahead, br.Behind, br.Merged
		} else {
			f.Ahead, f.Behind = -1, -1
		}
		return f
	}

	var out []Finding
	for _, wt := range repo.Worktrees {
		if wt.IsMain {
			// The main worktree is the repo itself, not reclaimable work.
			continue
		}
		// 1. Abandoned worktree: HEAD has no recent commit.
		if !wt.LastCommit.IsZero() && now.Sub(wt.LastCommit) >= th.WorktreeStale {
			out = append(out, enrich(Finding{
				Repo:       repo.Name,
				Kind:       KindAbandonedWorktree,
				Branch:     wt.Branch,
				Path:       wt.Path,
				LastCommit: wt.LastCommit,
				Reason:     "no commit on HEAD within staleness window",
			}, wt))
		}
		// 4. Worktree with no remote branch: work is local-only. Detached
		// worktrees have no branch to mirror, so skip them.
		if wt.Branch != "" && !wt.HasRemote {
			out = append(out, enrich(Finding{
				Repo:       repo.Name,
				Kind:       KindWorktreeNoRemote,
				Branch:     wt.Branch,
				Path:       wt.Path,
				LastCommit: wt.LastCommit,
				Reason:     "no origin/" + wt.Branch + " — work exists only locally",
			}, wt))
		}
	}
	return out
}

// branchFindings reports the two branch-level categories (merged-not-deleted,
// stale-unmerged) for a single repo. The base branch is never flagged, and a
// merged branch is reported as merged rather than stale.
func branchFindings(repo RepoData, now time.Time, th Thresholds) []Finding {
	var out []Finding
	for _, br := range repo.Branches {
		switch {
		case br.IsBase:
			// never flag the base branch itself
		case br.Merged:
			// 2. Merged but not deleted (takes precedence over stale).
			out = append(out, Finding{
				Repo:       repo.Name,
				Kind:       KindMergedNotDeleted,
				Branch:     br.Name,
				LastCommit: br.LastCommit,
				Ahead:      br.Ahead,
				Behind:     br.Behind,
				Merged:     true,
				Reason:     "merged into " + repo.BaseRef + " but not deleted",
			})
		case !br.LastCommit.IsZero() && now.Sub(br.LastCommit) >= th.BranchStale:
			// 3. Stale unmerged: not merged and untouched past the window.
			out = append(out, Finding{
				Repo:       repo.Name,
				Kind:       KindStaleUnmerged,
				Branch:     br.Name,
				LastCommit: br.LastCommit,
				Ahead:      br.Ahead,
				Behind:     br.Behind,
				Merged:     false,
				Reason:     "unmerged and untouched past staleness window",
			})
		}
	}
	return out
}
