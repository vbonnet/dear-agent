package ops

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

// Classification is the verdict for one worktree. Exactly one of these is
// assigned per worktree, and ONLY ClassMerged is ever auto-removed in
// execute mode — every other class is reported and kept. This allowlist
// stance (delete only what positively proves reapable) is the lesson of the
// 2026-05-{01,15,17} sprawl retros: a denylist sweep eventually eats
// unmerged work.
type Classification string

const (
	// ClassActive means a live AGM/tmux session (or a running session's
	// nested sandbox sub-worktree, or the sweep's own worktree) owns it.
	ClassActive Classification = "ACTIVE"
	// ClassMerged means provably landed — branch is an ancestor of the base
	// ref (fast-forward / merge-commit) OR its PR state is MERGED (squash).
	// The only reapable class.
	ClassMerged Classification = "MERGED"
	// ClassDirty means uncommitted or untracked changes are present. Never
	// reaped — a clean reap must never destroy work never committed.
	ClassDirty Classification = "DIRTY"
	// ClassOrphaned means clean, not active, but NOT provably merged — a
	// husk (no PR / open PR / closed-unmerged / unpushed commits). Kept and
	// surfaced; the Reason distinguishes the sub-cases (unpushed-commits is
	// real, unrecoverable data-loss risk).
	ClassOrphaned Classification = "ORPHANED"
	// ClassUnknown means safety could not be positively established (status,
	// ancestor, base-ref, or main-repo probe failed; detached HEAD). Kept,
	// fail-safe.
	ClassUnknown Classification = "UNKNOWN"
)

// Reapable reports whether execute-mode is allowed to remove this class.
func (c Classification) Reapable() bool { return c == ClassMerged }

// DiscoveredWorktree is one linked worktree found beneath the sweep base.
type DiscoveredWorktree struct {
	Path   string // absolute worktree path
	Repo   string // owning repo (the <repo> segment of ~/worktrees/<repo>/<name>)
	Branch string // checked-out branch ("" if detached)
}

// WorktreeStatus is the fully-classified record for one worktree — the row
// the dry-run table and the JSON artifact are built from.
type WorktreeStatus struct {
	Path       string         `json:"path"`
	Repo       string         `json:"repo"`
	Branch     string         `json:"branch"`
	MainRepo   string         `json:"main_repo,omitempty"`
	LastCommit time.Time      `json:"last_commit,omitempty"`
	Subject    string         `json:"subject,omitempty"`
	Dirty      bool           `json:"dirty"`
	Pushed     bool           `json:"pushed"`
	PRState    string         `json:"pr_state,omitempty"`
	Class      Classification `json:"class"`
	Reason     string         `json:"reason"`
	// DupGroup is set when two or more worktrees in the same repo share an
	// identical HEAD commit subject (the parallel-agent fan-out
	// fingerprint). DupCount is that group's size. Report-only — dedup
	// never by itself authorizes a reap.
	DupGroup string `json:"dup_group,omitempty"`
	DupCount int    `json:"dup_count,omitempty"`
}

// SweepDeps abstracts every filesystem/git/gh interaction the sweep needs so
// the classification matrix can be unit-tested without a real repo. It
// mirrors cleanup.ReaperGitOps in spirit (allowlist, fail-safe), widened to
// the cross-repo, filesystem-discovered case the on-Stop reaper structurally
// cannot cover.
type SweepDeps interface {
	// Discover walks base (~/worktrees) and returns the linked,
	// git-registered worktrees beneath it (main checkouts excluded).
	Discover(base string) ([]DiscoveredWorktree, error)
	IsDirty(worktreePath string) (bool, error)
	LastCommit(worktreePath string) (when time.Time, subject string, err error)
	MainRepo(worktreePath string) (string, error)
	BaseRef(repoPath string) string
	IsAncestor(repoPath, branch, base string) (bool, error)
	// PRState returns "MERGED"/"OPEN"/"CLOSED" with known=false when it
	// cannot be positively established.
	PRState(repoPath, branch string) (state string, known bool)
	HasUnpushedCommits(worktreePath, branch string) (bool, error)
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	DeleteBranch(repoPath, branch string, force bool) error
}

