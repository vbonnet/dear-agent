// Command agm-job is the host-side job runner for the dear-agent dispatch loop
// (ce-m3ya, Phase A of ce-cd14). It wraps an arbitrary shell command with:
//
//   - Atomic flock-based locking: prevents concurrent runs of the same job
//   - Mandatory effect verification: --verify <cmd> must exit 0 after the job
//   - Dual-layer escalation on failure: macOS notification + agm send
//   - Self-rotating log files under ~/.agm/logs/
//
// Usage:
//
//	agm-job run --name <name> --verify <cmd> [--escalate-session <session-id>] \
//	            [--timeout <duration>] [--log-max-bytes N] -- <command> [args...]
//
// The job is identified by --name (used for the lock file and log path). If two
// agm-job processes with the same name run concurrently, the second exits
// immediately with exit code 3 ("already running").
//
// Exit codes:
//
//	0  success (command ran and verify passed)
//	1  job failure (command failed or verify failed; escalation sent)
//	2  usage / configuration error
//	3  lock held by another instance (already running)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	exitSuccess     = 0
	exitFailure     = 1
	exitUsage       = 2
	exitAlreadyRun  = 3
	defaultMaxBytes = 10 * 1024 * 1024 // 10 MiB
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(exitUsage)
	}
	switch os.Args[1] {
	case "run":
		if err := runJob(os.Args[2:]); err != nil {
			var runErr *jobError
			if errors.As(err, &runErr) {
				os.Exit(runErr.code)
			}
			fmt.Fprintf(os.Stderr, "agm-job: %v\n", err)
			os.Exit(exitFailure)
		}
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "agm-job: unknown subcommand %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(exitUsage)
	}
}

type jobError struct {
	code int
	msg  string
}

func (e *jobError) Error() string { return e.msg }

type options struct {
	name            string
	verify          string
	escalateSession string
	timeout         time.Duration
	logMaxBytes     int64
	args            []string
}

func runJob(argv []string) error {
	fs := flag.NewFlagSet("agm-job run", flag.ContinueOnError)
	var opts options
	fs.StringVar(&opts.name, "name", "", "Job name (used for lock and log files) [required]")
	fs.StringVar(&opts.verify, "verify", "", "Verification command to run after the job [required]")
	fs.StringVar(&opts.escalateSession, "escalate-session", "", "AGM session ID to notify on failure (optional)")
	fs.DurationVar(&opts.timeout, "timeout", 0, "Timeout for the job command (0 = no timeout)")
	fs.Int64Var(&opts.logMaxBytes, "log-max-bytes", defaultMaxBytes, "Max log file size before rotation")

	// Collect args before and after `--`.
	rawArgs := argv
	sepIdx := -1
	for i, a := range rawArgs {
		if a == "--" {
			sepIdx = i
			break
		}
	}
	var flagArgs []string
	if sepIdx >= 0 {
		flagArgs = rawArgs[:sepIdx]
		opts.args = rawArgs[sepIdx+1:]
	} else {
		flagArgs = rawArgs
	}

	if err := fs.Parse(flagArgs); err != nil {
		return &jobError{exitUsage, err.Error()}
	}

	if opts.name == "" {
		return &jobError{exitUsage, "--name is required"}
	}
	if opts.verify == "" {
		return &jobError{exitUsage, "--verify is required (mandatory effect verification)"}
	}
	if len(opts.args) == 0 {
		return &jobError{exitUsage, "no command given (place command after --)"}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}

	logPath := filepath.Join(homeDir, ".agm", "logs", opts.name+".log")
	logFile, err := openRotatingLog(logPath, opts.logMaxBytes)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()

	logf := func(format string, a ...any) {
		ts := time.Now().Format("2006-01-02T15:04:05Z07:00")
		line := fmt.Sprintf("[%s] %s\n", ts, fmt.Sprintf(format, a...))
		_, _ = logFile.WriteString(line)
		fmt.Print(line)
	}

	lockDir := filepath.Join(homeDir, ".agm", "locks")
	lockPath := filepath.Join(lockDir, opts.name+".lock")
	if err := acquireLock(lockDir, lockPath, opts.name); err != nil {
		logf("lock held by another instance: %v", err)
		return &jobError{exitAlreadyRun, fmt.Sprintf("agm-job %q is already running", opts.name)}
	}
	defer releaseLock(lockPath)

	logf("start job=%q cmd=%q", opts.name, strings.Join(opts.args, " "))

	ctx := context.Background()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if opts.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	jobErr := runCommand(ctx, opts.args, logFile)
	if jobErr != nil {
		logf("job failed: %v", jobErr)
		escalate(opts, logf, fmt.Sprintf("job %q failed: %v", opts.name, jobErr))
		return &jobError{exitFailure, fmt.Sprintf("job failed: %v", jobErr)}
	}
	logf("job succeeded, running verify: %q", opts.verify)

	verifyErr := runCommand(ctx, []string{"/bin/sh", "-c", opts.verify}, logFile)
	if verifyErr != nil {
		logf("verify failed: %v", verifyErr)
		escalate(opts, logf, fmt.Sprintf("job %q verify failed: %v", opts.name, verifyErr))
		return &jobError{exitFailure, fmt.Sprintf("verify failed: %v", verifyErr)}
	}

	logf("done ok job=%q", opts.name)
	return nil
}

