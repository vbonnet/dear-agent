// safe-pr opens, closes, and reopens GitHub pull requests with a mandatory wayfinder
// session trace. It is the only sanctioned PR path for agent sessions: the
// PreToolUse hook .claude/hooks/pretool-pr-guard denies raw `gh pr create|
// close|reopen` and points here.
//
// Usage:
//
//	safe-pr create --wayfinder <project-dir> --title "..." --body "..." [gh flags...]
//	safe-pr close  --wayfinder <project-dir> <number|url> [gh flags...]
//	safe-pr reopen --wayfinder <project-dir> <number|url> --comment "..." [gh flags...]
//
// The wayfinder project dir (or WAYFINDER_PROJECT_DIR) must contain a
// active WAYFINDER-STATUS.md; its canonical project_name is stamped into the
// PR body (create) or mutation comment (close/reopen). Creation stops after
// the provider-visible PR mutation; safe-merge owns routine merge admission.
// Every invocation is audit-logged to
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
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/safepr"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sys/unix"
)

const (
	preflightGuardVerb          = "__safe-pr-preflight-guard"
	preflightTransactionGuardFD = 3
)

func main() {
	pkgversion.PopulateFromBuildInfo()
	if len(os.Args) == 3 && os.Args[1] == preflightGuardVerb {
		if err := runPreflightGuard(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "safe-pr preflight guard: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-pr: %v\n", err)
		os.Exit(1)
	}
}

type parsedArgs struct {
	req           safepr.Request
	wayfinderDir  string
	bead          string
	timeout       time.Duration
	verifyCI      bool
	skipPreflight bool
	showedHelp    bool
}

func parseArgs(argv []string) (*parsedArgs, error) {
	if len(argv) == 0 || argv[0] == "-h" || argv[0] == "--help" {
		fmt.Print(usage)
		return &parsedArgs{showedHelp: true}, nil
	}
	if argv[0] == "--version" || argv[0] == "-v" {
		fmt.Printf("safe-pr %s\n", pkgversion.String())
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
			// Opt-in: after successful non-draft creation, confirm CI actually
			// started on the new PR's head SHA and warn (never fail) if it did
			// not. Off by default so the common create path is never slowed by a
			// poll.
			p.verifyCI = true
		case "--skip-preflight":
			p.skipPreflight = true
		default:
			p.req.GhArgs = append(p.req.GhArgs, arg)
		}
	}
	return p, nil
}

func run(argv []string) error {
	return runContext(context.Background(), argv)
}

func runContext(ctx context.Context, argv []string) error {
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
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

	var githubOutcome githubExecution
	execute := func(transaction *safepr.WorktreeTransaction) error {
		if p.req.Verb == "create" && !p.skipPreflight {
			if err := runPreflightFull(cwd, transaction); err != nil {
				return err
			}
		}

		shutdown := otelsetup.InitTracer("safe-pr")
		defer func() {
			if err := shutdown(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "safe-pr: otel shutdown: %v\n", err)
			}
		}()

		var executeErr error
		githubOutcome, executeErr = executeGitHub(ctx, &p.req, p.timeout, p.verifyCI, transaction)
		return executeErr
	}
	var runErr error
	if p.req.Verb == "create" {
		runErr = protectCreateWorktree(cwd, "safe-pr create", execute)
	} else {
		runErr = execute(nil)
	}
	appendFinalAudit(&p.req, cwd, githubOutcome, runErr)
	return runErr
}

type githubExecution struct {
	prURL    string
	exitCode int
}

// executeGitHub is the external PR mutation boundary. Tests replace it so a
// unit test can prove control flow without creating a real GitHub pull request.
var executeGitHub = execGh

// appendSafePRAudit is the durable audit boundary. Tests replace it so command
// control-flow regressions cannot write to the developer's real audit log.
var appendSafePRAudit = safepr.AppendAudit

// protectCreateWorktree owns the worktree lock across both preflight and the
// GitHub mutation. Tests replace it to prove the command's transaction scope.
var protectCreateWorktree = safepr.WithWorktreeTransaction

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

// preflightTimeout bounds `make preflight-full` while covering the repository's
// cold race suite, long-running SQLite package, and vulnerability scan.
const preflightTimeout = 60 * time.Minute

