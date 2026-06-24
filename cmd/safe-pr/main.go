// safe-pr opens and closes GitHub pull requests with a mandatory wayfinder
// session trace. It is the only sanctioned PR path for agent sessions: the
// PreToolUse hook .claude/hooks/pretool-pr-guard denies raw `gh pr create|
// close|reopen` and points here.
//
// Usage:
//
//	safe-pr create --wayfinder <project-dir> --title "..." --body "..." [gh flags...]
//	safe-pr close  --wayfinder <project-dir> <number|url> [gh flags...]
//
// The wayfinder project dir (or WAYFINDER_PROJECT_DIR) must contain a
// WAYFINDER-STATUS.md with status: in_progress; its session_id is stamped
// into the PR body (create) or close comment (close). On create, squash
// auto-merge is armed on the new PR so it merges itself once required checks
// and reviews pass. Every invocation is audit-logged to
// ~/.local/state/dear-agent/safe-pr.log and emits an OTel span (safepr.<verb>)
// when a collector is configured.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/safepr"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-pr: %v\n", err)
		os.Exit(1)
	}
}

type parsedArgs struct {
	req          safepr.Request
	wayfinderDir string
	bead         string
	timeout      time.Duration
	verifyCI     bool
	showedHelp   bool
}

func parseArgs(argv []string) (*parsedArgs, error) {
	if len(argv) == 0 || argv[0] == "-h" || argv[0] == "--help" {
		fmt.Print(usage)
		return &parsedArgs{showedHelp: true}, nil
	}

	p := &parsedArgs{
		req:     safepr.Request{Verb: argv[0]},
		timeout: safepr.DefaultTimeout,
	}

	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(usage)
			return &parsedArgs{showedHelp: true}, nil
		case "--wayfinder":
			if i+1 >= len(argv) {
				return nil, fmt.Errorf("--wayfinder requires a wayfinder project directory argument")
			}
			p.wayfinderDir = argv[i+1]
			i++
		case "--bead":
			if i+1 >= len(argv) {
				return nil, fmt.Errorf("--bead requires a bead id argument (e.g. ce-5vje)")
			}
			p.bead = argv[i+1]
			i++
		case "--timeout":
			if i+1 >= len(argv) {
				return nil, fmt.Errorf("--timeout requires a duration argument (e.g. 60s)")
			}
			d, err := time.ParseDuration(argv[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid --timeout %q: %w", argv[i+1], err)
			}
			p.timeout = d
			i++
		case "--verify-ci":
			// Opt-in: after create+arm, confirm CI actually started on the new
			// PR's head SHA and warn (never fail) if it did not. Off by default
			// so the common create path is never slowed by a poll.
			p.verifyCI = true
		default:
			p.req.GhArgs = append(p.req.GhArgs, arg)
		}
	}
	return p, nil
}

func run(argv []string) error {
	p, err := parseArgs(argv)
	if err != nil {
		return err
	}
	if p.showedHelp {
		return nil
	}

	dir, err := safepr.ResolveSessionDir(p.wayfinderDir)
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	if err := validateRemoteURL(cwd); err != nil {
		return err
	}
	s, err := safepr.LoadSession(dir)
	if err != nil {
		return err
	}
	// Bead precedence: explicit --bead flag, then BEAD env var, then the bead
	// LoadSession read from WAYFINDER-STATUS.md. An explicit choice always wins
	// over the session default so a caller can target a different bead.
	if p.bead != "" {
		s.BeadID = p.bead
	} else if env := os.Getenv("BEAD"); env != "" {
		s.BeadID = env
	}
	p.req.Session = &s
	if err := p.req.Validate(); err != nil {
		return err
	}

	shutdown := otelsetup.InitTracer("safe-pr")
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "safe-pr: otel shutdown: %v\n", err)
		}
	}()

	return execGh(&p.req, p.timeout, p.verifyCI)
}

// expectedRemotePrefix is the required GitHub org prefix for origin remotes.
// Any URL that does not start with this string is rejected by validateRemoteURL.
const expectedRemotePrefix = "https://github.com/vbonnet/"

