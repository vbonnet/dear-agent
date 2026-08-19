// Package reaper provides reaper functionality.
package reaper

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const (
	// PromptDetectionTimeout is how long to wait for the agent to return to prompt
	// before falling back to timer-based waiting. This should be generous enough
	// to handle slow responses but not so long that stuck sessions block indefinitely.
	// Reduced to 90 seconds after fixing GetRawLine blocking bug. Most commands complete
	// within seconds; if no prompt after 90s, likely stuck and fallback should trigger.
	PromptDetectionTimeout = 90 * time.Second

	// PaneCloseTimeout is how long to wait for the tmux pane to close after sending
	// its native graceful-exit command. Agents should exit quickly after receiving
	// it, but we allow extra time
	// for cleanup operations and SessionEnd hooks.
	PaneCloseTimeout = 60 * time.Second

	// FallbackWaitTime is used when prompt detection fails or times out.
	// This is a conservative estimate of how long an agent might take to finish
	// a response and return to the prompt.
	// Reduced to 60 seconds to avoid excessive waiting for stuck sessions.
	FallbackWaitTime = 60 * time.Second

	// SIGTERMGracePeriod is how long to wait after sending SIGTERM before escalating to SIGKILL.
	SIGTERMGracePeriod = 10 * time.Second

	// PostKillPaneTimeout is how long to wait for pane to close after killing the process.
	PostKillPaneTimeout = 5 * time.Second

	// ReaperTimeout is the maximum wall-clock time a reaper may run before
	// it is considered a zombie itself. If exceeded, Phase 1 (graceful exit)
	// is abandoned: tmux is force-killed and the session is archived directly.
	ReaperTimeout = 10 * time.Minute
)

// Seams for the tmux + process boundary.
//
// These default to the real tmux/safety/syscall implementations and are
// overridden in tests so the reap loop (Run, stopProcess, sendExit,
// forceKillPaneProcess) can be exercised against a fake pane/process instead
// of a live tmux session. They are package-level function values rather than
// an injected interface so the production call graph stays exactly as it was —
// only the test seam is new. Tests must restore the originals (t.Cleanup).
var (
	checkSafetyFn      = safety.Check
	waitForPromptFn    = tmux.WaitForPromptSimple
	sendPromptSafeFn   = tmux.SendMultiLinePromptSafe
	waitForPaneCloseFn = tmux.WaitForPaneClose
	isPaneActiveFn     = tmux.IsPaneActive
	killSessionFn      = tmux.KillSessionChecked
	getPanePIDFn       = tmux.GetPanePID
	processKillFn      = syscall.Kill
	openStorageFn      = openStorage
)

// Reaper manages the async archival process for a AGM session.
// It waits for the harness to return to prompt, sends its native graceful-exit
// command, and archives the session.
type Reaper struct {
	SessionName string
	SessionsDir string
	SocketPath  string
	Options     ArchiveOptions
	logger      *slog.Logger
}

// ArchiveOptions are the lifecycle choices captured by the parent archive
// command and propagated into the detached reaper process.
type ArchiveOptions struct {
	SessionID           string
	Force               bool
	KeepSandbox         bool
	Outcome             manifest.SessionOutcome
	AllowSupervisorReap bool
}

// New creates a new Reaper for the given session
func New(sessionName, sessionsDir string) *Reaper {
	return NewWithOptions(sessionName, sessionsDir, ArchiveOptions{})
}

// NewWithOptions creates a Reaper with archive options preserved across the
// detached process boundary.
func NewWithOptions(sessionName, sessionsDir string, options ArchiveOptions) *Reaper {
	return &Reaper{
		SessionName: sessionName,
		SessionsDir: sessionsDir,
		SocketPath:  tmux.GetSocketPath(),
		Options:     options,
		// slog.Default(), not logging.DefaultLogger(): the latter is pinned to
		// os.Stderr, so every line of the reaping sequence bypassed the
		// --log-file the detached reaper is spawned with. ops.ArchiveSession
		// already logs through the process default, so the reaper's own phase
		// log was the only part of an async archive missing from its log file.
		logger: slog.Default(),
	}
}

