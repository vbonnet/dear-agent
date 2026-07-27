package ops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/cleanup"
	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/delegation"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/sandboxgc"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	inttmux "github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const archiveSandboxCleanupAttempts = 5

// ArchiveSessionRequest defines the input for archiving a session.
type ArchiveSessionRequest struct {
	// Identifier is a session ID, name, or UUID prefix.
	Identifier string `json:"identifier"`
	// Force skips pre-archive verification checks.
	Force bool `json:"force,omitempty"`
	// KeepSandbox preserves the sandbox directory for debugging instead of removing it.
	KeepSandbox bool `json:"keep_sandbox,omitempty"`
	// Outcome records why the session is being archived (completed|crashed|
	// killed|gc-stale). When empty, ArchiveSession defaults it to
	// manifest.OutcomeCompleted so every archived record is triage-legible.
	Outcome manifest.SessionOutcome `json:"outcome,omitempty"`
	// AllowSupervisorReap bypasses the supervisor-protection guard (but not the
	// active-tmux or verification guards). Set only by typed recovery paths that
	// already proved the supervisor is safe to reap: GC for stopped protected-role
	// orphans, and async supervisor auth recovery where the parent preflight still
	// sets AllowActiveTmux and the detached reaper performs the final archive only
	// after the pane exits.
	AllowSupervisorReap bool `json:"allow_supervisor_reap,omitempty"`
	// AllowActiveTmux permits an async reaper preflight to validate every other
	// archive guard while the pane is intentionally still alive. The final
	// archive call does not set this field and still enforces the active-pane
	// guard. This is internal process coordination, not an API-surface option.
	AllowActiveTmux bool `json:"-"`
	// Idempotent treats an already-archived record as a successful no-op. Async
	// reapers use this for crash recovery; interactive surfaces retain the
	// existing already-archived error and user guidance.
	Idempotent bool `json:"-"`
	// LegacySessionsDir identifies the old filesystem store where a reaper may
	// need to move the resolved session ID under .archive-old-format. It is
	// internal process context and intentionally excluded from JSON APIs.
	LegacySessionsDir string `json:"-"`
}

// ArchiveSessionResult is the output of ArchiveSession.
type ArchiveSessionResult struct {
	Operation           string                          `json:"operation"`
	SessionID           string                          `json:"session_id"`
	Name                string                          `json:"name"`
	PreviousStatus      string                          `json:"previous_status"`
	Outcome             manifest.SessionOutcome         `json:"outcome,omitempty"`
	DryRun              bool                            `json:"dry_run,omitempty"`
	Verification        *session.CompletionVerification `json:"verification,omitempty"`
	MCPProcessesCleaned int                             `json:"mcp_processes_cleaned,omitempty"`
	SandboxCleaned      bool                            `json:"sandbox_cleaned,omitempty"`
	PendingDelegations  int                             `json:"pending_delegations,omitempty"`
	PostCleanup         *CleanupResult                  `json:"post_cleanup,omitempty"`
	ExternalArchives    []ExternalArchiveOutcome        `json:"external_archives,omitempty"`
	SessionCleanup      *cleanup.Result                 `json:"session_cleanup,omitempty"`
	LegacyArchivePath   string                          `json:"legacy_archive_path,omitempty"`
	AlreadyArchived     bool                            `json:"already_archived,omitempty"`
}