// validateRemoteURL checks that the origin remote for the repo in dir points
// to the expected GitHub org. It fails loudly so agents cannot silently push
// to a stale or wrong remote (e.g. a dear-labs fork).
func validateRemoteURL(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("could not resolve origin remote URL in %s: %w", dir, err)
	}
	url := strings.TrimSpace(string(out))
	if !strings.HasPrefix(url, expectedRemotePrefix) {
		return fmt.Errorf("remote URL %q does not look like a vbonnet GitHub remote. Expected: %s<repo>", url, expectedRemotePrefix)
	}
	return nil
}

// prURLRe matches the PR URL gh prints on success. Anchored to a word
// boundary to satisfy CodeQL's regex-anchor check.
var prURLRe = regexp.MustCompile(`\bhttps://github\.com/[^\s]+/pull/\d+\b`)

// execGh runs the stamped gh command, bounded by timeout and with
// GIT_TERMINAL_PROMPT=0, then writes the audit record and span. The audit
// line is written on every outcome, success or failure.
func execGh(req *safepr.Request, timeout time.Duration, verifyCI bool) error {
	if timeout <= 0 {
		timeout = safepr.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := req.StampedArgs()
	cmd := exec.CommandContext(ctx, "gh", args...)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")

	_, span := otel.Tracer("safe-pr").Start(ctx, "safepr."+req.Verb)
	runErr := cmd.Run()
	prURL := prURLRe.FindString(out.String())

	exitCode := 0
	errText := ""
	if runErr != nil {
		exitCode = 1
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exitCode = ee.ExitCode()
		}
		errText = runErr.Error()
	}

	sessionID := ""
	if req.Session != nil {
		sessionID = req.Session.ID
	}
	span.SetAttributes(
		attribute.String("wayfinder.session_id", sessionID),
		attribute.String("pr.url", prURL),
		attribute.Int("pr.exit_code", exitCode),
	)
	span.End()

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	rec := safepr.AuditRecord{
		Time: time.Now().UTC().Format(time.RFC3339), Verb: req.Verb, Dir: cwd,
		Args: args, SessionID: sessionID,
		PRURL: prURL, ExitCode: exitCode, Error: errText,
	}
	if auditErr := safepr.AppendAudit(home, rec); auditErr != nil {
		// The PR action already happened; a failed audit write must not fail
		// the run, but it must be visible.
		fmt.Fprintf(os.Stderr, "safe-pr: WARNING: audit log write failed: %v\n", auditErr)
	}

	// Arm squash auto-merge on a freshly created PR so it merges itself once
	// required checks and reviews pass — every safe-pr PR is hands-off by
	// construction. Best-effort: the PR already exists, so a failure here
	// (auto-merge disabled on the repo, branch not yet pushed) must not fail
	// the run; it is surfaced as a warning and can be armed manually.
	if runErr == nil && req.Verb == "create" && prURL != "" {
		if mergeErr := armAutoMerge(prURL, timeout); mergeErr != nil {
			fmt.Fprintf(os.Stderr, "safe-pr: WARNING: could not arm auto-merge on %s: %v\n", prURL, mergeErr)
		}
		// Opt-in safety net for the push-then-PR-open race (bead ce-np2s): an
		// armed PR whose head SHA never gets check-runs would wait on auto-merge
		// forever with no signal. When asked, confirm CI actually started and
		// warn loudly if it did not — a warning only, since the PR exists and
		// the `agm pr scan-no-checks` sweep is the durable backstop.
		if verifyCI {
			warnIfNoCI(prURL)
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("gh exceeded %s and was killed — gh may have been waiting on an "+
			"interactive prompt; pass all required flags explicitly (safe-pr requires --title/--body "+
			"for create) and retry", timeout)
	}
	if runErr != nil {
		return fmt.Errorf("gh pr %s failed: %w", req.Verb, runErr)
	}
	return nil
}

// verifyCIPollWindow bounds how long warnIfNoCI waits for the first check-run
// to register on a freshly created PR's head SHA before warning.
const verifyCIPollWindow = 60 * time.Second

// warnIfNoCI polls the new PR's head SHA for any check-run and, if none has
// appeared within verifyCIPollWindow, prints a warning naming the PR and the
// recovery command. It never returns an error: the PR was created and armed, so
// a missing-CI condition is surfaced, not treated as a create failure. Any gh
// lookup failure along the way is silently ignored — this is a best-effort
// safety net, not a gate.
func warnIfNoCI(prURL string) {
	repo, num, ok := parsePRURL(prURL)
	if !ok {
		return
	}
	sha := ghHeadSHA(repo, num)
	if sha == "" {
		return
	}
	deadline := time.Now().Add(verifyCIPollWindow)
	for {
		if ghCheckRunCount(repo, sha) > 0 {
			return // CI registered — nothing to warn about.
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "safe-pr: WARNING: PR %s head %s still has 0 CI check-runs after %s "+
				"— the CI trigger may have been dropped (push-then-PR-open race). Re-trigger with: "+
				"agm pr scan-no-checks --repo %s --trigger\n", prURL, shortSHA(sha), verifyCIPollWindow, repo)
			return
		}
		time.Sleep(5 * time.Second)
	}
}