// Run executes the full two-phase reaper sequence:
//
// Phase 1 — Stop the process (must complete before Phase 2):
//  1. Wait for the agent to return to prompt (prompt detection)
//  2. Send the harness-native graceful-exit command
//  3. Wait for pane to close (timeout: 60s)
//  4. If timeout: send SIGTERM to pane process, wait 10s
//  5. If still alive: send SIGKILL, wait 5s
//  6. Kill tmux session as final cleanup
//
// Phase 2 — Archive in Dolt (only after pane is confirmed dead):
//  7. Archive session (update Dolt + cleanup resources)
//
// This ordering prevents zombie panes: a session is never marked archived
// in Dolt while its tmux pane is still alive.
func (r *Reaper) Run() error {
	startTime := time.Now()
	r.logger.Info("Starting reaper sequence (two-phase)", "session", r.SessionName, "timeout", ReaperTimeout)

	// Step 0: Safety guard check - don't send a shutdown command if a human is present.
	guardResult := checkSafetyFn(r.SessionName, safety.GuardOptions{
		SkipUninitialized: true, // reaper only runs on initialized sessions
		SkipMidResponse:   true, // reaper already waits for prompt
	})
	if !guardResult.Safe {
		r.logger.Warn("Safety guard blocked reaper", "violations", guardResult.Error())
		return fmt.Errorf("safety guard blocked reaper on session '%s':\n%s", r.SessionName, guardResult.Error())
	}

	adapter, sessionsDir, err := r.openLifecycleStorage()
	if err != nil {
		return err
	}
	defer adapter.Close()

	// Validate the same supervisor, verification, and delegation guards used by
	// immediate and bulk archive before touching the pane. Active tmux is the
	// only allowed condition here because stopping that pane is the reaper's
	// purpose; the final shared operation checks again after pane death.
	preflight, err := r.preflightArchive(adapter)
	if err != nil {
		return fmt.Errorf("archive preflight failed: %w", err)
	}
	if preflight.AlreadyArchived {
		r.logger.Info("Session already archived, skipping reaper")
		return nil
	}

	// Step 1: Mark session as "reaping" in Dolt BEFORE touching tmux.
	// This ensures that if the reaper is killed mid-sequence (e.g., machine
	// restart), the GC startup scan will find the session in "reaping" state
	// with a dead tmux and archive it automatically.
	r.logger.Info("Marking session as reaping in Dolt")
	harness, markErr := r.markReaping(adapter, sessionsDir)
	if markErr != nil {
		// Non-fatal: proceed with reaping even if we can't mark state.
		// The GC startup scan will catch it as "stopped" anyway.
		r.logger.Warn("Failed to mark session as reaping (will proceed)", "error", markErr)
	}

	// ── Phase 1: Stop the process (gated by ReaperTimeout) ─────────────
	zombieDetected := r.stopProcess(startTime, harness)

	// Step 6: Kill tmux session as final cleanup (idempotent, always runs)
	r.logger.Info("Killing tmux session (final cleanup)")
	if err := killSessionFn(r.SessionName); err != nil {
		r.logger.Warn("tmux kill-session cleanup returned an error", "error", err)
	}

	// Verify pane is actually gone
	if active, _ := isPaneActiveFn(r.SessionName); active {
		r.logger.Error("CRITICAL: pane still alive after all kill attempts — refusing to archive")
		return fmt.Errorf("pane for session '%s' is still alive after SIGKILL + kill-session; refusing to archive to prevent zombie", r.SessionName)
	}

	// ── Phase 2: Archive in Dolt (pane is confirmed dead) ──────────────

	r.logger.Info("Archiving session (pane confirmed dead)")
	if err := r.archiveSessionWithStorage(adapter, sessionsDir); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	if zombieDetected {
		r.logger.Warn("Session force-archived due to reaper timeout (zombie detected)",
			"session", r.SessionName, "elapsed", time.Since(startTime))
	}
	r.logger.Info("Session archived successfully")

	return nil
}

// stopProcess runs Phase 1: graceful shutdown with timeout protection.
// Returns true if the ReaperTimeout was exceeded (zombie detected),
// meaning the graceful-exit command was skipped and tmux should be force-killed.
func (r *Reaper) stopProcess(startTime time.Time, harness string) bool {
	// Step 2: Wait for the agent to be ready (prompt detection)
	if remaining := r.timeRemaining(startTime); remaining > 0 {
		r.logger.Info("Waiting for agent to return to prompt")
		promptTimeout := min(PromptDetectionTimeout, remaining)
		if err := r.waitForPrompt(promptTimeout); err != nil {
			r.logger.Warn("Prompt detection failed", "error", err)
			fallback := min(FallbackWaitTime, r.timeRemaining(startTime))
			if fallback > 0 {
				r.logger.Info("Falling back to timer", "duration", fallback)
				time.Sleep(fallback)
			}
		} else {
			r.logger.Info("Prompt detected - agent is ready")
		}
	}

	// Check for zombie timeout
	if r.timeRemaining(startTime) <= 0 {
		r.logger.Warn("Reaper timeout exceeded, force-archiving (skipping graceful exit)",
			"elapsed", time.Since(startTime), "timeout", ReaperTimeout)
		return true
	}

	// Step 3: Send the harness-native graceful-exit command.
	exitCommand := GracefulExitCommand(harness)
	paneAlive := true
	r.logger.Info("Sending graceful exit command", "command", exitCommand, "harness", harness)
	if err := r.sendExit(exitCommand); err != nil {
		r.logger.Warn("Failed to send graceful exit command (session may have already exited)", "command", exitCommand, "error", err)
		if active, _ := isPaneActiveFn(r.SessionName); !active {
			paneAlive = false
			r.logger.Info("Pane already closed")
		}
	} else {
		r.logger.Info("Graceful exit command sent successfully", "command", exitCommand)
	}

	// Step 4: Wait for pane to close
	if !paneAlive {
		return false
	}
	remaining := r.timeRemaining(startTime)
	if remaining <= 0 {
		r.logger.Warn("Reaper timeout exceeded after graceful exit, force-archiving",
			"elapsed", time.Since(startTime), "timeout", ReaperTimeout)
		return true
	}

	paneTimeout := min(PaneCloseTimeout, remaining)
	r.logger.Info("Waiting for pane to close", "timeout", paneTimeout)
	if err := r.waitForPaneClose(paneTimeout); err != nil {
		r.logger.Warn("Pane did not close after graceful exit, escalating to SIGTERM", "command", exitCommand, "error", err)
		// Step 5: SIGTERM → wait → SIGKILL escalation
		r.forceKillPaneProcess()
	} else {
		r.logger.Info("Pane closed successfully after graceful exit", "command", exitCommand)
	}

	return false
}