// ArchiveSession marks a session as archived.
// If ctx.DryRun is true, returns what would happen without executing.
// If verification finds critical issues and Force is false, returns an error.
func ArchiveSession(ctx *OpContext, req *ArchiveSessionRequest) (*ArchiveSessionResult, error) {
	if req == nil || req.Identifier == "" {
		return nil, ErrInvalidInput("identifier", "Session identifier is required. Provide a session ID, name, or UUID prefix.")
	}
	if ctx == nil || ctx.Storage == nil {
		return nil, ErrStorageError("archive_session", fmt.Errorf("storage is required"))
	}

	// Resolve session
	m, err := ctx.Storage.GetSession(req.Identifier)
	if err != nil {
		// Try name-based lookup
		m, err = findByName(ctx, req.Identifier)
		if err != nil {
			return nil, err
		}
	}
	if m == nil {
		return nil, ErrSessionNotFound(req.Identifier)
	}

	if m.SessionID == "" {
		return nil, ErrStorageError("archive_session", fmt.Errorf("resolved session has no stable session ID"))
	}

	// Archive and API delivery share the stable session-ID mutation boundary.
	// Reloading after acquisition makes the lifecycle decision authoritative:
	// either an in-flight paid completion commits before archive, or archive wins
	// and a later delivery observes the archived lifecycle before provider work.
	requestCtx := archiveOperationContext(ctx)
	lock := WithSessionLockContext
	if isAPISessionManifest(m) {
		lock = WithAPISessionLockContext
	}
	var result *ArchiveSessionResult
	err = lock(requestCtx, m.SessionID, func() error {
		if err := requestCtx.Err(); err != nil {
			return err
		}
		current, err := ctx.Storage.GetSession(m.SessionID)
		if err != nil {
			return ErrStorageError("archive_session_reload", err)
		}
		if current == nil {
			return ErrSessionNotFound(m.SessionID)
		}
		result, err = archiveResolvedSession(ctx, current, req)
		return err
	})
	return result, err
}

func isAPISessionManifest(m *manifest.Manifest) bool {
	if m == nil {
		return false
	}
	return m.Harness == "openai" || m.Harness == "gpt"
}

// archiveResolvedSession validates and mutates one current session snapshot
// while its stable-ID lifecycle lock is held.
func archiveResolvedSession(ctx *OpContext, m *manifest.Manifest, req *ArchiveSessionRequest) (*ArchiveSessionResult, error) {
	// Check if already archived
	if m.Lifecycle == manifest.LifecycleArchived {
		if req.Idempotent {
			return &ArchiveSessionResult{
				Operation:       "archive_session",
				SessionID:       m.SessionID,
				Name:            m.Name,
				PreviousStatus:  "archived",
				Outcome:         m.Outcome,
				AlreadyArchived: true,
			}, nil
		}
		return nil, ErrSessionArchived(m.Name)
	}

	if err := checkSupervisorProtection(m, req.Force || req.AllowSupervisorReap); err != nil {
		return nil, err
	}
	if err := checkActiveTmuxBlock(m, req.Force || req.AllowActiveTmux); err != nil {
		return nil, err
	}

	previousStatus := computeSessionStatus(m, ctx.Tmux)
	verification := runArchiveVerification(m)
	if err := blockOnVerification(verification, req.Force); err != nil {
		return nil, err
	}
	if err := blockOnPendingDelegations(m, req.Force); err != nil {
		return nil, err
	}

	outcome, err := normalizeArchiveOutcome(req.Outcome)
	if err != nil {
		return nil, err
	}

	// Dry run: return what would happen
	if ctx.DryRun {
		return &ArchiveSessionResult{
			Operation:      "archive_session",
			SessionID:      m.SessionID,
			Name:           m.Name,
			PreviousStatus: previousStatus,
			DryRun:         true,
			Verification:   verification,
			Outcome:        outcome,
		}, nil
	}

	m.Lifecycle = manifest.LifecycleArchived
	// Stamp the outcome so the archived record is triage-legible. Default to
	// "completed" — the common archive path is a session that finished normally.
	m.Outcome = outcome
	m.UpdatedAt = time.Now()
	if err := ctx.Storage.UpdateSession(m); err != nil {
		return nil, ErrStorageError("archive_session", err)
	}

	mcpKilled, postCleanup, sessionCleanup, legacyArchivePath := runArchiveCleanup(ctx, m, req, verification.HasOpenPR)
	externalArchives := archiveExternalForContext(ctx, m)
	for _, outcome := range externalArchives {
		if outcome.Status == ExternalArchiveFailed {
			slog.Warn("External session archive failed after AGM archival", "session", m.SessionID, "provider", outcome.Provider, "error", outcome.Detail)
		}
	}

	return &ArchiveSessionResult{
		Operation:           "archive_session",
		SessionID:           m.SessionID,
		Name:                m.Name,
		PreviousStatus:      previousStatus,
		Outcome:             outcome,
		Verification:        verification,
		MCPProcessesCleaned: mcpKilled,
		SandboxCleaned:      postCleanup.SandboxRemoved,
		PostCleanup:         postCleanup,
		ExternalArchives:    externalArchives,
		SessionCleanup:      sessionCleanup,
		LegacyArchivePath:   legacyArchivePath,
	}, nil
}