// runCommand executes argv, piping stdout+stderr to w. Returns non-nil on failure.
func runCommand(ctx context.Context, argv []string, w io.Writer) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //#nosec G204
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// acquireLock tries to atomically create the lock directory. On macOS, mkdir is
// atomic for a single path component — if it returns an error, the lock is held.
func acquireLock(lockDir, lockPath, name string) error {
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if os.IsExist(err) {
			// Check if the lock is stale (process no longer exists).
			if stale, _ := isStale(lockPath); stale {
				_ = os.Remove(lockPath)
				return os.Mkdir(lockPath, 0o700)
			}
			return fmt.Errorf("lock dir %q exists — %s already running", lockPath, name)
		}
		return err
	}
	// Write our PID inside the lock dir so staleness can be detected.
	_ = os.WriteFile(filepath.Join(lockPath, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600)
	return nil
}

func releaseLock(lockPath string) {
	_ = os.Remove(filepath.Join(lockPath, "pid"))
	_ = os.Remove(lockPath)
}

// isStale returns true if the PID recorded in the lock dir no longer exists.
func isStale(lockPath string) (bool, error) {
	pidBytes, err := os.ReadFile(filepath.Join(lockPath, "pid"))
	if err != nil {
		return false, err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(pidBytes)), "%d", &pid); err != nil {
		return false, err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true, nil
	}
	// On Unix, FindProcess never returns an error; signal 0 tests liveness.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return true, nil
	}
	return false, nil
}

// openRotatingLog opens (or creates) the log file at path, rotating it if it
// exceeds maxBytes by renaming it to <path>.1.
func openRotatingLog(path string, maxBytes int64) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= maxBytes {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //#nosec G304
}

// escalate fires a macOS notification (osascript) and, if --escalate-session is
// set, an agm send to that session.
func escalate(opts options, logf func(string, ...any), msg string) {
	if runtime.GOOS == "darwin" {
		notify(opts.name, msg, logf)
	}
	if opts.escalateSession != "" {
		sendToSession(opts.escalateSession, msg, logf)
	}
}

func notify(jobName, msg string, logf func(string, ...any)) {
	script := fmt.Sprintf(
		`display notification %q with title "agm-job failure" subtitle %q`,
		msg, jobName,
	)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput() //#nosec G204
	if err != nil {
		logf("osascript notify failed: %v: %s", err, out)
	}
}

func sendToSession(sessionID, msg string, logf func(string, ...any)) {
	out, err := exec.Command("agm", "send", sessionID, msg).CombinedOutput() //#nosec G204
	if err != nil {
		logf("agm send to %s failed: %v: %s", sessionID, err, out)
	}
}

func printUsage(w *os.File) {
	_, _ = fmt.Fprint(w, `agm-job — host-side job runner for the dear-agent dispatch loop (ce-m3ya).

Usage:
  agm-job run [flags] -- <command> [args...]

Flags:
  --name <name>               Job name (lock file + log file base name) [required]
  --verify <cmd>              Verification command run after the job [required]
  --escalate-session <id>     AGM session ID to notify on failure (optional)
  --timeout <duration>        Timeout for the job command (0 = no timeout)
  --log-max-bytes <N>         Max log size before rotation (default 10485760)

Exit codes:
  0  success
  1  job or verify failure (escalation sent)
  2  usage / configuration error
  3  lock held by another instance (already running)

Examples:
  # Run a src-health check with verify and macOS notification on failure
  agm-job run --name src-health \
      --verify 'test -f ~/.agm/logs/src-health.ok' \
      -- src-health

  # Run bead-burndown with a 5-minute timeout and session escalation
  agm-job run --name bead-burndown \
      --verify 'bd ready --db ~/beads/context-engine/.beads > /dev/null' \
      --escalate-session vroom-orchestrator \
      --timeout 5m \
      -- bd burndown
`)
}
