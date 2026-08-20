// Package safepr is the policy core of safe-pr, the one sanctioned path for
// opening, closing, and reopening GitHub pull requests from agent sessions.
//
// # Why PRs are wrapped at all
//
// Raw `gh pr create` from an agent leaves no trace of WHY the PR exists: no
// wayfinder session, no plan, no telemetry. Untraced PRs spray CI and review
// quota (every PR triggers the full required-check suite plus bot review) and
// cannot be attributed or audited afterwards. The PreToolUse hook
// .claude/hooks/pretool-pr-guard therefore denies raw `gh pr create|close|
// reopen` in Bash, and this wrapper is the path it points to (AGENTS.md
// principle 9 — atomic action wrappers).
//
// What the wrapper guarantees by construction
//
//   - Only the verbs `create`, `close`, and `reopen` exist; everything else is refused.
//   - A PR carries a wayfinder session trace: the caller names an active
//     wayfinder project (--wayfinder flag or WAYFINDER_PROJECT_DIR env) whose
//     WAYFINDER-STATUS.md is active, and its canonical project name is stamped
//     into the PR body (create) or mutation comment (close/reopen).
//   - Interactive and unstampable forms (--web, --fill, --body-file, missing
//     --title) are refused so the run is deterministic and headless-safe.
package safepr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/sessionid"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/statusread"
)

// DefaultTimeout bounds a gh invocation so a wedged network call or an
// accidentally-interactive prompt fails fast instead of hanging the session.
const DefaultTimeout = 60 * time.Second

// Session identifies the active wayfinder session a PR is attributed to.
type Session struct {
	ID          string
	ProjectPath string
	// BeadID is the bead this session's PR should close, if known. It is the
	// first entry of the WAYFINDER-STATUS.md `beads:` list (wayfinder
	// auto-creates one bead per task at SETUP). When set, safe-pr folds a
	// "Closes <bead>" line into the create body so the PR auto-closes its bead
	// on merge (bead-pr-sync scans the body for this reference).
	BeadID string
}

// ResolveSessionDir picks the wayfinder project directory: the --wayfinder
// flag wins, then the WAYFINDER_PROJECT_DIR environment variable. An empty
// result is an error that teaches the caller both options.
func ResolveSessionDir(flagDir string) (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}
	if env := os.Getenv("WAYFINDER_PROJECT_DIR"); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("no wayfinder session given: pass --wayfinder <project-dir> or set " +
		"WAYFINDER_PROJECT_DIR to the directory containing WAYFINDER-STATUS.md. Every PR must " +
		"carry a wayfinder trace. In a current AGM session, escalate via: " +
		"agm escalate ask --kind blocked-action --context \"<why no session exists>\" \"create PR\". " +
		"Outside AGM, add --session <registered-session>; if no registered session exists, " +
		"ask the current user directly")
}

// LoadSession reads <dir>/WAYFINDER-STATUS.md and returns the session it
// describes. It fails unless the file exists, parses, carries a project_name
// and has schema 2.0 plus an active status. Canonical planning and in-progress
// sessions are active; completed, abandoned, and blocked sessions
// are not valid attribution targets for new PRs.
func LoadSession(dir string) (Session, error) {
	path := filepath.Join(dir, "WAYFINDER-STATUS.md")
	st, err := statusread.ParseFromDir(dir)
	if err != nil {
		return Session{}, fmt.Errorf("cannot load validated status from %s: %w — point --wayfinder/WAYFINDER_PROJECT_DIR "+
			"at a wayfinder project directory (the one holding WAYFINDER-STATUS.md)", path, err)
	}
	if !isActiveStatus(st.Status) {
		return Session{}, fmt.Errorf("wayfinder session %s is %q, not active — start or resume "+
			"a session with `wayfinder session start <project-name>` before opening PRs against it",
			st.ProjectName, st.Status)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	beadID := ""
	if len(st.Beads) > 0 {
		beadID = strings.TrimSpace(st.Beads[0])
	}
	return Session{ID: st.ProjectName, ProjectPath: abs, BeadID: beadID}, nil
}

func isActiveStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "planning", "in-progress":
		return true
	default:
		return false
	}
}