func normalizeArchiveOutcome(outcome manifest.SessionOutcome) (manifest.SessionOutcome, error) {
	switch outcome {
	case manifest.OutcomeUnknown:
		return manifest.OutcomeCompleted, nil
	case manifest.OutcomeCompleted, manifest.OutcomeCrashed, manifest.OutcomeKilled, manifest.OutcomeGCStale:
		return outcome, nil
	default:
		return manifest.OutcomeUnknown, ErrInvalidInput("outcome", "Archive outcome must be one of: completed, crashed, killed, gc-stale.")
	}
}

// runArchiveCleanup performs the post-update cleanup steps (trust event,
// monitor deregister, MCP processes, tmux process group, worktree/sandbox
// cleanup, additionalDirectories removal). Returns (mcpKilled, postCleanup).
//
// hasOpenPR is the verification result's PR-awareness signal (fix for
// ce-93lw.27 gap #4): it gates whether the local session branch is
// force-deleted, so a branch with a confirmed open PR survives cleanup
// instead of being stripped out from under in-flight work.
func runArchiveCleanup(ctx *OpContext, m *manifest.Manifest, req *ArchiveSessionRequest, hasOpenPR bool) (int, *CleanupResult, *cleanup.Result, string) {
	legacyManifestPath := ""
	if req.LegacySessionsDir != "" {
		legacyManifestPath = filepath.Join(req.LegacySessionsDir, m.SessionID, "manifest.yaml")
	}
	if shouldSkipHostArchiveCleanup(ctx.Storage, m) {
		slog.Info("Skipping host resource cleanup for test session", "session", m.SessionID)
		return 0, &CleanupResult{}, nil, archiveLegacySessionDir(req.LegacySessionsDir, legacyManifestPath)
	}

	recordArchiveTrust(m.Name, m.WorkingDirectory, m.Context.Project, m.SessionID, m.CreatedAt)
	deregisterMonitor(ctx, m.Name)

	sandboxPath := ownedSandboxPathForArchive(m)
	mcpKilled, mcpErr := mcp.CleanupSessionMCPProcesses(
		&mcp.ProcFSFinder{}, &mcp.SignalKiller{},
		m.SessionID, sandboxPath,
	)
	if mcpErr != nil {
		slog.Warn("MCP cleanup error during archive", "session", m.SessionID, "error", mcpErr)
	}
	if mcpKilled > 0 {
		slog.Info("Cleaned up MCP processes during archive", "session", m.SessionID, "killed", mcpKilled)
	}

	killTmuxAndProcessGroup(m)

	cleanSandbox := sandboxCleanupFunc(cleanupSandboxDir)
	if ctx.archiveSandboxCleaner != nil {
		cleanSandbox = ctx.archiveSandboxCleaner
	}
	postCleanup := cleanupAfterArchive(
		m.SessionID, m.Name,
		m.WorkingDirectory, m.Context.Project, sandboxPath, m.Name,
		req.KeepSandbox, hasOpenPR, cleanSandbox,
	)
	sessionCleanup := cleanupTrackedSessionResources(ctx, m.Name)
	cleanupPendingDir(m.Name)
	if postCleanup.SandboxRemoved {
		logGCEntry(gclog.Entry{
			Operation:   "archive_sandbox_cleanup",
			SessionID:   m.SessionID,
			SessionName: m.Name,
		})
	}
	if sandboxPath != "" {
		if err := removeFromAdditionalDirectories(sandboxPath); err != nil {
			slog.Warn("Failed to remove sandbox from additionalDirectories", "session", m.SessionID, "path", sandboxPath, "error", err)
		}
	}
	legacyArchivePath := archiveLegacySessionDir(req.LegacySessionsDir, legacyManifestPath)
	return mcpKilled, postCleanup, sessionCleanup, legacyArchivePath
}

func shouldSkipHostArchiveCleanup(storage dolt.Storage, m *manifest.Manifest) bool {
	if m != nil && m.IsTest {
		return true
	}
	adapter, ok := storage.(*dolt.Adapter)
	return ok && adapter != nil && adapter.IsTestStore()
}