// prURLPathRe captures owner/repo and PR number from a github.com PR URL.
var prURLPathRe = regexp.MustCompile(`github\.com/([^/]+/[^/]+)/pull/(\d+)\b`)

// parsePRURL extracts "owner/repo" and the PR number string from a PR URL.
func parsePRURL(prURL string) (repo, number string, ok bool) {
	m := prURLPathRe.FindStringSubmatch(prURL)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// ghHeadSHA returns the head SHA of PR number num in repo, or "" on any error.
func ghHeadSHA(repo, num string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", num, "--repo", repo, "--json", "headRefOid", "--jq", ".headRefOid")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ghCheckRunCount returns the number of check-runs reported against sha, or 0 on
// any error (treated as "not yet started").
func ghCheckRunCount(repo, sha string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/commits/%s/check-runs", repo, sha), "--jq", ".total_count")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// armAutoMerge enables squash auto-merge on prURL so the PR merges itself once
// required checks and reviews pass. It runs with its own timeout and the same
// non-interactive environment as the create call. A non-nil error is advisory:
// the caller treats it as a warning, not a failure.
func armAutoMerge(prURL string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = safepr.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "merge", "--auto", "--squash", prURL)
	var errBuf bytes.Buffer
	cmd.Stdout = nil // discard: success message ("✓ Armed auto-merge…") must not pollute safe-pr stdout
	cmd.Stderr = &errBuf
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("gh pr merge --auto exceeded %s and was killed", timeout)
		}
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return fmt.Errorf("gh pr merge --auto: %w\nsafe-pr: WARNING: gh stderr: %s", err, msg)
		}
		return fmt.Errorf("gh pr merge --auto: %w", err)
	}
	return nil
}

const usage = `safe-pr — open/close GitHub PRs with a mandatory wayfinder session trace.

This is the only sanctioned PR path for agent sessions (AGENTS.md principle 9):
raw 'gh pr create|close|reopen' in Bash is denied by .claude/hooks/pretool-pr-guard.
Untraced PRs burn CI/review quota and cannot be attributed afterwards.

Usage:
  safe-pr create --wayfinder <project-dir> --title "..." --body "..." [gh flags...]
  safe-pr close  --wayfinder <project-dir> <number|url> [gh flags...]

Flags:
  --wayfinder <dir>   wayfinder project dir holding WAYFINDER-STATUS.md
                      (default: $WAYFINDER_PROJECT_DIR); session must be in_progress
  --bead <id>         bead this PR closes; folds "Closes <id>" into the create
                      body (default: $BEAD, then the session's first bead)
  --timeout <dur>     kill gh after this long (default 60s)
  --verify-ci         after create, poll the new PR's head SHA and warn (never
                      fail) if no CI check-run starts within 60s — catches the
                      push-then-PR-open race that strands a PR with 0 checks
  -h, --help          show this help

All other arguments pass through to 'gh pr create' / 'gh pr close'.
The session trace is stamped into the PR body (create) or comment (close).
On create, squash auto-merge is armed on the new PR (best-effort) so it merges
itself once required checks and reviews pass.
Refused for create: --web, --fill*, --body-file/-F, --editor (interactive or
unstampable); --title is required. Every run appends a JSONL audit record to
~/.local/state/dear-agent/safe-pr.log.

If no wayfinder session exists and no approved path is available, escalate:
  agm escalate --action "create PR" --reason "<why no session exists>"
`