// SweepOptions configures one sweep pass.
type SweepOptions struct {
	// WorktreesBase is the directory to scan (default ~/worktrees).
	WorktreesBase string
	// Execute removes ClassMerged worktrees. False (the zero value) is
	// dry-run: classify and report, mutate nothing.
	Execute bool
	// CheckPR enables the gh PR-state oracle (the squash-merge escape).
	// When false the sweep falls back to the strictly-local ancestor test,
	// so squash-merged husks are conservatively kept.
	CheckPR bool
	// ActiveSessions is the set of live session names AND/OR branches; a
	// worktree whose dir basename or branch is in this set is ACTIVE and
	// never touched. Supplied by the caller (the command layer queries
	// Dolt/tmux) so this package stays free of those deps.
	ActiveSessions map[string]bool
	// SelfPath, when set, is the worktree the sweep itself runs in — always
	// ACTIVE, never reaped (a fresh sweep worktree can look clean+merged).
	SelfPath string
}

// SweepResult is the outcome of a pass.
type SweepResult struct {
	Worktrees []WorktreeStatus  `json:"worktrees"`
	Removed   []string          `json:"removed"`
	Failed    map[string]string `json:"failed,omitempty"`
}

// Counts tallies worktrees by class for the summary line.
func (r *SweepResult) Counts() map[Classification]int {
	m := map[Classification]int{}
	for _, w := range r.Worktrees {
		m[w.Class]++
	}
	return m
}

// nestedWorktreeMarker identifies a running session's sandbox sub-worktree
// (…/<name>/.worktrees/<uuid>); it belongs to a live parent and is ACTIVE.
const nestedWorktreeMarker = "/.worktrees/"

// Sweep discovers every worktree under opts.WorktreesBase, classifies each,
// and (only when opts.Execute) removes the ones that are provably MERGED and
// clean, deleting their now-orphaned local branch too. It is best-effort and
// non-fatal: a failure on one worktree never aborts the others, and any
// worktree whose safety cannot be positively established is kept.
func Sweep(opts SweepOptions, deps SweepDeps, logger *slog.Logger) (*SweepResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	res := &SweepResult{Failed: map[string]string{}}

	discovered, err := deps.Discover(opts.WorktreesBase)
	if err != nil {
		return nil, err
	}

	selfPath := cleanPath(opts.SelfPath)
	for _, dw := range discovered {
		res.Worktrees = append(res.Worktrees, classify(opts, deps, dw, selfPath))
	}

	annotateDuplicates(res.Worktrees)
	sort.Slice(res.Worktrees, func(i, j int) bool {
		if res.Worktrees[i].Repo != res.Worktrees[j].Repo {
			return res.Worktrees[i].Repo < res.Worktrees[j].Repo
		}
		return res.Worktrees[i].Path < res.Worktrees[j].Path
	})

	if !opts.Execute {
		for i := range res.Worktrees {
			if res.Worktrees[i].Class.Reapable() {
				res.Removed = append(res.Removed, res.Worktrees[i].Path)
			}
		}
		return res, nil
	}

	for i := range res.Worktrees {
		w := &res.Worktrees[i]
		if !w.Class.Reapable() {
			continue
		}
		// Non-force on purpose: git refuses to remove a dirty or locked
		// worktree without --force, the last line of defense if a status
		// probe raced.
		if err := deps.RemoveWorktree(w.MainRepo, w.Path, false); err != nil {
			res.Failed[w.Path] = err.Error()
			logger.Warn("sweep: remove failed; keeping", "path", w.Path, "error", err)
			continue
		}
		// Worktree (the sprawl artifact) is gone. Delete the orphaned local
		// branch best-effort with -D: a squash-merged branch is one git
		// still considers unmerged, and a delete failure must not undo or
		// mask the (already safe) worktree removal.
		if err := deps.DeleteBranch(w.MainRepo, w.Branch, true); err != nil {
			logger.Warn("sweep: worktree removed but local branch delete failed (left dangling)",
				"path", w.Path, "branch", w.Branch, "error", err)
		}
		res.Removed = append(res.Removed, w.Path)
		logger.Info("sweep: removed", "path", w.Path, "branch", w.Branch, "via", w.Reason)
	}
	return res, nil
}

func setClass(st WorktreeStatus, c Classification, reason string) WorktreeStatus {
	st.Class, st.Reason = c, reason
	return st
}

// ownershipClass returns a terminal class (and ok=true) when a worktree must
// not even be evaluated for merge: it is owned by something live, or it is a
// detached HEAD with no branch to reason about. ok=false means "keep going".
func ownershipClass(opts SweepOptions, dw DiscoveredWorktree, selfPath string) (Classification, string, bool) {
	switch {
	case selfPath != "" && cleanPath(dw.Path) == selfPath:
		return ClassActive, "self-worktree", true
	case strings.Contains(dw.Path, nestedWorktreeMarker):
		return ClassActive, "nested-sandbox-worktree", true
	case opts.ActiveSessions[filepath.Base(dw.Path)]:
		return ClassActive, "live-session", true
	case dw.Branch != "" && opts.ActiveSessions[dw.Branch]:
		return ClassActive, "live-session", true
	case dw.Branch == "":
		return ClassUnknown, "detached-head", true
	}
	return "", "", false
}

