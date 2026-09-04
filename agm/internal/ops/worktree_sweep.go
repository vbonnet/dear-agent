package ops

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/ops/wtpolicy"
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
	// ClassAwaitingInput means the session's transcript ends with the
	// assistant waiting on the user (an AskUserQuestion tool call, or a
	// trailing question / question phrase). Kept and surfaced even if it
	// would otherwise look merged/clean — the user must be able to return
	// and answer. Outranks the merge/dirty oracles.
	ClassAwaitingInput Classification = "AWAITING_INPUT"
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
	Path       string // absolute worktree path
	Repo       string // owning repo (the <repo> segment of ~/worktrees/<repo>/<name>)
	Branch     string // checked-out branch ("" if detached)
	Locked     bool   // git worktree lock state
	LockReason string // lock reason, if locked
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
	Locked     bool           `json:"locked,omitempty"`
	LockReason string         `json:"lock_reason,omitempty"`
	// DupGroup is set when two or more worktrees in the same repo share an
	// identical HEAD commit subject (the parallel-agent fan-out
	// fingerprint). DupCount is that group's size. Report-only — dedup
	// never by itself authorizes a reap.
	DupGroup string `json:"dup_group,omitempty"`
	DupCount int    `json:"dup_count,omitempty"`
	// CommitsAboveMergeBase counts commits on branch above git merge-base
	// with the main base ref. Only populated for ClassOrphaned/no-pr.
	CommitsAboveMergeBase int `json:"commits_above_merge_base,omitempty"`
	// IsOrphanBranch is true when the branch has commits above the main
	// merge-base but no open or merged PR — abandoned in-flight work that
	// needs human attention.
	IsOrphanBranch bool `json:"is_orphan_branch,omitempty"`
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
	// CommitsAboveMergeBase counts commits on branch above base
	// (git rev-list --count base..branch). Returns -1 on error — callers
	// treat a negative count as "has unmerged work" to stay fail-safe.
	CommitsAboveMergeBase(repoPath, branch, base string) (int, error)
	// AwaitingInput reports whether the session that ran in worktreePath
	// ended waiting on the user (detail is a short tag for the report).
	// Positive only — see wtpolicy.AwaitingInput for the fail-safe stance.
	AwaitingInput(worktreePath string) (awaiting bool, detail string)
	UnlockWorktree(repoPath, worktreePath string) error
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	DeleteBranch(repoPath, branch string, force bool) error
}

// Compile-time proof that SweepDeps is a superset of the safety surface
// shared with archive-ui, so the shared oracles cannot drift from it.
var _ wtpolicy.GitProbe = (SweepDeps)(nil)

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
	// ActiveSessionsKnown records whether ActiveSessions is the complete
	// live set or a guess left over from a failed lookup. An executing
	// sweep refuses to run when it is false: "fewer ACTIVE matches" is not
	// conservative for a command that deletes, it is the failure mode that
	// removed two live worktrees during the 2026-07-10 audit (ce-3knl.1).
	// Dry runs still classify, so an operator can see what the sweep would
	// have done before restoring the lookup.
	ActiveSessionsKnown bool
}

