// Package safemerge is the policy core of safe-merge, the one sanctioned path
// for landing a feature branch into a golden ~/src checkout.
//
// # Why merges are wrapped at all
//
// The golden-tree landing step is `git -C ~/src/<repo> merge --squash <branch>`
// followed by a commit. Two things make the raw form unsafe to allow-list:
//
//   - It is an all-or-nothing chain. `merge --squash` only *stages* the change;
//     skipping the follow-up commit leaves the golden tree dirty and the work
//     half-landed. An agent merely *told* to run two commands in order will
//     eventually run one (CLAUDE.md principle 9 — atomic action wrappers).
//   - The allow-list pattern that would permit it, `git -C ~/src/* merge *`,
//     also permits every dangerous merge: into the wrong branch, with conflicts
//     left in a wedged tree, or as a `--no-ff` commit that breaks linear
//     history. The overnight 2026-06-12 retro traced a 5.3h stall to exactly
//     this ask-rule firing on the deploy step (bead ce-f87f).
//
// What the wrapper guarantees by construction
//
//   - The target repo is clean and sitting on its default branch before the
//     merge; a dirty or off-branch tree is refused, not entangled.
//   - squash mode runs merge --squash AND the commit as one unit; a conflict
//     rolls the tree back to its pre-merge state instead of leaving markers.
//   - Every merge carries a wayfinder session trace (--wayfinder /
//     WAYFINDER_PROJECT_DIR whose WAYFINDER-STATUS.md is in_progress) stamped
//     into the commit message, or an audited --emergency --reason hatch.
//   - Force is impossible: the wrapper never pushes and never rewrites history;
//     pushing the landed commit stays safe-push's job.
//
// The session-resolution logic here intentionally mirrors internal/safepr; once
// both wrappers are on main they should consolidate into one shared package
// (tracked as a follow-up to ce-f87f).
package safemerge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultTimeout bounds a merge so a wedged credential helper or hung git
// subprocess fails fast instead of blocking the session indefinitely.
const DefaultTimeout = 60 * time.Second

// Mode is how the feature branch is folded in.
type Mode string

const (
	// ModeSquash collapses the branch to a single staged change that the
	// wrapper then commits — the golden-tree default, keeps history linear.
	ModeSquash Mode = "squash"
	// ModeNoFF records an explicit merge commit (used for divergent golden
	// merges where the merge commit itself is wanted).
	ModeNoFF Mode = "no-ff"
)

// Session identifies the active wayfinder session a merge is attributed to.
type Session struct {
	ID          string
	ProjectPath string
}

// statusFrontmatter is the subset of WAYFINDER-STATUS.md safe-merge consumes.
// Unknown fields are ignored on purpose: the schema belongs to wayfinder.
type statusFrontmatter struct {
	SessionID string `yaml:"session_id"`
	Status    string `yaml:"status"`
}

// ResolveSessionDir picks the wayfinder project directory: the --wayfinder
// flag wins, then WAYFINDER_PROJECT_DIR. An empty result teaches both options.
func ResolveSessionDir(flagDir string) (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}
	if env := os.Getenv("WAYFINDER_PROJECT_DIR"); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("no wayfinder session given: pass --wayfinder <project-dir> or set " +
		"WAYFINDER_PROJECT_DIR to the directory holding WAYFINDER-STATUS.md. Every golden merge " +
		"must carry a wayfinder trace; if this is a genuine emergency with no session, use " +
		"--emergency --reason \"<why>\" (audited)")
}

// LoadSession reads <dir>/WAYFINDER-STATUS.md and returns the session it
// describes, failing unless it parses and is status: in_progress.
func LoadSession(dir string) (Session, error) {
	path := filepath.Join(dir, "WAYFINDER-STATUS.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Session{}, fmt.Errorf("cannot read %s: %w — point --wayfinder/WAYFINDER_PROJECT_DIR "+
			"at the wayfinder project directory (the one holding WAYFINDER-STATUS.md)", path, err)
	}
	fm, err := frontmatter(string(raw))
	if err != nil {
		return Session{}, fmt.Errorf("%s: %w", path, err)
	}
	var st statusFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &st); err != nil {
		return Session{}, fmt.Errorf("%s: cannot parse YAML frontmatter: %w", path, err)
	}
	if st.Status != "in_progress" {
		return Session{}, fmt.Errorf("wayfinder session at %s is %q, not in_progress — start or "+
			"resume a session before landing merges against it", dir, st.Status)
	}
	if st.SessionID == "" {
		st.SessionID = sessionIDFromDeliverables(dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return Session{ID: st.SessionID, ProjectPath: abs}, nil
}

// deliverableSessionIDRe matches the wayfinder_session_id frontmatter line that
// phase deliverables carry; used to recover an id the status rewrite dropped
// (wayfinder bug ce-6kl1).
var deliverableSessionIDRe = regexp.MustCompile(`(?m)^wayfinder_session_id:\s*"?([0-9a-fA-F-]+)"?`)

func sessionIDFromDeliverables(dir string) string {
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return ""
	}
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if got := deliverableSessionIDRe.FindSubmatch(raw); got != nil {
			return string(got[1])
		}
	}
	return ""
}