// Request is one validated safe-pr invocation.
type Request struct {
	Verb    string   // "create", "close", or "reopen"
	Session *Session // always required; set before calling Validate
	GhArgs  []string // caller's pass-through gh arguments
}

// deniedCloseFlags are gh pr close flags that defeat the wrapper's purpose.
var deniedCloseFlags = map[string]string{
	"--delete-branch": "deleting the branch on close is irreversible and prevents reopening — " +
		"if the PR is truly abandoned, delete the branch separately after confirming",
}

// deniedCreateFlags are gh pr create flags that defeat the wrapper's purpose:
// --web hands off to a browser (interactive, untraceable), --fill* derive the
// body from commits so the trace cannot be stamped, --body-file/-F supplies a
// body we will not rewrite, and --editor is interactive by definition.
var deniedCreateFlags = map[string]string{
	"--web": "opens an interactive browser flow — the PR would be created outside the audited path",
	"-w":    "opens an interactive browser flow — the PR would be created outside the audited path",
	"--fill": "derives the body from commits, so the wayfinder trace cannot be stamped — pass " +
		"--title and --body explicitly",
	"--fill-first": "derives the body from commits, so the wayfinder trace cannot be stamped — pass " +
		"--title and --body explicitly",
	"--fill-verbose": "derives the body from commits, so the wayfinder trace cannot be stamped — pass " +
		"--title and --body explicitly",
	"--body-file": "supplies the body from a file safe-pr will not rewrite — pass --body instead",
	"-F":          "supplies the body from a file safe-pr will not rewrite — pass --body instead",
	"--editor":    "is interactive — agents must pass --title and --body explicitly",
	"-e":          "is interactive — agents must pass --title and --body explicitly",
}

// Validate enforces the request invariants. Error messages follow principle 2:
// say what was attempted, the right way, and why.
func (r *Request) Validate() error {
	switch r.Verb {
	case "create", "close", "reopen":
	default:
		return fmt.Errorf("safe-pr only supports `create`, `close`, and `reopen`, not %q — other gh pr verbs "+
			"(view, list, checks, diff) are read-only and need no wrapper; `merge` keeps its existing "+
			"review-gated path", r.Verb)
	}
	if r.Session == nil || r.Session.ID == "" {
		return fmt.Errorf("no wayfinder session resolved; this is a wrapper " +
			"bug — Validate must run after session resolution")
	}
	for _, a := range r.GhArgs {
		flag := a
		if i := strings.IndexByte(a, '='); i > 0 {
			flag = a[:i]
		}
		var denied map[string]string
		switch r.Verb {
		case "create":
			denied = deniedCreateFlags
		case "close":
			denied = deniedCloseFlags
		}
		if why, bad := denied[flag]; bad {
			return fmt.Errorf("refusing %q: it %s", a, why)
		}
	}
	if r.Verb == "create" {
		if !hasFlag(r.GhArgs, "--title", "-t") {
			return fmt.Errorf("safe-pr create requires an explicit --title (and --body) so the run " +
				"is deterministic and headless-safe — gh would otherwise drop into an interactive prompt")
		}
	}
	if r.Verb == "close" || r.Verb == "reopen" {
		if !hasFlag(r.GhArgs, "--comment", "-c") {
			return fmt.Errorf("safe-pr %s requires an explicit --comment explaining why the PR "+
				"is being mutated — unattributed state changes are invisible to reviewers and cannot be audited; "+
				"use: safe-pr %s --wayfinder <dir> --comment \"reason\" <number>", r.Verb, r.Verb)
		}
	}
	if err := r.checkSessionLeak(); err != nil {
		return err
	}
	return nil
}

// checkSessionLeak refuses to publish a PR whose title, body, or comment
// carries a Claude Code session reference.
//
// Every pass-through argument is scanned, not just the value of --title/--body/
// --comment: the leak is the published text, and gh accepts it in split
// ("--body" "x"), inline ("--body=x"), and label/assignee positions alike.
// Scanning the whole argv means a new gh flag cannot silently open a new hole.
//
// This rejects rather than redacts. safe-pr is relaying an author's words, and
// a wrapper that quietly rewrote them would publish something the author never
// wrote — worse than refusing. sessionid.Redact exists for callers that own
// their text.
func (r *Request) checkSessionLeak() error {
	for _, a := range r.GhArgs {
		findings := sessionid.Scan(a)
		if len(findings) == 0 {
			continue
		}
		return fmt.Errorf("refusing to publish: this %s carries a private Claude Code session "+
			"reference:\n%s\nA session reference addresses a private transcript, gives reviewers "+
			"nothing, and is permanent once pushed — squash-merge folds it into main's history. "+
			"Remove it and re-run; describe what changed and why instead of linking the session",
			r.Verb, sessionid.Describe(findings))
	}
	return nil
}

