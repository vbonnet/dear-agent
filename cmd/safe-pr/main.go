// safe-pr opens and closes GitHub pull requests with a mandatory wayfinder
// session trace. It is the only sanctioned PR path for agent sessions: the
// PreToolUse hook .claude/hooks/pretool-pr-guard denies raw `gh pr create|
// close|reopen` and points here.
//
// Usage:
//
//	safe-pr create --wayfinder <project-dir> --title "..." --body "..." [gh flags...]
//	safe-pr close  --wayfinder <project-dir> <number|url> [gh flags...]
//	safe-pr create --emergency --reason "<why>" --title "..." [gh flags...]
//
// The wayfinder project dir (or WAYFINDER_PROJECT_DIR) must contain a
// WAYFINDER-STATUS.md with status: in_progress; its session_id is stamped
// into the PR body (create) or close comment (close). Every invocation is
// audit-logged to ~/.local/state/dear-agent/safe-pr.log and emits an OTel
// span (safepr.<verb>) when a collector is configured.
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
	timeout      time.Duration
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
		case "--emergency":
			p.req.Emergency = true
		case "--reason":
			if i+1 >= len(argv) {
				return nil, fmt.Errorf("--reason requires a text argument")
			}
			p.req.Reason = argv[i+1]
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

	if !p.req.Emergency {
		dir, err := safepr.ResolveSessionDir(p.wayfinderDir)
		if err != nil {
			return err
		}
		s, err := safepr.LoadSession(dir)
		if err != nil {
			return err
		}
		p.req.Session = &s
	}
	if err := p.req.Validate(); err != nil {
		return err
	}

	shutdown := otelsetup.InitTracer("safe-pr")
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "safe-pr: otel shutdown: %v\n", err)
		}
	}()

	return execGh(&p.req, p.timeout)
}

// prURLRe matches the PR URL gh prints on success.
var prURLRe = regexp.MustCompile(`https://github\.com/\S+/pull/\d+`)

// execGh runs the stamped gh command, bounded by timeout and with
// GIT_TERMINAL_PROMPT=0, then writes the audit record and span. The audit
// line is written on every outcome, success or failure.
func execGh(req *safepr.Request, timeout time.Duration) error {
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
		attribute.Bool("pr.emergency", req.Emergency),
		attribute.String("pr.url", prURL),
		attribute.Int("pr.exit_code", exitCode),
	)
	span.End()

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	rec := safepr.AuditRecord{
		Time: time.Now().UTC().Format(time.RFC3339), Verb: req.Verb, Dir: cwd,
		Args: args, SessionID: sessionID, Emergency: req.Emergency,
		Reason: req.Reason, PRURL: prURL, ExitCode: exitCode, Error: errText,
	}
	if auditErr := safepr.AppendAudit(home, rec); auditErr != nil {
		// The PR action already happened; a failed audit write must not fail
		// the run, but it must be visible.
		fmt.Fprintf(os.Stderr, "safe-pr: WARNING: audit log write failed: %v\n", auditErr)
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

const usage = `safe-pr — open/close GitHub PRs with a mandatory wayfinder session trace.

This is the only sanctioned PR path for agent sessions (CLAUDE.md principle 9):
raw 'gh pr create|close|reopen' in Bash is denied by .claude/hooks/pretool-pr-guard.
Untraced PRs burn CI/review quota and cannot be attributed afterwards.

Usage:
  safe-pr create --wayfinder <project-dir> --title "..." --body "..." [gh flags...]
  safe-pr close  --wayfinder <project-dir> <number|url> [gh flags...]

Flags:
  --wayfinder <dir>   wayfinder project dir holding WAYFINDER-STATUS.md
                      (default: $WAYFINDER_PROJECT_DIR); session must be in_progress
  --emergency         skip the session requirement (audited; requires --reason)
  --reason <text>     why no wayfinder session exists; stamped on the PR
  --timeout <dur>     kill gh after this long (default 60s)
  -h, --help          show this help

All other arguments pass through to 'gh pr create' / 'gh pr close'.
The session trace is stamped into the PR body (create) or comment (close).
Refused for create: --web, --fill*, --body-file/-F, --editor (interactive or
unstampable); --title is required. Every run appends a JSONL audit record to
~/.local/state/dear-agent/safe-pr.log.
`