// frontmatter extracts the YAML between the leading "---" fence pair.
func frontmatter(content string) (string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", fmt.Errorf("file does not start with YAML frontmatter (---)")
	}
	lines := strings.Split(content, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return strings.Join(lines[1:i], "\n"), nil
		}
	}
	return "", fmt.Errorf("unterminated YAML frontmatter (no closing ---)")
}

// Request is one validated safe-merge invocation.
type Request struct {
	Repo      string   // target golden repo (the merge destination)
	Branch    string   // feature branch to fold in
	Mode      Mode     // squash (default) or no-ff
	Message   string   // commit/merge message; trailer is always appended
	Session   *Session // nil only when Emergency
	Emergency bool
	Reason    string // required when Emergency
}

// Validate enforces the request invariants. Error messages follow principle 2:
// say what was attempted, the right way, and why.
func (r *Request) Validate() error {
	if strings.TrimSpace(r.Repo) == "" {
		return fmt.Errorf("--repo is required: the golden checkout to merge into " +
			"(e.g. --repo ~/src/dear-agent)")
	}
	if strings.TrimSpace(r.Branch) == "" {
		return fmt.Errorf("--branch is required: the feature branch to land")
	}
	if strings.HasPrefix(r.Branch, "-") {
		return fmt.Errorf("refusing branch %q: it looks like a flag, not a branch name — "+
			"safe-merge passes --branch as a literal git argument and will not let a flag "+
			"smuggle through", r.Branch)
	}
	switch r.Mode {
	case ModeSquash, ModeNoFF:
	default:
		return fmt.Errorf("--mode %q is not valid: use %q (default, linear) or %q "+
			"(explicit merge commit)", r.Mode, ModeSquash, ModeNoFF)
	}
	if r.Emergency {
		if strings.TrimSpace(r.Reason) == "" {
			return fmt.Errorf("--emergency requires --reason \"<why no wayfinder session exists>\" — " +
				"the reason is stamped on the commit and audit-logged so emergencies stay reviewable")
		}
	} else if r.Session == nil {
		return fmt.Errorf("no wayfinder session resolved and --emergency not set; this is a wrapper " +
			"bug — Validate must run after session resolution")
	}
	return nil
}

// Trailer renders the attribution block folded into the commit message.
func (r *Request) Trailer() string {
	if r.Emergency {
		return fmt.Sprintf("EMERGENCY (no wayfinder session): %s", strings.TrimSpace(r.Reason))
	}
	id := r.Session.ID
	if id == "" {
		// Honest trace: the project is the anchor; the id was lost by the
		// wayfinder CLI's status rewrite (ce-6kl1), not omitted by the caller.
		id = "unrecorded (status file lacks session_id — wayfinder bug ce-6kl1)"
	}
	return fmt.Sprintf("Wayfinder-Session: %s\nWayfinder-Project: %s",
		id, filepath.Base(r.Session.ProjectPath))
}

// CommitMessage is the full message stamped onto the landed commit: the
// caller's headline (or a derived default) plus the wayfinder trace trailer.
func (r *Request) CommitMessage(defaultBranch string) string {
	headline := strings.TrimSpace(r.Message)
	if headline == "" {
		headline = fmt.Sprintf("Merge branch '%s' into %s", r.Branch, defaultBranch)
	}
	return headline + "\n\n" + r.Trailer()
}

// gitRunner runs a git subprocess in repoDir under a context. It is a field so
// tests can substitute a recorder without spawning real git.
type gitRunner func(ctx context.Context, repoDir string, args ...string) (string, error)