func cleanupTrackedSessionResources(ctx *OpContext, sessionName string) *cleanup.Result {
	adapter, ok := ctx.Storage.(*dolt.Adapter)
	if !ok || adapter == nil || adapter.IsTestStore() {
		return nil
	}
	store := &cleanup.DoltWorktreeStore{Adapter: adapter}
	return cleanup.SessionResources(archiveOperationContext(ctx), sessionName, store, cleanup.RealGitOps{}, slog.Default())
}

func archiveOperationContext(opCtx *OpContext) context.Context {
	if opCtx != nil && opCtx.Context != nil {
		return opCtx.Context
	}
	return context.Background()
}

func cleanupPendingDir(sessionName string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	pendingDir := filepath.Join(homeDir, ".agm", "pending", sessionName)
	if _, err := os.Stat(pendingDir); err != nil {
		return
	}
	if err := os.RemoveAll(pendingDir); err != nil {
		slog.Warn("Failed to remove pending directory", "path", pendingDir, "error", err)
		return
	}
	slog.Info("Cleaned up pending message directory", "path", pendingDir)
}

func archiveLegacySessionDir(sessionsDir, manifestPath string) string {
	if sessionsDir == "" || manifestPath == "" {
		return ""
	}
	base, err := filepath.Abs(sessionsDir)
	if err != nil {
		slog.Warn("Could not resolve legacy sessions directory", "path", sessionsDir, "error", err)
		return ""
	}
	sessionDir, err := filepath.Abs(filepath.Dir(manifestPath))
	if err != nil {
		slog.Warn("Could not resolve legacy session directory", "path", manifestPath, "error", err)
		return ""
	}
	rel, err := filepath.Rel(base, sessionDir)
	if err != nil || rel == "." || rel == ".archive-old-format" || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.Contains(rel, string(filepath.Separator)) {
		slog.Warn("Refusing legacy session move outside direct sessions child", "sessions_dir", base, "session_dir", sessionDir)
		return ""
	}
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		slog.Info("No filesystem directory to move (pure-Dolt session)", "path", sessionDir)
		return ""
	} else if err != nil {
		slog.Warn("Could not stat legacy session directory", "path", sessionDir, "error", err)
		return ""
	}

	archiveBaseDir := filepath.Join(base, ".archive-old-format")
	archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))
	if err := os.MkdirAll(archiveBaseDir, 0o700); err != nil {
		slog.Warn("Failed to create legacy archive directory", "error", err)
		return ""
	}
	if _, err := os.Stat(archiveTargetDir); err == nil {
		archiveTargetDir += "-" + time.Now().UTC().Format("20060102T150405Z")
		slog.Warn("Legacy archive conflict; using timestamped target", "target", filepath.Base(archiveTargetDir))
	}
	if err := os.Rename(sessionDir, archiveTargetDir); err != nil {
		slog.Warn("Failed to move legacy session directory to archive", "error", err)
		return ""
	}
	slog.Info("Moved legacy session directory to archive", "path", archiveTargetDir)
	return archiveTargetDir
}

// checkSupervisorProtection blocks archive of supervisor sessions unless force.
func checkSupervisorProtection(m *manifest.Manifest, force bool) error {
	if force || !IsSupervisorSession(m.Name) {
		return nil
	}
	return &OpError{
		Status:      403,
		Type:        "archive/supervisor_protected",
		Code:        ErrCodeVerificationFailed,
		Title:       "Cannot archive protected supervisor session",
		Detail:      fmt.Sprintf("Session '%s' is a protected supervisor session. Use --force to override.", m.Name),
		Suggestions: []string{"use --force to override supervisor protection"},
	}
}

// checkActiveTmuxBlock blocks archive when the session has an active tmux pane.
//
// The tmux session name is resolved via session.TmuxSessionName, the SAME helper
// status computation uses: when m.Tmux.SessionName is empty it falls back to the
// sanitized session Name. Previously this returned nil (no block) whenever the
// explicit field was empty, but status computation still matched the fallback name
// — so a live detached pane could report "active" yet be archived. Resolving the
// name the same way here closes that bypass.
func checkActiveTmuxBlock(m *manifest.Manifest, force bool) error {
	if force {
		return nil
	}
	tmuxSessionName := session.TmuxSessionName(m)
	if tmuxSessionName == "" {
		return nil
	}
	socketPath := inttmux.GetSocketPath()
	cmd := exec.Command("tmux", "-S", socketPath, "has-session", "-t", tmuxSessionName)
	if err := cmd.Run(); err != nil {
		return nil //nolint:nilerr // tmux has-session failure means session doesn't exist; nothing to block
	}
	return &OpError{
		Status:      400,
		Type:        "archive/active_tmux_session",
		Code:        ErrCodeVerificationFailed,
		Title:       "Cannot archive session with active tmux pane",
		Detail:      "cannot archive session with active tmux pane — use --force to override",
		Suggestions: []string{"use --force to override and archive anyway"},
	}
}