// runPreflightFull runs `make -C dir preflight-full` and returns a clear error
// on failure. The protected child is a guard runner rather than make itself:
// AGM's test suite intentionally launches detached tmux servers, and directly
// passing it the transaction descriptor leaves the worktree lock live after
// make exits. The runner retains the descriptor while make runs, but marks it
// close-on-exec before launching make so descendants cannot retain it.
// Assigned to a var so tests can replace it without spawning make.
var runPreflightFull = func(dir string, transaction *safepr.WorktreeTransaction) error {
	ctx, cancel := context.WithTimeout(context.Background(), preflightTimeout)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("preflight-full resolve safe-pr executable: %w", err)
	}
	cmd := exec.CommandContext(ctx, executable, preflightGuardVerb, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := protectTransactionCommand(transaction, cmd); err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("preflight-full failed — fix issues before creating PR (timed out after %s)", preflightTimeout)
		}
		return fmt.Errorf("preflight-full failed — fix issues before creating PR: %w", err)
	}
	return nil
}

// runPreflightGuard is the protected preflight child. ExtraFiles are placed at
// fd 3 in a child that otherwise has only stdin/stdout/stderr; close that
// descriptor on the next exec while this runner keeps it open for its lifetime.
func runPreflightGuard(dir string) error {
	guardDir, err := preflightGuardDirectory(dir)
	if err != nil {
		return err
	}
	if err := closeOnExec(preflightTransactionGuardFD); err != nil {
		return fmt.Errorf("mark transaction guard close-on-exec: %w", err)
	}
	cmd := exec.Command("make", "-C", guardDir, "preflight-full")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// preflightGuardDirectory binds the hidden guard verb to the caller's working
// directory. The parent passes its cwd only as an integrity assertion; make
// always receives the runner-derived directory rather than command-line input.
func preflightGuardDirectory(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("preflight directory is required")
	}
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve preflight working directory: %w", err)
	}
	if filepath.Clean(dir) != filepath.Clean(currentDir) {
		return "", errors.New("preflight directory must match the guard working directory")
	}
	return currentDir, nil
}

func closeOnExec(fd int) error {
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		return err
	}
	_, err = unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags|unix.FD_CLOEXEC)
	return err
}

// prURLRe matches the PR URL gh prints on success. Anchored to a word
// boundary to satisfy CodeQL's regex-anchor check.
var prURLRe = regexp.MustCompile(`\bhttps://github\.com/[^\s]+/pull/\d+\b`)

