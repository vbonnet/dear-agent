// Package safepr is the policy core of safe-pr, the one sanctioned path for
// opening and closing GitHub pull requests from agent sessions.
//
// # Why PRs are wrapped at all
//
// Raw `gh pr create` from an agent leaves no trace of WHY the PR exists: no
// wayfinder session, no plan, no telemetry. Untraced PRs spray CI and review
// quota (every PR triggers the full required-check suite plus bot review) and
// cannot be attributed or audited afterwards. The PreToolUse hook
// .claude/hooks/pretool-pr-guard therefore denies raw `gh pr create|close|
// reopen` in Bash, and this wrapper is the path it points to (CLAUDE.md
// principle 9 — atomic action wrappers).
//
// What the wrapper guarantees by construction
//
//   - Only the verbs `create` and `close` exist; everything else is refused.
//   - A PR carries a wayfinder session trace: the caller names an active
//     wayfinder project (--wayfinder flag or WAYFINDER_PROJECT_DIR env) whose
//     WAYFINDER-STATUS.md is status: in_progress, and the session id is
//     stamped into the PR body (create) or close comment (close).
//   - The audited emergency hatch (--emergency --reason "...") replaces the
//     session trace with an explicit EMERGENCY marker — never silence.
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

	"gopkg.in/yaml.v3"
)

// DefaultTimeout bounds a gh invocation so a wedged network call or an
// accidentally-interactive prompt fails fast instead of hanging the session.
const DefaultTimeout = 60 * time.Second

// Session identifies the active wayfinder session a PR is attributed to.
type Session struct {
	ID          string
	ProjectPath string
}

// statusFrontmatter is the subset of WAYFINDER-STATUS.md frontmatter safe-pr
// consumes. Unknown fields are ignored on purpose: the schema belongs to
// wayfinder, not to us.
type statusFrontmatter struct {
	SessionID string `yaml:"session_id"`
	Status    string `yaml:"status"`
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
		"carry a wayfinder trace; if this is a genuine emergency with no session, use " +
		"--emergency --reason \"<why>\" (audited)")
}

// LoadSession reads <dir>/WAYFINDER-STATUS.md and returns the session it
// describes. It fails unless the file exists, parses, carries a session_id,
// and is status: in_progress — a completed or abandoned session is not a
// valid attribution target for new PRs.
func LoadSession(dir string) (Session, error) {
	path := filepath.Join(dir, "WAYFINDER-STATUS.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Session{}, fmt.Errorf("cannot read %s: %w — point --wayfinder/WAYFINDER_PROJECT_DIR "+
			"at a wayfinder project directory (the one holding WAYFINDER-STATUS.md)", path, err)
	}
	fm, err := frontmatter(string(raw))
	if err != nil {
		return Session{}, fmt.Errorf("%s: %w", path, err)
	}
	var st statusFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &st); err != nil {
		return Session{}, fmt.Errorf("%s: cannot parse YAML frontmatter: %w", path, err)
	}
	if st.SessionID == "" {
		return Session{}, fmt.Errorf("%s has no session_id in its frontmatter", path)
	}
	if st.Status != "in_progress" {
		return Session{}, fmt.Errorf("wayfinder session %s is %q, not in_progress — start or resume "+
			"a session (wayfinder start / wayfinder session) before opening PRs against it",
			st.SessionID, st.Status)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return Session{ID: st.SessionID, ProjectPath: abs}, nil
}

// frontmatter extracts the YAML between the leading "---" fence pair.
func frontmatter(content string) (string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
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

// Request is one validated safe-pr invocation.
type Request struct {
	Verb      string   // "create" or "close"
	Session   *Session // nil only when Emergency
	Emergency bool
	Reason    string   // required when Emergency
	GhArgs    []string // caller's pass-through gh arguments
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
	case "create", "close":
	default:
		return fmt.Errorf("safe-pr only supports `create` and `close`, not %q — other gh pr verbs "+
			"(view, list, checks, diff) are read-only and need no wrapper; `merge` keeps its existing "+
			"review-gated path", r.Verb)
	}
	if r.Emergency {
		if strings.TrimSpace(r.Reason) == "" {
			return fmt.Errorf("--emergency requires --reason \"<why no wayfinder session exists>\" — " +
				"the reason is stamped on the PR and audit-logged so emergencies stay reviewable")
		}
	} else if r.Session == nil || r.Session.ID == "" {
		return fmt.Errorf("no wayfinder session resolved and --emergency not set; this is a wrapper " +
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
	if r.Verb == "close" {
		if !hasFlag(r.GhArgs, "--comment", "-c") {
			return fmt.Errorf("safe-pr close requires an explicit --comment explaining why the PR " +
				"is being closed — unattributed closures are invisible to reviewers and cannot be audited; " +
				"use: safe-pr close --wayfinder <dir> --comment \"reason\" <number>")
		}
	}
	return nil
}

// Trailer renders the attribution block stamped onto the PR.
func (r *Request) Trailer() string {
	if r.Emergency {
		return fmt.Sprintf("---\nEMERGENCY (no wayfinder session): %s", strings.TrimSpace(r.Reason))
	}
	return fmt.Sprintf("---\nWayfinder-Session: %s\nWayfinder-Project: %s",
		r.Session.ID, filepath.Base(r.Session.ProjectPath))
}

// StampedArgs returns the final gh argv (after "gh"): the verb mapped to
// `pr <verb>` with the trace trailer folded into --body (create) or
// --comment (close), appending the flag when the caller did not pass one.
func (r *Request) StampedArgs() []string {
	target, short := "--body", "-b"
	if r.Verb == "close" {
		target, short = "--comment", "-c"
	}
	args := append([]string{"pr", r.Verb}, stampFlag(r.GhArgs, target, short, r.Trailer())...)
	return args
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
// success, refusal, or emergency — leaves a line, the same durable-trail
// contract src-recovery established for ~/src writes.
type AuditRecord struct {
	Time      string   `json:"time"`
	Verb      string   `json:"verb"`
	Dir       string   `json:"dir"`
	Args      []string `json:"args"`
	SessionID string   `json:"session_id,omitempty"`
	Emergency bool     `json:"emergency,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	PRURL     string   `json:"pr_url,omitempty"`
	ExitCode  int      `json:"exit_code"`
	Error     string   `json:"error,omitempty"`
}

// AppendAudit appends rec to ~/.local/state/dear-agent/safe-pr.log, creating
// the directory on first use. Audit failures are returned, not fatal — the
// caller decides whether a PR should fail because the log could not be
// written (it should not).
func AppendAudit(home string, rec AuditRecord) error {
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
	defer f.Close()
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("cannot marshal audit record: %w", err)
	}
	_, err = f.Write(append(line, '\n'))
	return err
}
