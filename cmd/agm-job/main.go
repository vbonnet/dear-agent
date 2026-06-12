// agm-job is the atomic run wrapper for agm loop jobs.
//
// Every loop's --cmd should be "agm-job run <name> --verify <cmd> -- <job-cmd>".
// agm-job enforces:
//   - Per-job overlap prevention via atomic mkdir lock (no flock on macOS).
//   - Mandatory effect verification: both the job and the verify command must
//     succeed for the run to be recorded as successful.
//   - Dual escalation on failure: macOS notification + agm send msg
//     meta-orchestrator.
//   - Self-rotating log under ~/.agm/logs/<name>.log (10 MB cap, 3 generations).
//
// Usage:
//
//	agm-job run <name> --verify <cmd> [--timeout 30m] -- <job-cmd...>
//
// Exit codes: 0=success (job+verify both 0), 1=failure or skip, 2=usage error.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	lockDirBase  = ".agm/locks"
	logsDir      = ".agm/logs"
	maxLogBytes  = 10 * 1024 * 1024 // 10 MB
	logRotations = 3
)

var rootCmd = &cobra.Command{
	Use:   "agm-job",
	Short: "Atomic run wrapper for agm loop jobs",
	Long: `agm-job wraps an agm loop job command with overlap prevention,
mandatory effect verification, escalation on failure, and self-rotating logs.

Every agm loop definition should set its --cmd to:
  agm-job run <name> --verify "<verify-cmd>" -- <job-cmd>`,
}

var (
	verifyCmd  string
	jobTimeout time.Duration
)

var runCmd = &cobra.Command{
	Use:   "run <name> --verify <cmd> -- <job-cmd...>",
	Short: "Run a job with lock, verify, and escalation",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runJob,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2)
	}
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&verifyCmd, "verify", "", "Shell command to verify job effect (required)")
	_ = runCmd.MarkFlagRequired("verify")
	runCmd.Flags().DurationVar(&jobTimeout, "timeout", 30*time.Minute, "Maximum runtime for the job command")
}