// runArchiveVerification runs completion verification on the session's working
// directory (or context project as fallback).
func runArchiveVerification(m *manifest.Manifest) *session.CompletionVerification {
	dir := m.WorkingDirectory
	if dir == "" {
		dir = m.Context.Project
	}
	return session.VerifyCompletion(dir)
}

// blockOnVerification returns an error if verification flagged critical issues
// and force is not set.
func blockOnVerification(verification *session.CompletionVerification, force bool) error {
	if !verification.Critical() || force {
		return nil
	}
	errs := verification.CriticalErrors()
	detail := fmt.Sprintf("Cannot archive: %s. Fix and retry, or use --force to override.", strings.Join(errs, "; "))
	return &OpError{
		Status:      400,
		Type:        "archive/verification_failed",
		Code:        ErrCodeVerificationFailed,
		Title:       "Pre-archive verification failed",
		Detail:      detail,
		Suggestions: append(errs, "use --force to skip verification checks"),
	}
}

// blockOnPendingDelegations errors when this session has unresolved
// delegations (caller can pass force=true to override; warning is still logged).
func blockOnPendingDelegations(m *manifest.Manifest, force bool) error {
	delegationDir, err := delegation.DefaultDir()
	if err != nil {
		return nil //nolint:nilerr // best-effort: missing delegation dir doesn't block archive
	}
	tracker, err := delegation.NewTracker(delegationDir)
	if err != nil {
		return nil //nolint:nilerr // best-effort: tracker init failure doesn't block archive
	}
	pending, err := tracker.Pending(m.Name)
	if err != nil || len(pending) == 0 {
		return nil //nolint:nilerr // best-effort: tracker query failure or no pending = don't block
	}
	summaries := make([]string, 0, len(pending))
	for _, d := range pending {
		s := d.TaskSummary
		if len(s) > 80 {
			s = s[:77] + "..."
		}
		summaries = append(summaries, fmt.Sprintf("→ %s: %s [ID: %s]", d.To, s, d.MessageID))
	}
	if force {
		slog.Warn("Archiving with pending delegations", "session", m.Name, "count", len(pending))
		return nil
	}
	detail := fmt.Sprintf("Cannot archive: %d pending delegation(s) have not been resolved:\n%s\n\nResolve with: agm delegation resolve-all %s\nOr use --force to override.",
		len(pending), strings.Join(summaries, "\n"), m.Name)
	return &OpError{
		Status:      400,
		Type:        "archive/pending_delegations",
		Code:        ErrCodeVerificationFailed,
		Title:       "Pending delegations block archive",
		Detail:      detail,
		Suggestions: []string{"resolve delegations first", "use --force to skip"},
	}
}

// killTmuxAndProcessGroup terminates the session's tmux process group and the
// tmux session itself (best-effort).
func killTmuxAndProcessGroup(m *manifest.Manifest) {
	if m.Tmux.SessionName == "" {
		return
	}
	if pidOut, err := exec.Command("tmux", "display-message", "-t", m.Tmux.SessionName, "-p", "#{pane_pid}").Output(); err == nil {
		if pid := strings.TrimSpace(string(pidOut)); pid != "" {
			exec.Command("kill", "-TERM", "-"+pid).Run()
			time.Sleep(contracts.Load().SessionLifecycle.ProcessKillGracePeriod.Duration)
			slog.Info("Killed process group during archive", "session", m.SessionID, "pane_pid", pid)
		}
	}
	exec.Command("tmux", "kill-session", "-t", m.Tmux.SessionName).Run()
	slog.Info("Killed tmux session during archive", "session", m.SessionID, "tmux", m.Tmux.SessionName)
}