// ErrActiveSessionsUnknown is returned by Sweep when --execute is requested
// without a trustworthy active-session set.
var ErrActiveSessionsUnknown = errors.New(
	"active-session discovery failed: refusing to execute a sweep that cannot " +
		"prove which worktrees are live (re-run without --execute to classify only)")

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
	// Fail closed before discovery: an executing sweep with an unknown live
	// set has no way to classify ACTIVE, so every live worktree would look
	// reapable.
	if opts.Execute && !opts.ActiveSessionsKnown {
		return nil, ErrActiveSessionsUnknown
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
		if w.Locked {
			if err := deps.UnlockWorktree(w.MainRepo, w.Path); err != nil {
				res.Failed[w.Path] = fmt.Sprintf("unlock: %v", err)
				logger.Warn("sweep: unlock failed; keeping", "path", w.Path, "error", err)
				continue
			}
		}
		// Non-force on purpose: git refuses to remove a dirty or locked
		// worktree without --force, the last line of defense if a status
		// probe raced.
		if err := deps.RemoveWorktree(w.MainRepo, w.Path, false); err != nil {
			removed := false
			if isLockedError(err) {
				if uerr := deps.UnlockWorktree(w.MainRepo, w.Path); uerr == nil {
					if rerr := deps.RemoveWorktree(w.MainRepo, w.Path, false); rerr == nil {
						removed = true
					}
				}
			}
			if !removed {
				res.Failed[w.Path] = err.Error()
				logger.Warn("sweep: remove failed; keeping", "path", w.Path, "error", err)
				continue
			}
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

// isLockActive reports whether reason names an active session or a live process.
func isLockActive(activeSessions map[string]bool, reason string) bool {
	reason = strings.TrimSpace(reason)
	if reason == "" || reason == "added with --lock" {
		return false
	}
	if rest, ok := strings.CutPrefix(reason, "safe-pr-owned:"); ok {
		parts := strings.Split(rest, ":")
		if len(parts) > 0 {
			if pid, err := strconv.Atoi(parts[0]); err == nil && pid > 0 {
				return processExists(pid)
			}
		}
		return false
	}
	sessionID := reason
	if s, ok := strings.CutPrefix(reason, "session:"); ok {
		sessionID = s
	}
	if len(activeSessions) > 0 && activeSessions[sessionID] {
		return true
	}
	return false
}

// processExists reports whether a process with the given pid is alive.
func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// isLockedError reports whether an error from git worktree remove indicates a locked worktree.
func isLockedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "unlock first")
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
	case dw.Locked && isLockActive(opts.ActiveSessions, dw.LockReason):
		return ClassActive, "locked-active-session", true
	case dw.Locked && gitpkg.IsExplicitPreserve(dw.LockReason):
		return ClassUnknown, "locked-preserved:" + dw.LockReason, true
	case dw.Branch == "":
		return ClassUnknown, "detached-head", true
	}
	return "", "", false
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
	st := WorktreeStatus{
		Path:       dw.Path,
		Repo:       dw.Repo,
		Branch:     dw.Branch,
		Locked:     dw.Locked,
		LockReason: dw.LockReason,
	}

	// Display-only; a failure here never changes the verdict.
	if when, subj, err := deps.LastCommit(dw.Path); err == nil {
		st.LastCommit, st.Subject = when, subj
	}

	if c, reason, ok := ownershipClass(opts, dw, selfPath); ok {
		return setClass(st, c, reason)
	}

	// A session blocked on a pending question outranks every reap path:
	// its worktree must survive even when it otherwise looks merged and
	// clean, so the user can come back and answer. (Checked before the
	// merge/dirty oracles, exactly as the requirement specifies — see
	// wtpolicy.AwaitingInput for the heuristic and its fail-safe stance.)
	if awaiting, detail := deps.AwaitingInput(dw.Path); awaiting {
		return setClass(st, ClassAwaitingInput, "awaiting-input:"+detail)
	}

	// Dirty-check (shared with archive-ui via wtpolicy): unknown status is
	// treated as not-safe, distinctly labeled from a genuinely dirty tree.
	dirty, known := wtpolicy.Dirty(deps, dw.Path)
	if !known {
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

	// Squash-safe merge oracle (shared with archive-ui via wtpolicy):
	// ancestor-of-base OR authoritative PR==MERGED, never ahead/behind.
	mv := wtpolicy.ProvablyMerged(deps, mainRepo, dw.Branch, base, opts.CheckPR)
	if mv.AncestorErr != nil {
		return setClass(st, ClassUnknown, "ancestor-check-failed")
	}
	st.PRState = mv.PRState
	if mv.Merged {
		st.Pushed = true
		return setClass(st, ClassMerged, mv.Reason)
	}

	// Clean, not active, not provably merged ⇒ husk.
	unpushed, err := deps.HasUnpushedCommits(dw.Path, dw.Branch)
	if err != nil {
		return setClass(st, ClassUnknown, "push-check-failed")
	}
	st.Pushed = !unpushed
	result := setClass(st, ClassOrphaned, huskReason(unpushed, st.PRState))

	// Enrich no-pr orphans: count commits above the merge-base to flag
	// branches with real in-flight work that was never opened as a PR.
	if result.Reason == "no-pr" {
		if count, countErr := deps.CommitsAboveMergeBase(mainRepo, dw.Branch, base); countErr == nil && count > 0 {
			result.CommitsAboveMergeBase = count
			result.IsOrphanBranch = true
		}
	}
	return result
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
			// Dot-prefixed children may be linked worktrees (which contain a
			// .git file) or standalone clones/husks (which contain a .git
			// directory or no git metadata). Only probe dot-prefixed children
			// if they are genuine linked worktrees.
			if strings.HasPrefix(ch.Name(), ".") {
				gitPath := filepath.Join(probe, ".git")
				fi, err := os.Stat(gitPath)
				if err != nil {
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					return nil, fmt.Errorf("stat %s: %w", gitPath, err)
				}
				if fi.IsDir() {
					continue
				}
			}
			resolvedProbe := probe
			if resolved, rerr := filepath.EvalSymlinks(probe); rerr == nil {
				resolvedProbe = resolved
			}
			if seen[resolvedProbe] {
				continue
			}
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
					Path:       wt.Path,
					Repo:       repoName,
					Branch:     wt.Branch,
					Locked:     wt.Locked,
					LockReason: wt.LockReason,
				})
			}
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

// CommitsAboveMergeBase counts commits on branch above base via git rev-list.
func (RealSweepDeps) CommitsAboveMergeBase(repoPath, branch, base string) (int, error) {
	return gitpkg.CommitsAhead(repoPath, branch, base)
}

// AwaitingInput reports whether the worktree's Claude Code transcript ends
// with the assistant waiting on the user.
func (RealSweepDeps) AwaitingInput(worktreePath string) (bool, string) {
	return wtpolicy.AwaitingInput(worktreePath)
}

// UnlockWorktree unlocks a locked worktree via gitpkg.
func (RealSweepDeps) UnlockWorktree(repoPath, worktreePath string) error {
	return gitpkg.UnlockWorktree(repoPath, worktreePath)
}

// RemoveWorktree removes the worktree (non-force lets git guard a dirty tree).
func (RealSweepDeps) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	return gitpkg.RemoveWorktree(repoPath, worktreePath, force)
}

// DeleteBranch deletes the now-orphaned local branch of a reaped worktree.
func (RealSweepDeps) DeleteBranch(repoPath, branch string, force bool) error {
	return gitpkg.DeleteBranch(repoPath, branch, force)
}