func runJob(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Everything after "--" is the job command.
	jobArgs := cmd.Flags().Args()
	if len(jobArgs) == 0 {
		return fmt.Errorf("job command required after '--'")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	logFile := filepath.Join(homeDir, logsDir, name+".log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o700); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	log := newJobLogger(logFile)
	defer log.Close()

	log.Printf("=== agm-job start: %s ===", name)
	log.Printf("job:    %s", strings.Join(jobArgs, " "))
	log.Printf("verify: %s", verifyCmd)

	// Acquire overlap lock.
	lockDir := filepath.Join(homeDir, lockDirBase, name)
	acquired, err := acquireLock(lockDir, log)
	if err != nil {
		return fmt.Errorf("lock error: %w", err)
	}
	if !acquired {
		log.Printf("SKIP: another instance of %q is still running — exiting 0", name)
		return nil // deliberate skip is not a failure
	}
	defer releaseLock(lockDir, log)

	ctx, cancel := context.WithTimeout(cmd.Context(), jobTimeout)
	defer cancel()

	// Run the job command.
	jobStart := time.Now()
	jobExitCode, jobErr := runShellCmd(ctx, jobArgs, log, "JOB")
	jobDuration := time.Since(jobStart).Round(time.Millisecond)
	log.Printf("job exit=%d duration=%s", jobExitCode, jobDuration)

	if jobErr != nil || jobExitCode != 0 {
		msg := fmt.Sprintf("job %q failed (exit %d): %v", name, jobExitCode, jobErr)
		log.Printf("FAIL: %s", msg)
		escalate(homeDir, name, msg)
		return fmt.Errorf("%s", msg)
	}

	// Run the mandatory verify command.
	verifyStart := time.Now()
	verifyExitCode, verifyErr := runShellCmd(ctx, []string{"/bin/sh", "-c", verifyCmd}, log, "VERIFY")
	verifyDuration := time.Since(verifyStart).Round(time.Millisecond)
	log.Printf("verify exit=%d duration=%s", verifyExitCode, verifyDuration)

	if verifyErr != nil || verifyExitCode != 0 {
		msg := fmt.Sprintf("job %q verify failed (exit %d): %v", name, verifyExitCode, verifyErr)
		log.Printf("FAIL: %s", msg)
		escalate(homeDir, name, msg)
		return fmt.Errorf("%s", msg)
	}

	log.Printf("=== agm-job success: %s ===", name)
	return nil
}

// acquireLock creates the lock directory atomically. Returns (true, nil) if
// acquired, (false, nil) if already held, (false, err) on unexpected error.
func acquireLock(lockDir string, log *jobLogger) (bool, error) {
	if err := os.Mkdir(lockDir, 0o700); err != nil {
		if os.IsExist(err) {
			// Check if the PID in the lock is still alive.
			pidFile := filepath.Join(lockDir, "pid")
			pidBytes, readErr := os.ReadFile(pidFile)
			if readErr != nil {
				log.Printf("stale lock (no pid file): removing %s", lockDir)
				_ = os.RemoveAll(lockDir)
				if err2 := os.Mkdir(lockDir, 0o700); err2 != nil {
					return false, fmt.Errorf("re-acquire after stale: %w", err2)
				}
			} else {
				pid, _ := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
				if isProcessAlive(pid) {
					log.Printf("lock held by PID %d", pid)
					return false, nil
				}
				log.Printf("stale lock from dead PID %d: removing %s", pid, lockDir)
				_ = os.RemoveAll(lockDir)
				if err2 := os.Mkdir(lockDir, 0o700); err2 != nil {
					return false, fmt.Errorf("re-acquire after dead pid: %w", err2)
				}
			}
		} else {
			return false, err
		}
	}
	// Write our PID.
	pidFile := filepath.Join(lockDir, "pid")
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600)
	return true, nil
}

func releaseLock(lockDir string, log *jobLogger) {
	if err := os.RemoveAll(lockDir); err != nil {
		log.Printf("WARNING: failed to release lock %s: %v", lockDir, err)
	}
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; Signal(0) checks liveness.
	return proc.Signal(syscall.Signal(0)) == nil
}

func runShellCmd(ctx context.Context, args []string, log *jobLogger, label string) (int, error) {
	if len(args) == 0 {
		return 2, fmt.Errorf("empty command")
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...) //#nosec G204 -- caller-supplied, user-controlled
	c.Stdout = log.writer(label + " stdout")
	c.Stderr = log.writer(label + " stderr")
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// escalate sends a macOS notification and an agm message to meta-orchestrator.
func escalate(homeDir, name, msg string) {
	// macOS notification (best-effort, ignore errors).
	_ = exec.Command("osascript", "-e", //#nosec G204 -- fixed template, name/msg are log-safe
		fmt.Sprintf(`display notification %q with title "agm-job failed" subtitle %q`,
			msg, name)).Run()

	// agm send msg to meta-orchestrator (best-effort).
	agmBin := filepath.Join(homeDir, "go", "bin", "agm")
	_ = exec.Command(agmBin, "send", "msg", "meta-orchestrator", //#nosec G204 -- fixed args
		fmt.Sprintf("agm-job %q failed: %s", name, msg)).Run()
}

// jobLogger writes timestamped lines to a size-capped rotating log file.
type jobLogger struct {
	f    *os.File
	path string
}

func newJobLogger(path string) *jobLogger {
	rotateLogs(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		// Fall back to stderr; don't fail the job over logging.
		return &jobLogger{f: nil, path: path}
	}
	return &jobLogger{f: f, path: path}
}

func (l *jobLogger) Printf(format string, args ...any) {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02T15:04:05"), fmt.Sprintf(format, args...))
	if l.f != nil {
		_, _ = l.f.WriteString(line)
	}
	_, _ = fmt.Fprint(os.Stderr, line)
}

func (l *jobLogger) writer(label string) *prefixWriter {
	return &prefixWriter{log: l, label: label}
}

func (l *jobLogger) Close() {
	if l.f != nil {
		_ = l.f.Close()
	}
}

type prefixWriter struct {
	log   *jobLogger
	label string
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	for line := range strings.SplitSeq(strings.TrimRight(string(b), "\n"), "\n") {
		if line != "" {
			p.log.Printf("%s: %s", p.label, line)
		}
	}
	return len(b), nil
}

// rotateLogs keeps at most logRotations generations of logFile at maxLogBytes each.
func rotateLogs(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxLogBytes {
		return
	}
	// Rotate: .log.2 → .log.3 (dropping oldest), .log.1 → .log.2, .log → .log.1
	for i := logRotations; i > 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", path, i-1)
		newPath := fmt.Sprintf("%s.%d", path, i)
		_ = os.Rename(oldPath, newPath)
	}
	_ = os.Rename(path, path+".1")
}