// resolvePRState collapses the gh oracle to a single display/decision string:
// the real state when known, "UNKNOWN" when gh could not answer, or
// "(pr-check-off)" when the oracle is disabled.
func resolvePRState(opts SweepOptions, deps SweepDeps, mainRepo, branch string) string {
	if !opts.CheckPR {
		return "(pr-check-off)"
	}
	if state, known := deps.PRState(mainRepo, branch); known {
		return state
	}
	return "UNKNOWN"
}

// huskReason names the husk sub-case for a clean, not-active, not-merged
// worktree. unpushed-commits (commits on no remote) is real, unrecoverable
// data-loss risk and outranks any PR state.
func huskReason(unpushed bool, prState string) string {
	switch {
	case unpushed:
		return "unpushed-commits"
	case prState == "OPEN":
		return "open-pr-unmerged"
	case prState == "CLOSED":
		return "pr-closed-unmerged"
	default:
		// No PR, or PR state could not be established: either way a
		// pushed-but-unmerged husk — kept, flagged for a human.
		return "no-pr"
	}
}

// classify decides one worktree's fate without mutating anything. Precedence
// is strictly fail-safe: every uncertain or unprovable state resolves to a
// kept class, and ClassMerged is reached ONLY through a positive
// ancestor-of-base or PR-MERGED proof on a clean tree.
func classify(opts SweepOptions, deps SweepDeps, dw DiscoveredWorktree, selfPath string) WorktreeStatus {
	st := WorktreeStatus{Path: dw.Path, Repo: dw.Repo, Branch: dw.Branch}

	// Display-only; a failure here never changes the verdict.
	if when, subj, err := deps.LastCommit(dw.Path); err == nil {
		st.LastCommit, st.Subject = when, subj
	}

	if c, reason, ok := ownershipClass(opts, dw, selfPath); ok {
		return setClass(st, c, reason)
	}

	dirty, err := deps.IsDirty(dw.Path)
	if err != nil {
		return setClass(st, ClassUnknown, "status-check-failed")
	}
	st.Dirty = dirty
	if dirty {
		return setClass(st, ClassDirty, "uncommitted-changes")
	}

	mainRepo, err := deps.MainRepo(dw.Path)
	if err != nil {
		return setClass(st, ClassUnknown, "main-repo-unresolved")
	}
	st.MainRepo = mainRepo

	base := deps.BaseRef(mainRepo)
	if base == "" {
		return setClass(st, ClassUnknown, "base-ref-unresolved")
	}

	// Squash-SAFE merge oracle, part 1: fast-forward / merge-commit landing.
	anc, err := deps.IsAncestor(mainRepo, dw.Branch, base)
	if err != nil {
		return setClass(st, ClassUnknown, "ancestor-check-failed")
	}
	if anc {
		st.Pushed, st.PRState = true, "(ancestor)"
		return setClass(st, ClassMerged, "ancestor-of-base")
	}

	// Part 2: squash merge — ahead/behind is meaningless, the PR head-ref
	// state is the only authoritative signal (the core retro finding).
	st.PRState = resolvePRState(opts, deps, mainRepo, dw.Branch)
	if st.PRState == "MERGED" {
		st.Pushed = true
		return setClass(st, ClassMerged, "pr-merged")
	}

	// Clean, not active, not provably merged ⇒ husk.
	unpushed, err := deps.HasUnpushedCommits(dw.Path, dw.Branch)
	if err != nil {
		return setClass(st, ClassUnknown, "push-check-failed")
	}
	st.Pushed = !unpushed
	return setClass(st, ClassOrphaned, huskReason(unpushed, st.PRState))
}

// annotateDuplicates flags worktrees that share an identical HEAD commit
// subject within the same repo — the one-task-many-worktrees fan-out
// fingerprint from the retro. Report-only: it tags rows so a human (or a
// follow-up) can see the fan-out; it never changes a Class or authorizes a
// reap on its own.
func annotateDuplicates(wts []WorktreeStatus) {
	type key struct{ repo, subject string }
	groups := map[key][]int{}
	for i, w := range wts {
		if w.Subject == "" {
			continue
		}
		k := key{w.Repo, w.Subject}
		groups[k] = append(groups[k], i)
	}
	for k, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		for _, i := range idxs {
			wts[i].DupGroup = k.subject
			wts[i].DupCount = len(idxs)
		}
	}
}