func execGit(ctx context.Context, repoDir string, args ...string) (string, error) {
	full := append([]string{"-C", repoDir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// GIT_TERMINAL_PROMPT=0: never block on a terminal credential prompt; the
	// keychain hang safe-push guards against can wedge any git subprocess.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Merger executes a validated merge. The git field is injectable for tests.
type Merger struct {
	git     gitRunner
	timeout time.Duration
}

// NewMerger returns a Merger that shells out to real git.
func NewMerger(timeout time.Duration) *Merger {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Merger{git: execGit, timeout: timeout}
}

// Result reports what a merge did, for audit logging and the caller's summary.
type Result struct {
	DefaultBranch string
	CommitMessage string
	RolledBack    bool
}

// rollback runs a best-effort recovery git command and logs (never returns) a
// failure: the caller is already returning the original merge error, and a
// rollback that itself fails must be visible without masking the root cause.
func (m *Merger) rollback(ctx context.Context, repo string, args ...string) {
	if out, err := m.git(ctx, repo, args...); err != nil {
		slog.Warn("safe-merge rollback step failed; golden tree may need manual recovery",
			"repo", repo, "cmd", strings.Join(args, " "), "err", err, "output", strings.TrimSpace(out))
	}
}

// Run lands r.Branch into r.Repo as one atomic unit:
//
//  1. verify the repo is a clean git tree on its default branch,
//  2. fold the branch in (squash+commit, or a no-ff merge commit),
//  3. on any failure roll the tree back to its pre-merge state.
//
// It returns a Result on success or an error whose message names the failed
// step; on a squash conflict the tree is reset so the golden checkout is never
// left half-merged.
func (m *Merger) Run(r *Request) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	if out, err := m.git(ctx, r.Repo, "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("%s is not a git work tree: %w (%s)", r.Repo, err, strings.TrimSpace(out))
	}

	// Clean tree: a merge into a dirty tree entangles the caller's WIP with the
	// landed change and makes rollback ambiguous.
	status, err := m.git(ctx, r.Repo, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("cannot read status of %s: %w", r.Repo, err)
	}
	if strings.TrimSpace(status) != "" {
		return nil, fmt.Errorf("refusing to merge into %s: working tree is not clean —\n%s\n"+
			"commit, stash, or src-recovery the tree first; safe-merge will not fold a branch "+
			"into uncommitted work", r.Repo, strings.TrimSpace(status))
	}

	// Default branch: the golden landing target is always the repo's default
	// branch. Merging into a random feature branch in the golden checkout is a
	// mistake, not a workflow.
	defBranch, err := defaultBranch(ctx, m.git, r.Repo)
	if err != nil {
		return nil, err
	}
	cur, err := m.git(ctx, r.Repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("cannot read current branch of %s: %w", r.Repo, err)
	}
	if strings.TrimSpace(cur) != defBranch {
		return nil, fmt.Errorf("refusing to merge into %s: HEAD is on %q, not the default branch %q — "+
			"check out %s before landing (the golden tree only takes merges on its default branch)",
			r.Repo, strings.TrimSpace(cur), defBranch, defBranch)
	}

	msg := r.CommitMessage(defBranch)
	res := &Result{DefaultBranch: defBranch, CommitMessage: msg}

	switch r.Mode {
	case ModeNoFF:
		if out, err := m.git(ctx, r.Repo, "merge", "--no-ff", "-m", msg, r.Branch); err != nil {
			// A no-ff merge that conflicts leaves MERGE_HEAD; --abort restores.
			m.rollback(ctx, r.Repo, "merge", "--abort")
			res.RolledBack = true
			return res, fmt.Errorf("no-ff merge of %q failed and was rolled back: %w\n%s",
				r.Branch, err, strings.TrimSpace(out))
		}
	case ModeSquash:
		if out, err := m.git(ctx, r.Repo, "merge", "--squash", r.Branch); err != nil {
			// merge --squash creates no MERGE_HEAD, so reset --hard (not
			// --abort) is the correct rollback to the pre-merge commit.
			m.rollback(ctx, r.Repo, "reset", "--hard", "HEAD")
			res.RolledBack = true
			return res, fmt.Errorf("squash merge of %q conflicted and was rolled back: %w\n%s\n"+
				"resolve the conflict on the feature branch (rebase it onto %s) and retry",
				r.Branch, err, strings.TrimSpace(out), defBranch)
		}
		// The atomic second half: commit the staged squash. Skipping this is
		// the exact half-landed state the wrapper exists to make impossible.
		if out, err := m.git(ctx, r.Repo, "commit", "-m", msg); err != nil {
			m.rollback(ctx, r.Repo, "reset", "--hard", "HEAD")
			res.RolledBack = true
			return res, fmt.Errorf("squash staged but the commit failed and was rolled back: %w\n%s",
				err, strings.TrimSpace(out))
		}
	}
	return res, nil
}

// defaultBranch resolves the repo's default branch from origin/HEAD, falling
// back to a `main`/`master` probe when the remote head ref is not set.
func defaultBranch(ctx context.Context, git gitRunner, repo string) (string, error) {
	if out, err := git(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out)
		if i := strings.LastIndex(ref, "/"); i >= 0 {
			ref = ref[i+1:]
		}
		if ref != "" {
			return ref, nil
		}
	}
	for _, cand := range []string{"main", "master"} {
		if _, err := git(ctx, repo, "rev-parse", "--verify", cand); err == nil {
			return cand, nil
		}
	}
	return "", fmt.Errorf("cannot determine default branch of %s: origin/HEAD is unset and neither "+
		"main nor master exists — run `git -C %s remote set-head origin -a`", repo, repo)
}

// AuditRecord is one JSONL line in the safe-merge audit log. Every invocation —
// success, refusal, rollback — leaves a line, the same durable-trail contract
// safe-pr and src-recovery established.
type AuditRecord struct {
	Time       string `json:"time"`
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	Mode       string `json:"mode"`
	SessionID  string `json:"session_id,omitempty"`
	Emergency  bool   `json:"emergency,omitempty"`
	Reason     string `json:"reason,omitempty"`
	RolledBack bool   `json:"rolled_back,omitempty"`
	ExitCode   int    `json:"exit_code"`
	Error      string `json:"error,omitempty"`
}

// AppendAudit appends rec to ~/.local/state/dear-agent/safe-merge.log, creating
// the directory on first use. Audit failures are returned, not fatal.
func AppendAudit(home string, rec AuditRecord) error {
	dir := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create audit log dir %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "safe-merge.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot open audit log: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("cannot marshal audit record: %w", err)
	}
	_, err = f.Write(append(line, '\n'))
	return err
}