// timeRemaining returns the time remaining before ReaperTimeout is exceeded.
// Returns <= 0 if the timeout has been exceeded.
func (r *Reaper) timeRemaining(startTime time.Time) time.Duration {
	return ReaperTimeout - time.Since(startTime)
}

// forceKillPaneProcess escalates from SIGTERM to SIGKILL to ensure the
// pane process is dead. Called when graceful exit plus wait times out.
func (r *Reaper) forceKillPaneProcess() {
	// Get the PID of the process in the pane
	pid, err := getPanePIDFn(r.SessionName)
	if err != nil {
		r.logger.Warn("Could not get pane PID (pane may already be gone)", "error", err)
		return
	}

	r.logger.Info("Sending SIGTERM to pane process", "pid", pid)
	if err := processKillFn(pid, syscall.SIGTERM); err != nil {
		r.logger.Warn("SIGTERM failed (process may already be gone)", "pid", pid, "error", err)
		return
	}

	// Wait for process to exit after SIGTERM
	r.logger.Info("Waiting for process to exit after SIGTERM", "grace_period", SIGTERMGracePeriod)
	if err := r.waitForPaneClose(SIGTERMGracePeriod); err == nil {
		r.logger.Info("Process exited after SIGTERM")
		return
	}

	// Escalate to SIGKILL
	r.logger.Warn("Process did not exit after SIGTERM, sending SIGKILL", "pid", pid)
	if err := processKillFn(pid, syscall.SIGKILL); err != nil {
		r.logger.Warn("SIGKILL failed", "pid", pid, "error", err)
	}

	// Brief wait for pane to close after SIGKILL
	if err := r.waitForPaneClose(PostKillPaneTimeout); err != nil {
		r.logger.Warn("Pane still alive after SIGKILL — will attempt tmux kill-session", "error", err)
	} else {
		r.logger.Info("Process killed with SIGKILL")
	}
}

// markReaping updates the session lifecycle to "reaping" in Dolt.
// This must happen BEFORE killing tmux so that if the reaper crashes,
// the GC startup scan can detect and archive the orphaned session.
func (r *Reaper) markReaping(adapter *dolt.Adapter, sessionsDir string) (string, error) {
	m, _, err := session.ResolveIdentifier(r.archiveIdentifier(), sessionsDir, adapter)
	if err != nil {
		return "", fmt.Errorf("session not found: %w", err)
	}

	if m.Lifecycle == manifest.LifecycleArchived {
		return m.Harness, nil // already archived, nothing to do
	}

	m.Lifecycle = manifest.LifecycleReaping
	if err := adapter.UpdateSession(m); err != nil {
		return m.Harness, fmt.Errorf("failed to update session in Dolt: %w", err)
	}

	r.logger.Info("Session marked as reaping in Dolt")
	return m.Harness, nil
}

func (r *Reaper) preflightArchive(adapter *dolt.Adapter) (*ops.ArchiveSessionResult, error) {
	opCtx := &ops.OpContext{Storage: adapter, DryRun: true}
	req := r.archiveRequest()
	req.AllowActiveTmux = true
	req.Idempotent = true
	return ops.ArchiveSession(opCtx, &req)
}

// waitForPrompt monitors output stream for the agent prompt.
// Uses tmux control mode to detect when the pane is ready for input.
func (r *Reaper) waitForPrompt(timeout time.Duration) error {
	return waitForPromptFn(r.SessionName, timeout)
}