// cleanupSandboxDir unmounts and removes the sandbox directory for a session.
// Preserves .claude/settings.local.json from the overlay upper layer before
// removal, as it contains RBAC permission rules that should not be lost.
//
// Removal goes through the sandboxgc safety gates (ce-uxju): allowlist path
// validation, no live process inside, and — after a best-effort unmount — no
// mount point left inside (deleting through a live overlay mount can destroy
// the source repo, per ~/.agm/cleanup-runbook.md). The live-session gate is
// intentionally nil here: the caller archived this session immediately before
// invoking cleanup. A specific live-process refusal is retried for a bounded
// grace window because a just-terminated harness may briefly retain the
// sandbox as it exits; every retry re-runs all checker safety gates. Every
// other refusal keeps the sandbox for the periodic `agm sandbox gc` sweep to
// retry once the blocker is gone.
//
// Returns (removed, existed, reason): existed is false only when there was
// no sandbox directory to remove in the first place (not a failure); every
// other refusal or removal failure reports existed=true so callers can
// distinguish "nothing to do" from a real, worth-surfacing failure. reason
// carries the underlying refusal so the cleanup audit log records what
// actually blocked the reap, rather than a generic line that sends the
// operator to a log file with no detail in it.
func cleanupSandboxDir(sessionID, mergedPath string) (removed bool, existed bool, reason string) {
	base, err := sandboxgc.DefaultBase()
	if err != nil {
		slog.Warn("Failed to get home dir for sandbox cleanup", "error", err)
		// We cannot even determine whether a sandbox exists — treat as a
		// real failure rather than silently reporting "nothing to remove".
		return false, true, fmt.Sprintf("cannot resolve the sandbox base directory: %v", err)
	}
	return cleanupSandboxDirWithChecker(sessionID, mergedPath, base, sandboxgc.NewChecker(base, nil))
}

func cleanupSandboxDirWithChecker(sessionID, mergedPath, base string, checker *sandboxgc.Checker) (removed bool, existed bool, reason string) {
	return cleanupSandboxDirWithCheckerAndRetry(
		sessionID,
		mergedPath,
		base,
		checker,
		archiveSandboxCleanupAttempts,
		contracts.Load().SessionLifecycle.ProcessKillGracePeriod.Duration,
		time.Sleep,
		time.Now,
	)
}

func cleanupSandboxDirWithCheckerAndRetry(
	sessionID, mergedPath, base string,
	checker *sandboxgc.Checker,
	attempts int,
	retryDelay time.Duration,
	sleep func(time.Duration),
	now func() time.Time,
) (removed bool, existed bool, reason string) {
	sandboxDir := filepath.Join(base, sessionID)
	if checker == nil || filepath.Clean(checker.Base) != filepath.Clean(base) {
		slog.Warn("Refusing sandbox cleanup with a mismatched safety checker", "session", sessionID)
		return false, true, "safety checker base does not match the cleanup base"
	}
	if err := sandboxgc.ValidateSandboxPath(base, sandboxDir); err != nil {
		slog.Warn("Refusing sandbox cleanup outside the allowlisted base", "session", sessionID, "error", err)
		return false, true, fmt.Sprintf("sandbox path is outside the allowlisted base: %v", err)
	}
	expectedMergedPath := filepath.Join(sandboxDir, "merged")
	if mergedPath != expectedMergedPath {
		slog.Warn("Refusing sandbox cleanup with an unattributed merged path",
			"session", sessionID, "path", mergedPath, "expected", expectedMergedPath)
		return false, true, fmt.Sprintf("merged path %s is not the expected %s", mergedPath, expectedMergedPath)
	}
	if _, err := os.Lstat(sandboxDir); os.IsNotExist(err) {
		return false, false, ""
	}

	// Preserve .claude/settings.local.json from the upper layer before removal.
	// This file contains RBAC permission rules written by ConfigureProjectPermissions.
	preserveSettingsFromUpper(sandboxDir)

	// Unmount the validated merged path before checker.Reap retries the default path.
	if err := checker.Unmount(mergedPath); err != nil {
		slog.Warn("Failed to unmount sandbox", "path", mergedPath, "error", err)
	}

	return reapSandboxWithRetry(sessionID, sandboxDir, checker, attempts, retryDelay, sleep, now)
}