// Trailer renders the attribution block stamped onto the PR.
func (r *Request) Trailer() string {
	return fmt.Sprintf("---\nWayfinder-Session: %s\nWayfinder-Project: %s",
		r.Session.ID, filepath.Base(r.Session.ProjectPath))
}

// StampedArgs returns the final gh argv (after "gh"): the verb mapped to
// `pr <verb>` with the trace trailer folded into --body (create) or
// --comment (close/reopen), appending the flag when the caller did not pass one.
//
// On create, when the session carries a bead, a "Closes <bead>" line is folded
// in above the trace trailer so the PR auto-closes its bead on merge — unless
// the caller already referenced the bead in their args (no duplicate line).
func (r *Request) StampedArgs() []string {
	target, short := "--body", "-b"
	if r.Verb == "close" || r.Verb == "reopen" {
		target, short = "--comment", "-c"
	}
	trailer := r.Trailer()
	if r.Verb == "create" && r.Session != nil && r.Session.BeadID != "" &&
		!referencesBead(r.GhArgs, r.Session.BeadID) {
		trailer = "Closes " + r.Session.BeadID + "\n\n" + trailer
	}
	args := append([]string{"pr", r.Verb}, stampFlag(r.GhArgs, target, short, trailer)...)
	return args
}

// referencesBead reports whether any pass-through arg already mentions beadID,
// so safe-pr does not stamp a second "Closes <bead>" line when the caller wrote
// their own.
func referencesBead(args []string, beadID string) bool {
	for _, a := range args {
		if strings.Contains(a, beadID) {
			return true
		}
	}
	return false
}

// stampFlag appends trailer to the value of `long`/`short` in args, handling
// both the split ("--body", "x") and inline ("--body=x") forms; when the flag
// is absent it is appended with the trailer as its whole value.
func stampFlag(args []string, long, short, trailer string) []string {
	out := make([]string, 0, len(args)+2)
	stamped := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case (a == long || a == short) && i+1 < len(args):
			out = append(out, a, args[i+1]+"\n\n"+trailer)
			i++
			stamped = true
		case strings.HasPrefix(a, long+"="):
			out = append(out, a+"\n\n"+trailer)
			stamped = true
		default:
			out = append(out, a)
		}
	}
	if !stamped {
		out = append(out, long, trailer)
	}
	return out
}

func hasFlag(args []string, long, short string) bool {
	for _, a := range args {
		if a == long || a == short || strings.HasPrefix(a, long+"=") {
			return true
		}
	}
	return false
}

// AuditRecord is one JSONL line in the safe-pr audit log. Every invocation —
// success or refusal — leaves a line, the same durable-trail contract
// src-recovery established for ~/src writes.
type AuditRecord struct {
	Time      string   `json:"time"`
	Verb      string   `json:"verb"`
	Dir       string   `json:"dir"`
	Args      []string `json:"args"`
	SessionID string   `json:"session_id,omitempty"`
	PRURL     string   `json:"pr_url,omitempty"`
	ExitCode  int      `json:"exit_code"`
	Error     string   `json:"error,omitempty"`
}

// AppendAudit appends rec to ~/.local/state/dear-agent/safe-pr.log, creating
// the directory on first use. Audit failures are returned, not fatal — the
// caller decides whether a PR should fail because the log could not be
// written (it should not).
func AppendAudit(home string, rec AuditRecord) (err error) {
	if home == "" {
		home = "/tmp"
	}
	dir := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create audit log dir %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "safe-pr.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot open audit log: %w", err)
	}
	// Closing a writable handle can surface deferred write errors (flush
	// failures, full disk). Capture the close error, but never let it mask
	// an earlier write error.
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("cannot close audit log: %w", cerr)
		}
	}()
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("cannot marshal audit record: %w", err)
	}
	if _, err = f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}
