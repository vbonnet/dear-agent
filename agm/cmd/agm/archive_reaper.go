package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/reaper"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
)

// spawnReaper spawns a detached agm-reaper process for async archival.
// The reaper waits for the harness prompt, sends its native graceful-exit
// command, and archives the session.
func spawnReaper(sessionID, tmuxSession, harness string, outcome manifest.SessionOutcome, allowSupervisorReap bool) error {
	// Find agm-reaper binary (should be in same directory as agm)
	agmPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	reaperPath := filepath.Join(filepath.Dir(agmPath), "agm-reaper")

	logFile := reaperLogPath(tmuxSession)

	// Check if reaper binary exists
	if _, err := os.Stat(reaperPath); err != nil {
		ui.PrintError(err,
			"agm-reaper binary not found",
			fmt.Sprintf("  • Expected location: %s\n"+
				"  • Log file: %s\n"+
				"  • Reinstall the coherent pair: make install-agm\n"+
				"  • Or from agm/: make install\n"+
				"  • Or use synchronous archive: agm session archive %s (without --async)",
				reaperPath, logFile, sessionID))
		return fmt.Errorf("agm-reaper binary not found (log: %s): %w", logFile, err)
	}

	// The CLI and detached reaper share lifecycle serialization code. Refuse to
	// cross the process boundary unless the exact binary at reaperPath proves it
	// was built from the same VCS revision. The reaper repeats this check after
	// exec so a post-merge rename between this probe and cmd.Start still fails
	// closed instead of running mixed lifecycle schemas.
	expectedRevision := pkgversion.RevisionIdentity(GitCommit)
	if expectedRevision == "" || expectedRevision == "unknown" || expectedRevision == "unknown-dirty" {
		return fmt.Errorf("cannot verify agm-reaper revision: agm has no embedded VCS revision")
	}
	check := exec.Command(reaperPath, "--check-revision", expectedRevision)
	if out, err := check.CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("agm-reaper revision mismatch: %s", detail)
	}

	// Get sessions directory from config
	sessionsDir := cfg.SessionsDir
	startupRead, startupWrite, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create agm-reaper startup acknowledgement pipe: %w", err)
	}
	defer func() { _ = startupRead.Close() }()

	reaperLog, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		_ = startupWrite.Close()
		return fmt.Errorf("open agm-reaper log %s: %w", logFile, err)
	}

	// Build command with detachment
	reaperArgs := buildReaperArgs(sessionID, tmuxSession, logFile, sessionsDir, expectedRevision, forceArchive, keepSandbox, allowSupervisorReap, outcome)
	cmd := exec.Command(reaperPath, reaperArgs...)

	// Detach process from parent using setsid
	// This ensures the reaper survives even if the parent shell exits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}

	// The first inherited descriptor is fd 3. The child writes one readiness
	// record only after validating its revision and opening the durable log.
	cmd.ExtraFiles = []*os.File{startupWrite}
	cmd.Stdout = reaperLog
	cmd.Stderr = reaperLog
	cmd.Stdin = nil

	// Start the detached process, then wait only for its bounded startup
	// acknowledgement. Lifecycle work continues asynchronously after that gate.
	if err := cmd.Start(); err != nil {
		_ = startupWrite.Close()
		_ = reaperLog.Close()
		ui.PrintError(err,
			"Failed to spawn reaper process",
			fmt.Sprintf("  • Command: %s --session %s --log-file %s --sessions-dir %s\n"+
				"  • Check permissions: ls -l %s\n"+
				"  • Verify binary is executable: chmod +x %s\n"+
				"  • Test manually: %s --help",
				reaperPath, tmuxSession, logFile, sessionsDir, reaperPath, reaperPath, reaperPath))
		return fmt.Errorf("failed to start reaper: %w", err)
	}
	_ = startupWrite.Close()
	_ = reaperLog.Close()
	if err := awaitReaperStartup(startupRead, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("agm-reaper startup failed (log: %s): %w", logFile, err)
	}

	// Don't wait for process - it's detached
	pid := cmd.Process.Pid

	// Release process resources immediately to prevent zombie process
	// This is safe because the process is fully detached via setsid
	if err := cmd.Process.Release(); err != nil {
		// Log warning but don't fail - process is already running
		fmt.Fprintf(os.Stderr, "Warning: failed to release process resources: %v\n", err)
	}

	// Report success
	ui.PrintSuccess("Async archive started")
	fmt.Printf("\nReaper process spawned:\n")
	fmt.Printf("  PID: %d\n", pid)
	fmt.Printf("  Session ID: %s\n", sessionID)
	fmt.Printf("  Tmux session: %s\n", tmuxSession)
	fmt.Printf("  Log file: %s\n", logFile)
	fmt.Printf("\nThe reaper will:\n")
	fmt.Printf("  1. Wait for %s to return to prompt (smart detection, not fixed interval)\n", archiveHarnessDisplayName(harness))
	fmt.Printf("  2. Send %s command\n", reaper.GracefulExitCommand(harness))
	fmt.Printf("  3. Wait for pane to close\n")
	fmt.Printf("  4. Archive the session\n")
	fmt.Printf("\nMonitor progress: tail -f %s\n", logFile)

	return nil
}

// reaperLogPath confines the detached reaper log to the system temp directory
// even if a malformed tmux identity contains Unix or Windows path separators.
func reaperLogPath(tmuxSession string) string {
	sanitized := tmuxSession
	if idx := strings.LastIndex(sanitized, "/"); idx != -1 {
		sanitized = sanitized[idx+1:]
	}
	if idx := strings.LastIndex(sanitized, "\\"); idx != -1 {
		sanitized = sanitized[idx+1:]
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("agm-reaper-%s.log", filepath.Base(sanitized)))
}

func buildReaperArgs(sessionID, tmuxSession, logFile, sessionsDir, expectedRevision string, force, keepSandbox, allowSupervisorReap bool, outcome manifest.SessionOutcome) []string {
	args := []string{"--session-id", sessionID, "--session", tmuxSession, "--log-file", logFile, "--sessions-dir", sessionsDir, "--expected-revision", expectedRevision, "--startup-fd", "3"}
	if force {
		args = append(args, "--force")
	}
	if keepSandbox {
		args = append(args, "--keep-sandbox")
	}
	if allowSupervisorReap {
		args = append(args, "--allow-supervisor-reap")
	}
	if outcome != manifest.OutcomeUnknown {
		args = append(args, "--outcome", string(outcome))
	}
	return args
}

func awaitReaperStartup(reader *os.File, timeout time.Duration) error {
	result := make(chan error, 1)
	go func() {
		// A panic on this reader goroutine would take down the whole agm
		// process for what is only a startup handshake. Convert it into the
		// same bounded failure the caller already handles; the channel is
		// buffered, so this never blocks even after the timeout fired.
		defer func() {
			if r := recover(); r != nil {
				result <- fmt.Errorf("startup acknowledgement reader panicked: %v", r)
			}
		}()
		line, err := bufio.NewReader(reader).ReadString('\n')
		if err != nil {
			result <- fmt.Errorf("startup acknowledgement closed before readiness: %w", err)
			return
		}
		if line != "ready\n" {
			result <- fmt.Errorf("invalid startup acknowledgement %q", strings.TrimSpace(line))
			return
		}
		result <- nil
	}()

	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("startup acknowledgement timed out after %s", timeout)
	}
}