// GracefulExitCommand owns the native shutdown command selected for a harness.
// Pi removed /exit in favor of /quit; the remaining interactive harnesses retain
// the existing /exit contract. Legacy Pi aliases are accepted for old manifests.
func GracefulExitCommand(harness string) string {
	switch strings.ToLower(strings.TrimSpace(harness)) {
	case "pi", "pi-cli":
		return "/quit"
	default:
		return "/exit"
	}
}

// sendExit sends one already-selected native command to exit the harness cleanly.
// It avoids the sender attribution header that agm send adds.
func (r *Reaper) sendExit(exitCommand string) error {
	// Use tmux.SendMultiLinePromptSafe to send the command literally. This function:
	// 1. Waits for the agent prompt (handles busy pane states)
	// 2. Sends ESC to interrupt any thinking (shouldInterrupt=true)
	// 3. Sends the native exit command in literal mode (not as a message)
	// 4. Sends Enter to execute
	if err := sendPromptSafeFn(r.SessionName, exitCommand, true); err != nil {
		return fmt.Errorf("failed to send %s: %w", exitCommand, err)
	}

	r.logger.Info("Graceful exit sent via SendMultiLinePromptSafe", "command", exitCommand)
	return nil
}

// waitForPaneClose waits for tmux pane to close
func (r *Reaper) waitForPaneClose(timeout time.Duration) error {
	return waitForPaneCloseFn(r.SessionName, timeout)
}

// archiveSession delegates the durable lifecycle transition and all archive
// side effects to ops.ArchiveSession. The reaper owns only the preceding
// process-stop phase and reaping tombstone.
func (r *Reaper) archiveSession() error {
	adapter, sessionsDir, err := r.openLifecycleStorage()
	if err != nil {
		return err
	}
	defer adapter.Close()
	return r.archiveSessionWithStorage(adapter, sessionsDir)
}

func (r *Reaper) openLifecycleStorage() (*dolt.Adapter, string, error) {
	sessionsDir, err := r.getSessionsDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get sessions directory: %w", err)
	}
	adapter, err := openStorageFn()
	if err != nil {
		return nil, "", err
	}
	if err := adapter.ApplyMigrations(); err != nil {
		_ = adapter.Close()
		return nil, "", fmt.Errorf("failed to apply Dolt migrations: %w", err)
	}
	return adapter, sessionsDir, nil
}

func (r *Reaper) archiveSessionWithStorage(adapter *dolt.Adapter, sessionsDir string) error {
	req := r.archiveRequest()
	req.Idempotent = true
	req.LegacySessionsDir = sessionsDir
	result, err := ops.ArchiveSession(&ops.OpContext{Storage: adapter}, &req)
	if err != nil {
		return err
	}
	for _, outcome := range result.ExternalArchives {
		if outcome.Status == ops.ExternalArchiveFailed {
			r.logger.Warn("External session archive failed after AGM archival", "session", result.SessionID, "provider", outcome.Provider, "error", outcome.Detail)
		}
	}
	if result.AlreadyArchived {
		r.logger.Info("Session already archived, skipping duplicate cleanup")
	}
	return nil
}

func (r *Reaper) archiveRequest() ops.ArchiveSessionRequest {
	return ops.ArchiveSessionRequest{
		Identifier:          r.archiveIdentifier(),
		Force:               r.Options.Force,
		KeepSandbox:         r.Options.KeepSandbox,
		Outcome:             r.Options.Outcome,
		AllowSupervisorReap: r.Options.AllowSupervisorReap,
	}
}

func (r *Reaper) archiveIdentifier() string {
	if r.Options.SessionID != "" {
		return r.Options.SessionID
	}
	return r.SessionName
}

// openStorage selects the same isolated SQLite lifecycle store used by AGM
// commands when a reaper is spawned from a named test environment. Reapers are
// separate processes, so ignoring AGM_DB_PATH here would make --async mark and
// archive operations fall back to production Dolt instead of the test session.
func openStorage() (*dolt.Adapter, error) {
	if dbPath := os.Getenv("AGM_DB_PATH"); dbPath != "" {
		adapter, err := dolt.NewSQLiteAdapter(dbPath)
		if err != nil {
			return nil, fmt.Errorf("open test SQLite storage: %w", err)
		}
		return adapter, nil
	}

	config, err := dolt.DefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Dolt config: %w", err)
	}
	adapter, err := dolt.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Dolt: %w", err)
	}
	return adapter, nil
}

// getSessionsDir returns the configured sessions directory path
// Falls back to ~/sessions if not configured
func (r *Reaper) getSessionsDir() (string, error) {
	// Use configured directory if provided
	if r.SessionsDir != "" {
		return r.SessionsDir, nil
	}

	// Fall back to default ~/.claude/sessions
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".claude", "sessions"), nil
}