func reapSandboxWithRetry(
	sessionID, sandboxDir string,
	checker *sandboxgc.Checker,
	attempts int,
	retryDelay time.Duration,
	sleep func(time.Duration),
	now func() time.Time,
) (removed bool, existed bool, reason string) {
	if attempts < 1 {
		attempts = 1
	}
	if retryDelay <= 0 {
		attempts = 1
	}
	retryDeadline := now().Add(time.Duration(attempts) * retryDelay)
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 && !now().Before(retryDeadline) {
			slog.Warn("Sandbox not removed before archive cleanup grace deadline — periodic sandbox gc will retry",
				"session", sessionID, "path", sandboxDir, "attempts", attempt-1,
				"retry_deadline", retryDeadline)
			return false, true, "sandbox cleanup grace deadline exceeded"
		}
		err := checker.Reap(sandboxDir)
		if err == nil {
			slog.Info("Removed sandbox directory", "session", sessionID, "path", sandboxDir)
			return true, true, ""
		}
		if attempt == attempts || !retryableArchiveSandboxRefusal(err) {
			slog.Warn("Sandbox not removed during archive cleanup — periodic sandbox gc will retry",
				"session", sessionID, "path", sandboxDir, "attempts", attempt, "error", err)
			return false, true, err.Error()
		}
		remaining := retryDeadline.Sub(now())
		if remaining <= 0 {
			slog.Warn("Sandbox not removed before archive cleanup grace deadline — periodic sandbox gc will retry",
				"session", sessionID, "path", sandboxDir, "attempts", attempt,
				"retry_deadline", retryDeadline, "error", err)
			return false, true, err.Error()
		}
		delay := min(retryDelay, remaining)
		slog.Info("Waiting for transient sandbox holder to exit before retrying cleanup",
			"session", sessionID, "path", sandboxDir, "attempt", attempt, "max_attempts", attempts,
			"retry_delay", delay, "retry_deadline", retryDeadline, "error", err)
		sleep(delay)
	}

	return false, true, "exhausted sandbox cleanup retries"
}

func retryableArchiveSandboxRefusal(err error) bool {
	var refusal *sandboxgc.RefusalError
	return errors.As(err, &refusal) &&
		refusal.Reason == sandboxgc.ReasonLiveProcess &&
		refusal.ProcessID > 0
}

func ownedSandboxPathForArchive(m *manifest.Manifest) string {
	if m == nil || m.Sandbox == nil {
		return ""
	}
	if err := manifest.ValidateSandboxOwnership(m.SessionID, m.Sandbox); err != nil {
		slog.Warn("Ignoring invalid sandbox ownership metadata during archive",
			"session", m.SessionID, "error", err)
		return ""
	}
	base, err := sandboxgc.DefaultBase()
	if err != nil {
		slog.Warn("Ignoring sandbox ownership without a resolvable cleanup base",
			"session", m.SessionID, "error", err)
		return ""
	}
	sandboxDir := filepath.Dir(m.Sandbox.MergedPath)
	expectedDir := filepath.Join(base, m.SessionID)
	if sandboxDir != expectedDir {
		// Creation records the physical spelling of the sandbox root, which is
		// not the spelling host cleanup addresses: centralized storage reaches
		// the same directory through the ~/.agm symlink, and a symlinked HOME
		// diverges even in dotfile mode. Two spellings of one directory is not
		// foreign ownership — prove they are the same directory, then continue
		// in the cleanup base's spelling so the allowlist gates and the reaper
		// operate on paths they can validate.
		if !sameHostDirectory(sandboxDir, expectedDir) {
			slog.Warn("Ignoring sandbox ownership outside the current host cleanup base",
				"session", m.SessionID, "path", m.Sandbox.MergedPath)
			return ""
		}
		sandboxDir = expectedDir
	}
	if err := sandboxgc.ValidateSandboxPath(base, sandboxDir); err != nil {
		slog.Warn("Ignoring sandbox ownership outside the allowlisted cleanup base",
			"session", m.SessionID, "error", err)
		return ""
	}
	return filepath.Join(sandboxDir, "merged")
}

// sameHostDirectory reports whether two path spellings name one existing
// directory. It is deliberately conservative: anything it cannot resolve and
// stat as a directory is treated as a different location, so an unresolvable
// path never widens what archive cleanup is willing to delete.
func sameHostDirectory(left, right string) bool {
	leftInfo, ok := statDirectory(left)
	if !ok {
		return false
	}
	rightInfo, ok := statDirectory(right)
	if !ok {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}

func statDirectory(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, false
	}
	return info, true
}