// execGh runs the stamped gh command, bounded by timeout and with
// GIT_TERMINAL_PROMPT=0, and returns the GitHub outcome used by the final
// transaction audit boundary.
func execGh(ctx context.Context, req *safepr.Request, timeout time.Duration, verifyCI bool, transaction *safepr.WorktreeTransaction) (githubExecution, error) {
	if timeout <= 0 {
		timeout = safepr.DefaultTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := req.StampedArgs()
	// #nosec G702 -- the executable is the fixed gh binary; Request.Validate
	// admits only supported PR verbs/flags, and argv is never interpreted by a shell.
	cmd := exec.CommandContext(commandCtx, "gh", args...)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	if err := protectTransactionCommand(transaction, cmd); err != nil {
		return githubExecution{}, err
	}

	_, span := otel.Tracer("safe-pr").Start(commandCtx, "safepr."+req.Verb)
	runErr := cmd.Run()
	prURL := prURLRe.FindString(out.String())

	exitCode := 0
	if runErr != nil {
		exitCode = 1
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exitCode = ee.ExitCode()
		}
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

	if commandCtx.Err() == context.DeadlineExceeded {
		return githubExecution{prURL: prURL, exitCode: exitCode}, fmt.Errorf("gh exceeded %s and was killed — gh may have been waiting on an "+
			"interactive prompt; pass all required flags explicitly (safe-pr requires --title/--body "+
			"for create) and retry", timeout)
	}
	if runErr != nil {
		return githubExecution{prURL: prURL, exitCode: exitCode}, fmt.Errorf("gh pr %s failed: %w", req.Verb, runErr)
	}
	handlePostCreate(ctx, req, prURL, verifyCI, transaction)
	return githubExecution{prURL: prURL, exitCode: exitCode}, nil
}

func appendFinalAudit(req *safepr.Request, cwd string, outcome githubExecution, finalErr error) {
	exitCode := outcome.exitCode
	errText := ""
	if finalErr != nil {
		if exitCode == 0 {
			exitCode = 1
		}
		errText = finalErr.Error()
	}
	sessionID := ""
	if req.Session != nil {
		sessionID = req.Session.ID
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		home = ""
		fmt.Fprintf(os.Stderr, "safe-pr: WARNING: could not determine home directory for audit log: %v\n", homeErr)
	}
	rec := safepr.AuditRecord{
		Time: time.Now().UTC().Format(time.RFC3339), Verb: req.Verb, Dir: cwd,
		Args: req.StampedArgs(), SessionID: sessionID,
		PRURL: outcome.prURL, ExitCode: exitCode, Error: errText,
	}
	if auditErr := appendSafePRAudit(home, rec); auditErr != nil {
		// The PR action may already have happened; a failed audit write must not
		// change the command outcome, but it must be visible.
		fmt.Fprintf(os.Stderr, "safe-pr: WARNING: audit log write failed: %v\n", auditErr)
	}
}

func handlePostCreate(ctx context.Context, req *safepr.Request, prURL string, verifyCI bool, transaction *safepr.WorktreeTransaction) {
	if req.Verb != "create" || prURL == "" {
		return
	}
	draft := requestsDraft(req.GhArgs)
	// Opt-in safety net for the push-then-PR-open race (bead ce-np2s): an
	// unarmed PR whose head SHA never gets check-runs would otherwise need
	// manual investigation. When asked, confirm CI actually started and warn
	// loudly if it did not; the PR already exists, so this remains advisory.
	if verifyCI && !draft {
		warnIfNoCI(ctx, prURL, transaction)
	}
}

// requestsDraft reports whether the pass-through GitHub CLI arguments request
// draft creation. GitHub's boolean flags accept -d/--draft and explicit
// Boolean values; the final occurrence wins, matching gh argument parsing.
func requestsDraft(args []string) bool {
	draft := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if ghCreateFlagConsumesNext(arg) {
			i++
			continue
		}
		if arg == "--draft" || arg == "-d" {
			draft = true
			continue
		}
		value, longForm := strings.CutPrefix(arg, "--draft=")
		if !longForm {
			value, longForm = strings.CutPrefix(arg, "-d=")
		}
		if longForm {
			parsed, err := strconv.ParseBool(value)
			if err == nil {
				draft = parsed
			}
			continue
		}
		clusterDraft, consumesNext := draftInShorthandCluster(arg)
		if clusterDraft {
			draft = true
		}
		if consumesNext {
			i++
		}
	}
	return draft
}

func ghCreateFlagConsumesNext(arg string) bool {
	longValueFlags := []string{
		"--assignee", "--base", "--body", "--body-file", "--head", "--label",
		"--milestone", "--project", "--recover", "--reviewer", "--template", "--title",
	}
	shortValueFlags := []string{"-a", "-B", "-b", "-H", "-l", "-m", "-p", "-R", "-r", "-T", "-t"}
	return slices.Contains(longValueFlags, arg) || slices.Contains(shortValueFlags, arg)
}

// draftInShorthandCluster follows pflag's shorthand-cluster rule: Boolean
// shorthands may be combined, while the first value-taking shorthand consumes
// the rest of the token. For example, -dt requests draft and then a title from
// the next argument, while -td gives title the value "d" and is not draft.
func draftInShorthandCluster(arg string) (draft bool, consumesNext bool) {
	if len(arg) < 3 || !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return false, false
	}
	valueTaking := "aBbHlmpRrTt"
	clusterText, explicitValue, hasExplicitValue := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
	cluster := []rune(clusterText)
	for i, shorthand := range cluster {
		if shorthand == 'd' {
			if i == len(cluster)-1 && hasExplicitValue {
				parsed, err := strconv.ParseBool(explicitValue)
				if err == nil {
					draft = parsed
				}
				return draft, false
			}
			draft = true
			continue
		}
		if strings.ContainsRune(valueTaking, shorthand) {
			return draft, i == len(cluster)-1 && !hasExplicitValue
		}
	}
	return draft, false
}

// verifyCIPollWindow bounds how long warnIfNoCI waits for the first check-run
// to register on a freshly created PR's head SHA before warning.
const verifyCIPollWindow = 60 * time.Second

