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
	"regexp"
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
// describes. It fails unless the file exists, parses, and is status:
// in_progress — a completed or abandoned session is not a valid attribution
// target for new PRs.
//
// session_id is taken from the status frontmatter when present, but the
// wayfinder CLI's phase transitions currently rewrite the status file with a
// schema that drops session_id (bug ce-6kl1; 179/589 corpus status files are
// affected), so an empty id falls back to the wayfinder_session_id recorded
// in the phase deliverables' frontmatter. A session with no recoverable id
// is still valid — the project path is the trace anchor — and the trailer
// says so explicitly rather than failing the PR.
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
	if st.Status != "in_progress" {
		return Session{}, fmt.Errorf("wayfinder session at %s is %q, not in_progress — start or resume "+
			"a session (wayfinder start / wayfinder session) before opening PRs against it",
			dir, st.Status)
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

// deliverableSessionIDRe matches the wayfinder_session_id line wayfinder
// phase deliverables carry in their frontmatter.
var deliverableSessionIDRe = regexp.MustCompile(`(?m)^wayfinder_session_id:\s*"?([0-9a-fA-F-]+)"?`)

// sessionIDFromDeliverables recovers the session id from any phase
// deliverable's frontmatter when the status file has lost it (ce-6kl1).
// Returns "" when no deliverable records one.
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

// Request is one validated safe-pr invocation.
type Request struct {
	Verb      string   // "create" or "close"
	Session   *Session // nil only when Emergency
	Emergency bool
	Reason    string   // required when Emergency
	GhArgs    []string // caller's pass-through gh arguments
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
	"-f": "derives the body from commits (short for --fill), so the wayfinder trace cannot be " +
		"stamped — pass --title and --body explicitly",
	"--body-file": "supplies the body from a file safe-pr will not rewrite — pass --body instead",
	"-F":          "supplies the body from a file safe-pr will not rewrite — pass --body instead",
	"--editor":    "is interactive — agents must pass --title and --body explicitly",
	"-e":          "is interactive — agents must pass --title and --body explicitly",
	"--template": "supplies the body from a template safe-pr will not rewrite — pass --body " +
		"instead",
	"-T": "supplies the body from a template safe-pr will not rewrite — pass --body instead",
}

// deniedAlwaysFlags are refused for every verb: safe-pr operates on the repo
// of the current working directory, so the allow-listed wrapper cannot be
// steered at arbitrary repositories the user's gh auth happens to reach.
var deniedAlwaysFlags = map[string]string{
	"--repo": "safe-pr operates on the repository of the current directory — run it from the " +
		"repo (or worktree) the PR belongs to instead of targeting another repo",
	"-R": "safe-pr operates on the repository of the current directory — run it from the " +
		"repo (or worktree) the PR belongs to instead of targeting another repo",
}

// Validate enforces the request invariants. Error messages follow principle 2:
// say what was attempted, the right way, and why.
func (r *Request) Validate() error {
	switch r.Verb {
	case "create", "close":
	default:
		return fmt.Errorf("safe-pr only supports `create` and `close`, not %q — read-only gh pr verbs "+
			"(view, list, checks, diff) need no wrapper, `merge` keeps its existing review-gated path, "+
			"and `reopen` has no sanctioned automated path: reopening a closed PR is a human decision, "+
			"so ask the supervisor/user to do it", r.Verb)
	}
	if r.Emergency {
		if strings.TrimSpace(r.Reason) == "" {
			return fmt.Errorf("--emergency requires --reason \"<why no wayfinder session exists>\" — " +
				"the reason is stamped on the PR and audit-logged so emergencies stay reviewable")
		}
	} else if r.Session == nil {
		return fmt.Errorf("no wayfinder session resolved and --emergency not set; this is a wrapper " +
			"bug — Validate must run after session resolution")
	}
	for _, a := range r.GhArgs {
		flag := a
		if i := strings.IndexByte(a, '='); i > 0 {
			flag = a[:i]
		}
		if why, bad := deniedAlwaysFlags[flag]; bad {
			return fmt.Errorf("refusing %q: %s", a, why)
		}
		if r.Verb == "create" {
			if why, bad := deniedCreateFlags[flag]; bad {
				return fmt.Errorf("refusing %q: it %s", a, why)
			}
		}
	}
	if r.Verb == "create" && !hasFlag(r.GhArgs, "--title", "-t") {
		return fmt.Errorf("safe-pr create requires an explicit --title (and --body) so the run " +
			"is deterministic and headless-safe — gh would otherwise drop into an interactive prompt")
	}
	return nil
}

// Trailer renders the attribution block stamped onto the PR.
func (r *Request) Trailer() string {
	if r.Emergency {
		return fmt.Sprintf("---\nEMERGENCY (no wayfinder session): %s", strings.TrimSpace(r.Reason))
	}
	id := r.Session.ID
	if id == "" {
		// Honest trace: the project is the anchor; the id was lost by the
		// wayfinder CLI's status rewrite (ce-6kl1), not omitted by the caller.
		id = "unrecorded (status file lacks session_id — wayfinder bug ce-6kl1)"
	}
	return fmt.Sprintf("---\nWayfinder-Session: %s\nWayfinder-Project: %s",
		id, filepath.Base(r.Session.ProjectPath))
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
// the split ("--body", "x"), inline long ("--body=x"), and combined short
// ("-bx" / "-b=x") forms; when the flag is absent it is appended with the
// trailer as its whole value. Covering every spelling matters: a missed form
// would make the appended --body win under gh's last-flag-wins parsing and
// silently replace the caller's body with just the trailer.
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
		case strings.HasPrefix(a, short) && len(a) > len(short):
			// Combined short form: -bvalue or -b=value.
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
		if a == long || a == short || strings.HasPrefix(a, long+"=") ||
			(strings.HasPrefix(a, short) && len(a) > len(short)) {
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