// preserveSettingsFromUpper copies .claude/settings.local.json from the sandbox
// upper layer back to the first repo that has a .claude/ directory. This prevents
// RBAC permission rules from being lost when the sandbox is cleaned up.
func preserveSettingsFromUpper(sandboxDir string) {
	upperSettings := filepath.Join(sandboxDir, "upper", ".claude", "settings.local.json")
	data, err := os.ReadFile(upperSettings)
	if err != nil {
		return // file doesn't exist in upper layer - nothing to preserve
	}

	// Find the original repo's .claude/ directory from the merged symlinks.
	// The merged dir contains symlinks pointing to the original repos.
	mergedClaude := filepath.Join(sandboxDir, "merged", ".claude")
	target, err := os.Readlink(mergedClaude)
	if err != nil {
		// Not a symlink (kernel overlayfs or already unmounted).
		slog.Debug("Cannot determine repo .claude/ path for settings preservation",
			"sandbox", sandboxDir)
		return
	}

	destPath := filepath.Join(target, "settings.local.json")
	if err := os.WriteFile(destPath, data, 0600); err != nil {
		slog.Warn("Failed to preserve settings.local.json to repo",
			"dest", destPath, "error", err)
	} else {
		slog.Info("Preserved settings.local.json from sandbox upper layer",
			"dest", destPath)
	}
}

// logGCEntry writes a best-effort entry to the gc.jsonl log.
func logGCEntry(entry gclog.Entry) {
	logger, err := gclog.NewDefault()
	if err != nil {
		slog.Warn("Failed to create gc logger", "error", err)
		return
	}
	if err := logger.Log(entry); err != nil {
		slog.Warn("Failed to write gc log entry", "error", err)
	}
}

// recordArchiveTrust counts branch commits and records a success or
// false_completion trust event. All errors are logged and swallowed — trust
// recording must never block the archive operation.
func recordArchiveTrust(sessionName, workDir, projectDir, sessionID string, createdAt time.Time) {
	dir := workDir
	if dir == "" {
		dir = projectDir
	}

	runner := &execGitRunner{workDir: dir}

	// Prefer agm/<uuid> branch convention; fall back to session name.
	branch := "agm/" + sessionID
	if _, err := runner.run("rev-parse", "--verify", branch); err != nil {
		branch = sessionName
	}

	commits, _ := getCommits(runner, branch)
	duration := time.Since(createdAt).Round(time.Second)

	var eventType TrustEventType
	var detail string
	if len(commits) > 0 {
		eventType = TrustEventSuccess
		detail = fmt.Sprintf("commits: %d, duration: %s", len(commits), duration)
	} else {
		eventType = TrustEventFalseCompletion
		detail = fmt.Sprintf("no commits, duration: %s", duration)

		recordErrorMemory(
			"session archived with no commits",
			ErrMemCatFalseCompletion,
			fmt.Sprintf("session=%s branch=%s duration=%s", sessionName, branch, duration),
			"Review session logs; may indicate early termination or permission blocks",
			SourceAGMArchive,
			sessionName,
		)
	}

	if err := RecordTrustEventForSession(sessionName, eventType, detail); err != nil {
		slog.Warn("Failed to record archive trust event", "session", sessionName, "error", err)
	}
}

// deregisterMonitor removes the given session name from all other sessions' monitor lists.
func deregisterMonitor(ctx *OpContext, sessionName string) {
	sessions, err := ctx.Storage.ListSessions(nil)
	if err != nil {
		slog.Warn("Failed to list sessions for monitor deregistration", "error", err)
		return
	}

	for _, s := range sessions {
		if len(s.Monitors) == 0 {
			continue
		}
		monitors := make([]string, 0, len(s.Monitors))
		found := false
		for _, mon := range s.Monitors {
			if mon == sessionName {
				found = true
				continue
			}
			monitors = append(monitors, mon)
		}
		if found {
			s.Monitors = monitors
			if err := ctx.Storage.UpdateSession(s); err != nil {
				slog.Warn("Failed to deregister monitor from session",
					"session", s.Name, "monitor", sessionName, "error", err)
			} else {
				slog.Info("Deregistered monitor on archive",
					"session", s.Name, "monitor", sessionName)
			}
		}
	}
}