func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// underBase reports whether path is located at or beneath base.
func underBase(path, base string) bool {
	base = cleanPath(base)
	if base == "" {
		return true
	}
	rel, err := filepath.Rel(base, cleanPath(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RealSweepDeps implements SweepDeps against the real filesystem + git CLI
// + gh. All git/gh logic delegates to the shared internal/git helpers so the
// sweep, the on-Stop reaper, and audit-resources cannot drift apart.
type RealSweepDeps struct{}

// Discover walks base for ~/worktrees/<repo>/<name> dirs and resolves each
// repo's linked worktrees via git (main checkouts and anything not physically
// under base excluded). Worktree dirs that git no longer registers are
// skipped — those are `agm audit resources --fix` / prune territory, not the
// sweep's (it only acts on live registered worktrees).
func (RealSweepDeps) Discover(base string) ([]DiscoveredWorktree, error) {
	base = cleanPath(base)
	// git reports canonical (symlink-resolved) worktree paths, so the
	// underBase location allowlist only holds if base is resolved the same
	// way (~/worktrees, or macOS /var→/private/var, may be a symlink).
	if resolved, rerr := filepath.EvalSymlinks(base); rerr == nil {
		base = resolved
	}
	repoEntries, err := os.ReadDir(base)
	if err != nil {
		// Missing base ⇒ nothing to sweep, not an error.
		return nil, nil //nolint:nilerr // absence is the empty result, by design
	}

	var out []DiscoveredWorktree
	seen := map[string]bool{}
	for _, repoEnt := range repoEntries {
		if !repoEnt.IsDir() {
			continue
		}
		repoName := repoEnt.Name()
		repoDir := filepath.Join(base, repoName)

		// One ListWorktrees per repo dir is enough — it enumerates every
		// linked worktree of that repo. Probe via the first child that is
		// itself a git worktree.
		children, err := os.ReadDir(repoDir)
		if err != nil {
			continue
		}
		for _, ch := range children {
			if !ch.IsDir() {
				continue
			}
			probe := filepath.Join(repoDir, ch.Name())
			wts, err := gitpkg.ListWorktrees(probe)
			if err != nil || wts == nil {
				continue
			}
			for _, wt := range wts {
				if wt.IsMain || seen[wt.Path] || !underBase(wt.Path, base) {
					continue
				}
				seen[wt.Path] = true
				out = append(out, DiscoveredWorktree{
					Path:   wt.Path,
					Repo:   repoName,
					Branch: wt.Branch,
				})
			}
			break // repo fully enumerated from this one probe
		}
	}
	return out, nil
}

// IsDirty reports whether the worktree has uncommitted/untracked changes.
func (RealSweepDeps) IsDirty(worktreePath string) (bool, error) {
	return gitpkg.HasUncommittedChanges(worktreePath)
}

// LastCommit returns the worktree HEAD's committer date and subject.
func (RealSweepDeps) LastCommit(worktreePath string) (time.Time, string, error) {
	return gitpkg.LastCommitInfo(worktreePath)
}

// MainRepo returns the primary checkout the linked worktree belongs to.
func (RealSweepDeps) MainRepo(worktreePath string) (string, error) {
	return gitpkg.MainWorktreePath(worktreePath)
}

// BaseRef resolves the ref a branch's merged-ness is measured against.
func (RealSweepDeps) BaseRef(repoPath string) string {
	return gitpkg.ResolveBaseRef(repoPath)
}

// IsAncestor reports whether branch's tip is reachable from base.
func (RealSweepDeps) IsAncestor(repoPath, branch, base string) (bool, error) {
	return gitpkg.IsAncestor(repoPath, branch, base)
}

// PRState returns the gh PR state for branch (known=false ⇒ do not act).
func (RealSweepDeps) PRState(repoPath, branch string) (string, bool) {
	return gitpkg.PRState(repoPath, branch)
}

// HasUnpushedCommits reports whether branch has commits on no remote ref.
func (RealSweepDeps) HasUnpushedCommits(worktreePath, branch string) (bool, error) {
	return gitpkg.HasUnpushedCommits(worktreePath, branch)
}

// RemoveWorktree removes the worktree (non-force lets git guard a dirty tree).
func (RealSweepDeps) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	return gitpkg.RemoveWorktree(repoPath, worktreePath, force)
}

// DeleteBranch deletes the now-orphaned local branch of a reaped worktree.
func (RealSweepDeps) DeleteBranch(repoPath, branch string, force bool) error {
	return gitpkg.DeleteBranch(repoPath, branch, force)
}