// warnIfNoCI polls the new PR's head SHA for any check-run and, if none has
// appeared within verifyCIPollWindow, prints a warning naming the PR and the
// recovery command. It never returns an error: the PR was created, so a
// missing-CI condition is surfaced rather than treated as a create failure. Any
// gh lookup failure along the way is silently ignored — this is a best-effort
// safety net, not a gate.
func warnIfNoCI(ctx context.Context, prURL string, transaction *safepr.WorktreeTransaction) {
	if ctx.Err() != nil {
		return
	}
	repo, num, ok := parsePRURL(prURL)
	if !ok {
		return
	}
	pollCtx, cancel := context.WithTimeout(ctx, verifyCIPollWindow)
	defer cancel()
	sha := ghHeadSHA(pollCtx, repo, num, transaction)
	if sha == "" {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ghCheckRunCount(pollCtx, repo, sha, transaction) > 0 {
			return // CI registered — nothing to warn about.
		}
		select {
		case <-pollCtx.Done():
			if ctx.Err() == nil && pollCtx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(os.Stderr, "safe-pr: WARNING: PR %s head %s still has 0 CI check-runs after %s "+
					"— the CI trigger may have been dropped (push-then-PR-open race). Re-trigger with: "+
					"agm pr scan-no-checks --repo %s --trigger\n", prURL, shortSHA(sha), verifyCIPollWindow, repo)
			}
			return
		case <-ticker.C:
		}
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
func ghHeadSHA(ctx context.Context, repo, num string, transaction *safepr.WorktreeTransaction) string {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "gh", "pr", "view", num, "--repo", repo, "--json", "headRefOid", "--jq", ".headRefOid")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	if err := protectTransactionCommand(transaction, cmd); err != nil {
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ghCheckRunCount returns the number of check-runs reported against sha, or 0 on
// any error (treated as "not yet started").
func ghCheckRunCount(ctx context.Context, repo, sha string, transaction *safepr.WorktreeTransaction) int {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "gh", "api",
		fmt.Sprintf("repos/%s/commits/%s/check-runs", repo, sha), "--jq", ".total_count")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	if err := protectTransactionCommand(transaction, cmd); err != nil {
		return 0
	}
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

func protectTransactionCommand(transaction *safepr.WorktreeTransaction, cmd *exec.Cmd) error {
	if cmd == nil {
		return fmt.Errorf("protect safe-pr child command: command is required")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	cmd.WaitDelay = time.Second
	return transaction.ProtectCommand(cmd)
}

const usage = `safe-pr — open/close/reopen GitHub PRs with a mandatory wayfinder session trace.

This is the only sanctioned PR path for agent sessions (AGENTS.md principle 9):
raw 'gh pr create|close|reopen' in Bash is denied by .claude/hooks/pretool-pr-guard.
Untraced PRs burn CI/review quota and cannot be attributed afterwards.

Usage:
  safe-pr create --wayfinder <project-dir> --title "..." --body "..." [gh flags...]
  safe-pr close  --wayfinder <project-dir> <number|url> [gh flags...]
  safe-pr reopen --wayfinder <project-dir> <number|url> --comment "..." [gh flags...]

Flags:
  --wayfinder <dir>   wayfinder project dir holding WAYFINDER-STATUS.md
                      (default: $WAYFINDER_PROJECT_DIR); session must be in-progress
  --bead <id>         bead this PR closes; folds "Closes <id>" into the create
                      body (default: $BEAD, then the session's first bead)
  --timeout <dur>     kill gh after this long (default 60s)
  --verify-ci         after create, poll the new PR's head SHA and warn (never
                      fail) if no CI check-run starts within 60s — catches the
                      push-then-PR-open race that strands a PR with 0 checks
  --skip-preflight    skip the preflight-full gate (emergency/hotfix only);
                      by default, safe-pr create runs 'make preflight-full' in
                      the current directory and blocks PR creation on failure
  -h, --help          show this help

All other arguments pass through to 'gh pr create' / 'gh pr close' / 'gh pr reopen'.
The session trace is stamped into the PR body (create) or comment (close/reopen).
On create, 'make preflight-full' is run first to ensure local build/test/lint
health before the PR hits CI (shift-left gate). Use --skip-preflight only for
emergencies; prefer fixing the underlying issues instead.
On create, safe-pr stops after provider-visible PR creation. Once checks and
review are ready, use safe-merge for routine agent-authored merge admission.
A PR created with --draft remains a human handoff.
Refused for create: --web, --fill*, --body-file/-F, --editor (interactive or
unstampable); --title is required. Every run appends a JSONL audit record to
~/.local/state/dear-agent/safe-pr.log.

If no wayfinder session exists and no approved path is available in a current
AGM session, escalate:
  agm escalate ask --kind blocked-action --context "<why no session exists>" "create PR"
Outside AGM, add --session <registered-session>. If no registered session
exists, ask the current user directly.
`
