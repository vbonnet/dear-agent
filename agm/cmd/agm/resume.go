package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/discovery"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	uuidpkg "github.com/vbonnet/dear-agent/agm/internal/uuid"
)

var (
	resumeDetached         bool
	resumeForceParent      bool
	resumePrompt           string
	resumePromptFile       string
	resumeDeletePromptFile bool
	sendResumePromptSafe   = tmux.SendMultiLinePromptSafeContext
	agyResumeWorkspaceLock = agysession.AcquireWorkspaceCreateLock
)

var resumeCmd = &cobra.Command{
	Use:   "resume [identifier]",
	Short: "Resume an AGM session by ID, tmux name, or fuzzy match",
	Long: `Resume an AGM-managed harness session by various identifier types:

- Session or conversation ID: agmresume c4eb298c
- Tmux session name:         agmresume worker-1
- Fuzzy match on project:    agmresume workspace-design
- Interactive (no args):     agmresume

The command will:
1. Resolve the identifier to find the AGM session record
2. Check session health (worktree exists, harness metadata present)
3. Create or attach to tmux session
4. Send 'cd' to worktree directory
5. Send the harness-specific resume command to the tmux pane
6. Update manifest last_activity timestamp

Flags:
  --detached     Create/resume session without attaching (session runs in background)
  --prompt       Send a prompt to the session after resume (useful for crash recovery)
  --prompt-file  Send file contents as prompt after resume (useful for crash recovery)
  --delete-prompt-file  Delete the prompt file after a successful read and validation

Examples:
  agmresume c4eb298c              # By ID prefix
  agmresume worker-1              # By tmux name
  agmresume workspace-design      # By project path pattern
  agmresume orchestrator --detached  # Resume without attaching
  agmresume worker-1 --prompt "continue working on the auth module"  # Resume and inject prompt
  agmresume worker-1 --detached --prompt "pick up where you left off"  # Background resume with prompt
  agmresume worker-1 --prompt-file /path/to/recovery.txt  # Resume with prompt from file
  agmresume                       # Interactive picker (TODO)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if resumeDeletePromptFile && resumePromptFile == "" {
			return fmt.Errorf("--delete-prompt-file requires --prompt-file")
		}
		// Get identifier from args or prompt
		var identifier string
		if len(args) > 0 {
			identifier = args[0]
		} else {
			// TODO: Interactive picker for Phase 3
			return fmt.Errorf("interactive picker not yet implemented - please provide identifier")
		}

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer func() { _ = adapter.Close() }()

		// Resolve identifier to SessionID
		sessionID, manifestPath, err := resolveSessionIdentifier(adapter, identifier)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				fmt.Sprintf("  • Try: agmlist --all to see available sessions\n"+
					"  • Identifier can be UUID, tmux name, or project path pattern"))
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Resolved identifier %q to session: %s", identifier, sessionID))

		return resumeResolvedSession(cmd.Context(), adapter, sessionID, manifestPath)
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument (session identifier)
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Get Dolt adapter
		adapter, err := getStorage()
		if err != nil {
			// Fail gracefully - return empty list if can't connect to Dolt
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		defer func() { _ = adapter.Close() }()

		// List sessions from Dolt (exclude archived sessions from completion)
		filter := &dolt.SessionFilter{
			ExcludeArchived: true,
		}
		sessions, err := adapter.ListSessions(filter)
		if err != nil {
			// Fail gracefully - return empty list if query fails
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Build completion suggestions
		var suggestions []string
		for _, m := range sessions {
			// Add tmux name (primary identifier)
			if m.Tmux.SessionName != "" {
				suggestions = append(suggestions, m.Tmux.SessionName)
			}

			// Add manifest name (secondary identifier, if different from tmux name)
			if m.Name != "" && m.Name != m.Tmux.SessionName {
				suggestions = append(suggestions, m.Name)
			}
		}

		// Return suggestions with NoFileComp directive (prevent file completion)
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

// HealthStatus represents session health check results
type HealthStatus struct {
	UUID              string
	ManifestPath      string
	WorktreeExists    bool
	WorktreePath      string
	SessionEnvExists  bool
	SessionEnvPath    string
	FileHistoryExists bool
	FileHistoryPath   string
	TmuxSessionName   string
	TmuxExists        bool
	CanResume         bool
	Issues            []string
	Warnings          []string
}

// resolveSessionIdentifier finds the Claude UUID and manifest path from various identifier types
func resolveSessionIdentifier(adapter *dolt.Adapter, identifier string) (string, string, error) {
	// Defensive check: ensure cfg is initialized
	if cfg == nil {
		return "", "", fmt.Errorf("config not initialized")
	}

	// Use configured sessions directory instead of hardcoded default
	sessionsDir := cfg.SessionsDir

	// List manifests from Dolt (exclude archived sessions)
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{
		ExcludeArchived: true,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	if len(manifests) == 0 {
		return "", "", fmt.Errorf("no session manifests found")
	}

	// Build sessionID -> manifestPath mapping
	// For Dolt-backed sessions, construct synthetic paths even if YAML doesn't exist
	manifestPaths := make(map[string]string)
	for _, m := range manifests {
		// Construct expected path based on session ID
		manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
		manifestPaths[m.SessionID] = manifestPath
	}

	// Build tmux mapping using Dolt adapter
	tmuxMapping, _ := discovery.GetTmuxMappingWithAdapter(sessionsDir, adapter)

	matches, matchType := matchSessionIdentifier(manifests, tmuxMapping, identifier)

	// Handle results
	if len(matches) == 0 {
		// No manifest found - try to find orphaned session in history and offer to import
		m, manifestPath, err := offerToImportOrphanedSession(adapter, identifier)
		if err == nil {
			// Successfully imported (v2: SessionID is top-level)
			return m.SessionID, manifestPath, nil
		}
		// Fall through to original error (orphaned session not found or user declined)
		return "", "", fmt.Errorf("no sessions found matching %q", identifier)
	}

	if len(matches) > 1 {
		// Multiple matches - show user and ask to be more specific
		ui.PrintWarning(fmt.Sprintf("Multiple sessions matched %q by %s:", identifier, matchType))
		for i, m := range matches {
			tmuxName := tmuxMapping[m.SessionID]
			if tmuxName == "" {
				tmuxName = "-"
			}
			fmt.Printf("  %d. ID: %s | Tmux: %s | Project: %s\n",
				i+1, m.SessionID, tmuxName, m.Context.Project)
		}
		return "", "", fmt.Errorf("ambiguous identifier - please be more specific")
	}

	// Single match found - prefer child execution over planning parent
	m := preferExecutionChild(adapter, matches[0])

	manifestPath, ok := manifestPaths[m.SessionID]
	if !ok {
		return "", "", fmt.Errorf("manifest path not found for session ID %s", m.SessionID)
	}
	return m.SessionID, manifestPath, nil
}

// matchSessionIdentifier runs the layered match strategies (session ID prefix,
// tmux name, project path, manifest name, session ID substring) against the
// list of known manifests. Returns the first non-empty match list and the
// human-readable match-type label.
func matchSessionIdentifier(manifests []*manifest.Manifest, tmuxMapping map[string]string, identifier string) ([]*manifest.Manifest, string) {
	if matches := filterManifests(manifests, func(m *manifest.Manifest) bool {
		return strings.HasPrefix(m.SessionID, identifier) || m.SessionID == identifier
	}); len(matches) > 0 {
		return matches, "session ID"
	}
	if matches := matchByTmuxName(manifests, tmuxMapping, identifier); len(matches) > 0 {
		return matches, "tmux name"
	}
	if matches := filterManifests(manifests, func(m *manifest.Manifest) bool {
		return strings.Contains(m.Context.Project, identifier)
	}); len(matches) > 0 {
		return matches, "project path"
	}
	if matches := filterManifests(manifests, func(m *manifest.Manifest) bool {
		return m.Name == identifier
	}); len(matches) > 0 {
		return matches, "manifest name"
	}
	return filterManifests(manifests, func(m *manifest.Manifest) bool {
		return strings.Contains(m.SessionID, identifier)
	}), "session ID"
}

// filterManifests returns the subset of manifests for which pred returns true.
func filterManifests(manifests []*manifest.Manifest, pred func(*manifest.Manifest) bool) []*manifest.Manifest {
	var out []*manifest.Manifest
	for _, m := range manifests {
		if pred(m) {
			out = append(out, m)
		}
	}
	return out
}

// matchByTmuxName returns manifests whose SessionID is mapped to the literal
// tmux session name `identifier` in tmuxMapping.
func matchByTmuxName(manifests []*manifest.Manifest, tmuxMapping map[string]string, identifier string) []*manifest.Manifest {
	var matches []*manifest.Manifest
	for sessionID, tmuxName := range tmuxMapping {
		if tmuxName != identifier {
			continue
		}
		for _, m := range manifests {
			if m.SessionID == sessionID {
				matches = append(matches, m)
				break
			}
		}
	}
	return matches
}

// preferExecutionChild returns the most recent non-archived execution child of
// m if any exist (so that resume targets the execution session, not the
// planning session). When --force-parent is set, the parent is returned
// unchanged with a warning.
func preferExecutionChild(adapter *dolt.Adapter, m *manifest.Manifest) *manifest.Manifest {
	if resumeForceParent {
		fmt.Println(ui.Yellow("  ⚠ Using --force-parent: Resuming planning session"))
		return m
	}
	children, err := adapter.GetChildren(m.SessionID)
	if err != nil || len(children) == 0 {
		return m
	}
	var mostRecentChild *manifest.Manifest
	for _, child := range children {
		if child.Lifecycle == manifest.LifecycleArchived {
			continue
		}
		if mostRecentChild == nil || child.UpdatedAt.After(mostRecentChild.UpdatedAt) {
			mostRecentChild = child
		}
	}
	if mostRecentChild == nil {
		return m
	}
	ui.PrintSuccess(fmt.Sprintf(
		"Found planning session '%s' with execution session '%s'",
		m.Name, mostRecentChild.Name))
	fmt.Println(ui.Blue("  → Resuming execution session (use --force-parent to resume planning session)"))
	return mostRecentChild
}

// checkSessionHealth validates that a session can be resumed (v2 schema)
func checkSessionHealth(adapter *dolt.Adapter, sessionID, manifestPath string) (*HealthStatus, error) {
	// Read manifest from Dolt
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to read session from Dolt: %w", err)
	}

	// Determine tmux session name - use manifest value or fallback to session name
	tmuxSessionName := m.Tmux.SessionName
	if tmuxSessionName == "" {
		// Fallback: use sanitized session name if Tmux.SessionName is empty
		tmuxSessionName = sanitizeTmuxName(m.Name)
		if tmuxSessionName == "" {
			tmuxSessionName = "session" // Last resort fallback
		}
	}

	health := &HealthStatus{
		UUID:            sessionID, // v2: SessionID (keeping field name UUID for backward compat in struct)
		ManifestPath:    manifestPath,
		WorktreePath:    m.Context.Project, // v2: Context.Project
		SessionEnvPath:  "",                // v2: Not stored in manifest
		FileHistoryPath: "",                // v2: Not stored in manifest
		TmuxSessionName: tmuxSessionName,
		Issues:          []string{},
		Warnings:        []string{},
		CanResume:       true,
	}

	// Check working directory exists (v2: Context.Project)
	if _, err := os.Stat(m.Context.Project); os.IsNotExist(err) {
		health.WorktreeExists = false
		health.Issues = append(health.Issues,
			fmt.Sprintf("Working directory not found: %s", m.Context.Project))
		health.CanResume = false
	} else {
		health.WorktreeExists = true
	}

	// Session env and file history checks removed (not in v2 schema)
	health.SessionEnvExists = true  // Always true since not checked
	health.FileHistoryExists = true // Always true since not checked

	// Check tmux session exists
	tmuxExists, err := tmux.HasSession(tmuxSessionName)
	if err != nil {
		health.Warnings = append(health.Warnings,
			fmt.Sprintf("Failed to check tmux session: %v", err))
	}
	health.TmuxExists = tmuxExists

	return health, nil
}

// displayHealthStatus prints health check results
func displayHealthStatus(health *HealthStatus) {
	fmt.Println("\nSession Health Check:")
	fmt.Println("────────────────────────────────────────────────")

	// Worktree
	if health.WorktreeExists {
		fmt.Printf("✓ Worktree:      %s\n", health.WorktreePath)
	} else {
		fmt.Printf("✗ Worktree:      %s (NOT FOUND)\n", health.WorktreePath)
	}

	// Tmux
	if health.TmuxExists {
		fmt.Printf("✓ Tmux:          %s (EXISTS)\n", health.TmuxSessionName)
	} else {
		fmt.Printf("○ Tmux:          %s (will create)\n", health.TmuxSessionName)
	}

	fmt.Println()

	// Display issues
	if len(health.Issues) > 0 {
		fmt.Printf("\n%s Critical Issues:\n", ui.Red("✗"))
		for _, issue := range health.Issues {
			fmt.Printf("  • %s\n", issue)
		}
		fmt.Println()
	}

	// Display warnings
	if len(health.Warnings) > 0 {
		ui.PrintWarning("Warnings:")
		for _, warning := range health.Warnings {
			fmt.Printf("  • %s\n", warning)
		}
		fmt.Println()
	}
}

// shouldSendResumeCommands determines whether resume should send commands to the tmux session.
// Fix (commit e7cacf8): NEVER send commands to existing tmux sessions.
// Sending resume commands to an active session injects text into the running agent.
// IsClaudeRunning detection was unreliable (process name varies), so the safe
// default is to always just attach when a session already exists.
func shouldSendResumeCommands(tmuxExists bool) bool {
	return !tmuxExists
}

// sendPostResumePrompt delivers a prompt to the session after it is ready.
// It reads the prompt from promptText (inline) or promptFile, then uses
// the context-aware multiline path, which waits for the active harness prompt
// before sending.
func sendPostResumePrompt(ctx context.Context, sessionName, promptText, promptFile string, deletePromptFile bool) error {
	var message string
	if promptText != "" {
		message = promptText
	} else {
		var err error
		message, err = readResumePromptFile(promptFile, deletePromptFile)
		if err != nil {
			return err
		}
	}

	ui.PrintSuccess("Sending post-resume prompt...")
	if err := sendResumePromptSafe(ctx, sessionName, message, false); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}
	return nil
}

func readResumePromptFile(promptFile string, deletePromptFile bool) (string, error) {
	content, err := os.ReadFile(promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %s: %w", promptFile, err)
	}
	const maxSize = 10 * 1024
	if len(content) > maxSize {
		return "", fmt.Errorf("prompt file too large: %d bytes (max 10KB)", len(content))
	}
	if deletePromptFile {
		if err := os.Remove(promptFile); err != nil {
			return "", fmt.Errorf("failed to remove consumed prompt file %s: %w", promptFile, err)
		}
	}
	return string(content), nil
}

// resumeResolvedSession runs the full resume workflow (harness detection,
// archived/health checks, and the tmux+harness resume) for a session that has
// already been resolved to a sessionID and manifestPath. It is shared by the
// `agm session resume` command and the bare `agm` default-command resume path,
// so both routes perform a real resume instead of a no-op placeholder.
func resumeResolvedSession(ctx context.Context, adapter *dolt.Adapter, sessionID, manifestPath string) error {
	// Read manifest from Dolt to check lifecycle
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		ui.PrintError(err, "Failed to read session from Dolt",
			"  • Session may not exist in database\n"+
				"  • Try: agm session list --all")
		return err
	}

	// Auto-detect harness from manifest
	harnessName := m.Harness
	if harnessName == "" {
		harnessName = "claude-code" // Default for backward compatibility
	}

	// Warn if harness unavailable
	if err := agent.ValidateHarnessAvailability(harnessName); err != nil {
		ui.PrintWarning(fmt.Sprintf("⚠️  %s", err.Error()))
	}

	fmt.Printf("Using harness: %s\n", harnessName)

	// Check if session is archived
	if m.Lifecycle == manifest.LifecycleArchived {
		ui.PrintArchivedSessionError(sessionID)
		return fmt.Errorf("cannot resume archived session")
	}

	// Check session health
	health, err := checkSessionHealth(adapter, sessionID, manifestPath)
	if err != nil {
		ui.PrintError(err,
			"Session health check failed",
			"  • Run diagnostics: agmdoctor\n"+
				"  • List all sessions: agmlist --all")
		return err
	}

	// Display health status
	displayHealthStatus(health)

	// If critical issues, abort
	if !health.CanResume {
		ui.PrintError(
			fmt.Errorf("session cannot be resumed"),
			"Critical issues prevent resuming this session",
			"  • Fix the issues above and try again",
		)
		return fmt.Errorf("session health check failed")
	}

	// Resume the session
	if err := resumeSession(ctx, adapter, sessionID, manifestPath, harnessName, health); err != nil {
		ui.PrintError(err,
			"Failed to resume session",
			"  • Check tmux is running: tmux list-sessions\n"+
				"  • Verify session health: agmdoctor\n"+
				"  • Try manual attach: tmux attach -t "+health.TmuxSessionName)
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("Successfully resumed session %s", sessionID))
	return nil
}

// resumeSessionRuntime is the narrow impure boundary for the resume
// transaction. Production keeps the existing tmux and manifest behavior;
// tests can make ordering, compensation, and post-readiness effects
// deterministic without mutating package-global function hooks.
type resumeSessionRuntime struct {
	loadManifest      func(context.Context, *dolt.Adapter, string, string) (*manifest.Manifest, error)
	createTmux        func(string, string) (string, error)
	killTmux          func(createdResumeTmux) error
	dispatch          func(*dolt.Adapter, *manifest.Manifest, string, *HealthStatus) error
	wait              func(string, *HealthStatus) error
	persistTmuxName   func(context.Context, *dolt.Adapter, *manifest.Manifest, string) (resumeTmuxNameChange, error)
	restoreTmuxName   func(context.Context, *dolt.Adapter, *manifest.Manifest, resumeTmuxNameChange) error
	restorePermission func(string, *manifest.Manifest, *HealthStatus)
	updateActivity    func(*dolt.Adapter, string, string) error
	updateTabTitle    func(string)
	deliverPrompt     func(string, string, string, bool) error
	attachTmux        func(string) error
	attach            func(string) error
}

type createdResumeTmux struct {
	Name string
	ID   string
}

func (created createdResumeTmux) owned() bool {
	return created.Name != "" && created.ID != ""
}

type resumeTmuxNameChange struct {
	Applied bool
	Change  dolt.TmuxSessionNameChange
}

func realResumeSessionRuntime(ctx context.Context) resumeSessionRuntime {
	return resumeSessionRuntime{
		loadManifest: getResumeManifest,
		createTmux:   tmux.NewSessionWithID,
		killTmux:     killCreatedResumeTmux,
		dispatch:     dispatchResumeCommand,
		wait: func(harnessName string, health *HealthStatus) error {
			return waitForResumedHarness(ctx, harnessName, health)
		},
		persistTmuxName:   persistResumeTmuxName,
		restoreTmuxName:   restoreResumeTmuxName,
		restorePermission: restorePermissionMode,
		updateActivity:    updateManifestActivity,
		updateTabTitle:    updateVSCodeTabTitle,
		deliverPrompt: func(sessionName, promptText, promptFile string, deletePromptFile bool) error {
			return sendPostResumePrompt(ctx, sessionName, promptText, promptFile, deletePromptFile)
		},
		attachTmux: tmux.AttachSession,
	}
}

// killCreatedResumeTmux compensates a failed resume and verifies that the
// exact session created by this attempt is gone. Both mutation and verification
// use strict error-reporting paths so an inaccessible socket cannot masquerade
// as a missing target.
func killCreatedResumeTmux(created createdResumeTmux) error {
	if !created.owned() {
		return fmt.Errorf("cannot clean up tmux session %q without its immutable identity", created.Name)
	}
	if err := tmux.KillSessionIDChecked(created.ID); err != nil {
		return err
	}
	exists, err := tmux.HasSessionIDStrict(created.ID)
	if err != nil {
		return fmt.Errorf("verify tmux session identity cleanup: %w", err)
	}
	if exists {
		return fmt.Errorf("tmux session %q (%s) still exists after cleanup", created.Name, created.ID)
	}
	return nil
}

func rollbackCreatedResumeTmux(ctx context.Context, runtime resumeSessionRuntime, adapter *dolt.Adapter, m *manifest.Manifest, created createdResumeTmux, nameChange resumeTmuxNameChange, primaryErr error) error {
	if cleanupErr := runtime.killTmux(created); cleanupErr != nil {
		return errors.Join(primaryErr, fmt.Errorf("failed to clean up newly created tmux session %q (%s): %w", created.Name, created.ID, cleanupErr))
	}
	if nameChange.Applied {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if runtime.restoreTmuxName == nil {
			return errors.Join(primaryErr, fmt.Errorf("resume runtime does not provide tmux-name compensation"))
		}
		if restoreErr := runtime.restoreTmuxName(cleanupCtx, adapter, m, nameChange); restoreErr != nil {
			return errors.Join(primaryErr, fmt.Errorf("failed to compensate canonical tmux-name persistence: %w", restoreErr))
		}
	}
	return primaryErr
}

func persistResumeTmuxName(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, sessionName string) (resumeTmuxNameChange, error) {
	change, err := adapter.BeginTmuxSessionNameChange(ctx, m.SessionID, sessionName)
	if err != nil {
		return resumeTmuxNameChange{}, fmt.Errorf("persist canonical tmux session name %q: %w", sessionName, err)
	}
	latest, err := adapter.GetSession(m.SessionID)
	if err != nil {
		reloadErr := fmt.Errorf("reload session after canonical tmux-name persistence: %w", err)
		if change != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			restored, restoreErr := adapter.RestoreTmuxSessionNameChange(cleanupCtx, *change)
			if restoreErr != nil {
				return resumeTmuxNameChange{}, errors.Join(reloadErr, fmt.Errorf("compensate tmux-name persistence after reload failure: %w", restoreErr))
			}
			if !restored {
				return resumeTmuxNameChange{}, errors.Join(reloadErr, fmt.Errorf("compensate tmux-name persistence after reload failure: session metadata no longer matches this resume transaction"))
			}
		}
		return resumeTmuxNameChange{}, reloadErr
	}
	m.Tmux.SessionName = latest.Tmux.SessionName
	m.UpdatedAt = latest.UpdatedAt
	if change == nil {
		return resumeTmuxNameChange{}, nil
	}
	return resumeTmuxNameChange{Applied: true, Change: *change}, nil
}

func restoreResumeTmuxName(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, change resumeTmuxNameChange) error {
	if !change.Applied {
		return nil
	}
	swapped, err := adapter.RestoreTmuxSessionNameChange(ctx, change.Change)
	if err != nil {
		return err
	}
	if !swapped {
		return fmt.Errorf("session metadata no longer matches this resume transaction")
	}
	latest, err := adapter.GetSession(m.SessionID)
	if err != nil {
		return fmt.Errorf("reload session after tmux-name compensation: %w", err)
	}
	m.Tmux.SessionName = latest.Tmux.SessionName
	m.UpdatedAt = latest.UpdatedAt
	return nil
}

// resumeSession performs the complete resume workflow.
func resumeSession(ctx context.Context, adapter *dolt.Adapter, sessionID, manifestPath, harnessName string, health *HealthStatus) error {
	return resumeSessionWithRuntime(ctx, adapter, sessionID, manifestPath, harnessName, health, realResumeSessionRuntime(ctx))
}

func resumeSessionWithRuntime(ctx context.Context, adapter *dolt.Adapter, sessionID, manifestPath, harnessName string, health *HealthStatus, runtime resumeSessionRuntime) error {
	m, err := loadResumeSessionManifest(ctx, adapter, sessionID, harnessName, runtime)
	if err != nil {
		return err
	}
	createdTmux, err := ensureResumeTmuxSession(ctx, health, runtime)
	if err != nil {
		if createdTmux.owned() {
			return rollbackCreatedResumeTmux(ctx, runtime, adapter, m, createdTmux, resumeTmuxNameChange{}, err)
		}
		return err
	}
	sendCommands := shouldSendResumeCommands(health.TmuxExists)
	if err := runHarnessResume(ctx, adapter, m, harnessName, health, sendCommands, runtime); err != nil {
		if createdTmux.owned() {
			return rollbackCreatedResumeTmux(ctx, runtime, adapter, m, createdTmux, resumeTmuxNameChange{}, err)
		}
		return err
	}
	var nameChange resumeTmuxNameChange
	if createdTmux.owned() {
		nameChange, err = persistCreatedResumeTmuxName(ctx, runtime, adapter, m, health)
		if err != nil {
			return rollbackCreatedResumeTmux(ctx, runtime, adapter, m, createdTmux, resumeTmuxNameChange{}, err)
		}
	}
	transactionalPrompt := agent.NormalizeHarnessName(harnessName) == "codex-cli"
	if transactionalPrompt {
		// Codex canonical-name persistence must commit before the optional prompt
		// can trigger work. Prompt delivery is strict for a cold Codex resume;
		// any failure compensates both the exact created tmux identity and this
		// transaction's compare-and-swap metadata write. Other harnesses retain
		// their established warning-only prompt semantics and finalization order.
		if err := deliverPostResumePrompt(ctx, health.TmuxSessionName, runtime, createdTmux.owned()); err != nil {
			if createdTmux.owned() {
				return rollbackCreatedResumeTmux(ctx, runtime, adapter, m, createdTmux, nameChange, err)
			}
			return err
		}
	}
	if sendCommands {
		runtime.restorePermission(harnessName, m, health)
	}
	return finalizeResumeSession(ctx, adapter, sessionID, manifestPath, health, sendCommands, !transactionalPrompt, runtime)
}

func loadResumeSessionManifest(ctx context.Context, adapter *dolt.Adapter, sessionID, harnessName string, runtime resumeSessionRuntime) (*manifest.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loadManifest := runtime.loadManifest
	if loadManifest == nil {
		loadManifest = getResumeManifest
	}
	m, err := loadManifest(ctx, adapter, sessionID, harnessName)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func ensureResumeTmuxSession(ctx context.Context, health *HealthStatus, runtime resumeSessionRuntime) (createdResumeTmux, error) {
	if err := ctx.Err(); err != nil {
		return createdResumeTmux{}, err
	}
	if health.TmuxExists {
		ui.PrintSuccess(fmt.Sprintf("Attaching to existing tmux session: %s", health.TmuxSessionName))
		return createdResumeTmux{}, nil
	}
	createdName := tmux.SanitizeSessionName(health.TmuxSessionName)
	ui.PrintSuccess(fmt.Sprintf("Creating tmux session: %s", createdName))
	if runtime.createTmux == nil {
		return createdResumeTmux{}, fmt.Errorf("resume runtime does not provide tmux creation")
	}
	sessionID, err := runtime.createTmux(createdName, health.WorktreePath)
	created := createdResumeTmux{Name: createdName, ID: sessionID}
	if err != nil {
		return created, fmt.Errorf("failed to create tmux session: %w", err)
	}
	if sessionID == "" {
		return createdResumeTmux{}, fmt.Errorf("tmux creation returned no immutable session identity")
	}
	// NewSession creates the sanitized name. Carry that exact identity through
	// dispatch, readiness, rollback, prompt delivery, and attach.
	health.TmuxSessionName = createdName
	return created, nil
}

func persistCreatedResumeTmuxName(ctx context.Context, runtime resumeSessionRuntime, adapter *dolt.Adapter, m *manifest.Manifest, health *HealthStatus) (resumeTmuxNameChange, error) {
	if runtime.persistTmuxName == nil {
		return resumeTmuxNameChange{}, fmt.Errorf("resume runtime does not provide tmux-name persistence")
	}
	return runtime.persistTmuxName(ctx, adapter, m, health.TmuxSessionName)
}

func runHarnessResume(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, harnessName string, health *HealthStatus, sendCommands bool, runtime resumeSessionRuntime) error {
	if !sendCommands {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return withAgyResumeWorkspaceLock(ctx, harnessName, health.WorktreePath, func() error {
		if err := runtime.dispatch(adapter, m, harnessName, health); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := runtime.wait(harnessName, health); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	})
}

func withAgyResumeWorkspaceLock(ctx context.Context, harnessName, workDir string, resume func() error) error {
	if agent.NormalizeHarnessName(harnessName) == "agy" {
		releaseWorkspaceLock, err := agyResumeWorkspaceLock(ctx, workDir)
		if err != nil {
			return fmt.Errorf("acquire AGY workspace lifecycle lock for resume: %w", err)
		}
		defer func() {
			if unlockErr := releaseWorkspaceLock(); unlockErr != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to release AGY workspace lock after resume: %v", unlockErr))
			}
		}()
	}
	return resume()
}

func finalizeResumeSession(ctx context.Context, adapter *dolt.Adapter, sessionID, manifestPath string, health *HealthStatus, sendCommands, deliverPrompt bool, runtime resumeSessionRuntime) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Update manifest last_activity (best effort - don't fail if this errors)
	if err := runtime.updateActivity(adapter, sessionID, manifestPath); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to update manifest activity: %v", err))
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Update VS Code tab title if running in VS Code
	runtime.updateTabTitle(health.TmuxSessionName)
	if deliverPrompt {
		if err := deliverPostResumePrompt(ctx, health.TmuxSessionName, runtime, false); err != nil {
			return err
		}
	}
	return attachResumedSession(ctx, sessionID, health, sendCommands, runtime)
}

func deliverPostResumePrompt(ctx context.Context, sessionName string, runtime resumeSessionRuntime, strict bool) error {
	// Send post-resume prompt if --prompt or --prompt-file was specified.
	// This happens after the harness is ready, before attach.
	// Works for both new sessions (sendCommands=true) and existing sessions.
	if resumePrompt == "" && resumePromptFile == "" {
		return nil
	}
	if err := runtime.deliverPrompt(sessionName, resumePrompt, resumePromptFile, resumeDeletePromptFile); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if strict {
			return fmt.Errorf("failed to deliver transactional post-resume prompt: %w", err)
		}
		// Non-fatal: warn but continue so the user can still attach and type manually
		ui.PrintWarning(fmt.Sprintf("Failed to send post-resume prompt: %v", err))
		return nil
	}
	ui.PrintSuccess("Post-resume prompt delivered.")
	return nil
}

func attachResumedSession(ctx context.Context, sessionID string, health *HealthStatus, sendCommands bool, runtime resumeSessionRuntime) error {
	// NOTE: No need to release global lock before attach - using fine-grained locks
	// AttachSession never holds any lock, so it can block indefinitely without issues
	if err := ctx.Err(); err != nil {
		return err
	}
	if resumeDetached {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' resumed (detached)", health.TmuxSessionName))
		fmt.Printf("  • To attach later: tmux attach -t %s\n", health.TmuxSessionName)
		fmt.Printf("  • To view logs: agm logs %s\n", sessionID)
		return nil
	}
	socketPath := tmux.GetSocketPath()
	debug.Log("Attaching to tmux session: %s (socket: %s)", health.TmuxSessionName, socketPath)
	ui.PrintSuccess(fmt.Sprintf("Attaching to tmux session: %s", health.TmuxSessionName))
	if sendCommands {
		fmt.Println("\nNote: You will be attached to the tmux session. Press Ctrl+B then D to detach.")
	}
	fmt.Println()
	attachTmux := runtime.attachTmux
	if attachTmux == nil {
		attachTmux = runtime.attach
	}
	if attachTmux == nil {
		return fmt.Errorf("resume runtime does not provide tmux attachment")
	}
	if err := attachTmux(health.TmuxSessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

func getResumeManifest(ctx context.Context, adapter *dolt.Adapter, sessionID, harnessName string) (*manifest.Manifest, error) {
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to read session from Dolt: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := migrateAmbiguousLegacyAgyModel(adapter, m, harnessName); err != nil {
		return nil, err
	}
	return m, nil
}

// dispatchResumeCommand builds the harness-specific resume command and sends
// it to the tmux session. For Claude, falls back to uuid.Discover if the
// stored UUID is missing.
func dispatchResumeCommand(adapter *dolt.Adapter, m *manifest.Manifest, harnessName string, health *HealthStatus) error {
	var fullCmd string
	switch agent.NormalizeHarnessName(harnessName) {
	case "opencode-cli":
		fullCmd = ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
			Harness: "opencode-cli", Model: m.Model, SessionName: health.TmuxSessionName,
			WorkDir: health.WorktreePath,
		}).Command
	case "codex-cli":
		// Pre-trust the workdir so a Codex relaunch does not block on the
		// interactive trust prompt in non-git sandbox dirs (ce-cmsq).
		if err := agent.EnsureCodexWorkdirTrusted(health.WorktreePath); err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not pre-trust Codex workdir %s: %v", health.WorktreePath, err))
		}
		fullCmd = buildCodexResumeCommand(m, health)
	case "agy":
		fullCmd = buildAgyResumeCommand(m, health)
	case "claude-code":
		fullCmd = buildClaudeResumeCommand(adapter, m, health)
	default:
		fullCmd = fmt.Sprintf("cd %s && exit", launchparity.ShellQuote(health.WorktreePath))
		ui.PrintWarning(fmt.Sprintf("Harness '%s' does not support resume - starting in working directory", harnessName))
	}
	if err := tmux.SendCommand(health.TmuxSessionName, fullCmd); err != nil {
		return fmt.Errorf("failed to send resume command: %w", err)
	}
	return nil
}

func activeHarnessHasTmuxResumeCommand(harnessName string) bool {
	switch agent.NormalizeHarnessName(harnessName) {
	case "claude-code", "codex-cli", "agy", "opencode-cli":
		return true
	default:
		return false
	}
}

func buildCodexResumeCommand(m *manifest.Manifest, health *HealthStatus) string {
	model := m.Model
	if model == "" {
		model = agent.HarnessDefaults["codex-cli"]
	}
	return ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: model, SessionName: health.TmuxSessionName,
		WorkDir: health.WorktreePath, PermissionMode: m.PermissionMode, Codex: m.Codex,
	}).Command
}

func buildAgyResumeCommand(m *manifest.Manifest, health *HealthStatus) string {
	if m.Agy != nil && m.Agy.ConversationID != "" {
		model := m.Model
		if isAmbiguousLegacyAgyDefault(model) {
			model = ""
		}
		return ops.BuildAgyResumeCommand(ops.HarnessLaunchSpec{
			Harness: "agy", Model: model, SessionName: health.TmuxSessionName,
			WorkDir: health.WorktreePath, PermissionMode: m.PermissionMode,
			ExtraAddDirs: []string{health.WorktreePath},
		}, m.Agy.ConversationID).Command
	}
	ui.PrintWarning("No AGY conversation ID found - starting new AGY session")
	model := m.Model
	if model == "" {
		model = agent.HarnessDefaults["agy"]
	}
	return ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: model, SessionName: health.TmuxSessionName,
		WorkDir: health.WorktreePath, PermissionMode: m.PermissionMode,
	}).Command
}

func isAmbiguousLegacyAgyDefault(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "2.5-flash", "gemini-2.5-flash":
		return true
	default:
		return false
	}
}

// migrateAmbiguousLegacyAgyModel clears the former AGY default on saved
// conversations. Older import and association paths wrote this value without
// observing the native selection, so it cannot safely be distinguished from
// an explicit choice. The conversation itself remains the source of truth.
func migrateAmbiguousLegacyAgyModel(adapter *dolt.Adapter, m *manifest.Manifest, harnessName string) error {
	if agent.NormalizeHarnessName(harnessName) != "agy" || m.Agy == nil || m.Agy.ConversationID == "" || !isAmbiguousLegacyAgyDefault(m.Model) {
		return nil
	}
	m.Model = ""
	if err := adapter.UpdateSession(m); err != nil {
		return fmt.Errorf("migrate ambiguous legacy AGY model provenance: %w", err)
	}
	return nil
}

// buildClaudeResumeCommand assembles `claude --resume <uuid>` (or a bare
// `claude` if no UUID can be discovered).
func buildClaudeResumeCommand(adapter *dolt.Adapter, m *manifest.Manifest, health *HealthStatus) string {
	resumeUUID := m.Claude.UUID
	if resumeUUID == "" {
		findInManifests := func(name string) (*manifest.Manifest, error) {
			manifests, listErr := adapter.ListSessions(&dolt.SessionFilter{})
			if listErr != nil {
				return nil, listErr
			}
			for _, ms := range manifests {
				if ms.Tmux.SessionName == name || ms.Name == name {
					return ms, nil
				}
			}
			return nil, fmt.Errorf("no session found for: %s", name)
		}
		discoveredUUID, discoverErr := uuidpkg.Discover(health.TmuxSessionName, findInManifests, false)
		if discoverErr == nil && discoveredUUID != "" {
			resumeUUID = discoveredUUID
			ui.PrintSuccess(fmt.Sprintf("Discovered Claude UUID via fallback: %s", resumeUUID[:8]))
		}
	}
	if resumeUUID != "" {
		// `claude --resume` is scoped to the current directory's project slug.
		// If the conversation was started in a different directory than the
		// recorded worktree (e.g. an associated pre-existing session), resuming
		// from the worktree yields "No conversation found". Resume from the
		// conversation's actual cwd when it can be located.
		resumeDir := resolveResumeDir(resumeUUID, health.WorktreePath)
		return fmt.Sprintf("cd %s && claude --resume %s && exit",
			launchparity.ShellQuote(resumeDir),
			launchparity.ShellQuote(resumeUUID))
	}
	ui.PrintWarning("No Claude UUID found - starting new Claude session")
	return fmt.Sprintf("cd %s && claude && exit", launchparity.ShellQuote(health.WorktreePath))
}

// resolveResumeDir returns the directory `claude --resume` should run from for
// the given conversation UUID. It prefers the conversation's recorded cwd (so
// the project slug matches and the transcript is found); if that cannot be
// determined it falls back to the worktree path.
func resolveResumeDir(resumeUUID, worktreePath string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return worktreePath
	}
	cwd, err := claude.FindTranscriptCwd(homeDir, resumeUUID)
	if err != nil || cwd == "" {
		return worktreePath
	}
	if cwd != worktreePath {
		ui.PrintWarning(fmt.Sprintf(
			"Conversation lives in %s, not the worktree %s - resuming from there",
			cwd, worktreePath))
	}
	return cwd
}

func waitForResumedHarness(ctx context.Context, harnessName string, health *HealthStatus) error {
	switch agent.NormalizeHarnessName(harnessName) {
	case "claude-code":
		return waitForResumedClaude(ctx, health)
	case "codex-cli":
		return waitForResumedCodex(ctx, health)
	case "agy":
		return waitForResumedAgy(ctx, health)
	default:
		return nil
	}
}

// waitForResumedClaude waits first for the claude process to appear, then for
// the conversation prompt to render (60s timeout each behind a spinner).
func waitForResumedClaude(ctx context.Context, health *HealthStatus) error {
	var processWaitErr error
	spinErr := spinner.New().
		Title("Waiting for Claude process to start...").
		Accessible(true).
		Action(func() {
			processWaitErr = tmux.WaitForProcessReadyContext(ctx, health.TmuxSessionName, "claude", 15*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	fmt.Println()
	if processWaitErr != nil {
		ui.PrintWarning("Claude process is taking longer than expected")
		fmt.Println("  Continuing to wait for conversation to load...")
	} else {
		ui.PrintSuccess("Claude process started!")
	}
	var promptWaitErr error
	spinErr = spinner.New().
		Title("Waiting for conversation to load...").
		Accessible(true).
		Action(func() {
			promptWaitErr = tmux.WaitForPromptOrResumeFailureContext(ctx, health.TmuxSessionName, 60*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	fmt.Println()
	// A fatal resume failure (e.g. "No conversation found") means the harness
	// will never reach a prompt - abort instead of attaching to a dead pane.
	var resumeFailure *tmux.ResumeFailureError
	if errors.As(promptWaitErr, &resumeFailure) {
		return fmt.Errorf("claude could not resume this conversation: %s\n"+
			"  • The stored Claude UUID may not match a conversation in the worktree's project directory\n"+
			"  • Verify the UUID: agm session list --all\n"+
			"  • Diagnose resumability: agm admin doctor --validate",
			resumeFailure.Detail)
	}
	if promptWaitErr != nil {
		ui.PrintWarning("Conversation is taking longer than expected to load")
		fmt.Println("  Attaching now - conversation should appear shortly")
	} else {
		ui.PrintSuccess("Conversation loaded and ready!")
	}
	return nil
}

func waitForResumedAgy(ctx context.Context, health *HealthStatus) error {
	return waitForResumedAgyWithWait(ctx, health, tmux.WaitForAgyPrompt)
}

func waitForResumedAgyWithWait(ctx context.Context, health *HealthStatus, wait func(context.Context, string, time.Duration) error) error {
	var promptWaitErr error
	spinErr := spinner.New().
		Title("Waiting for AGY conversation to load...").
		Accessible(true).
		Action(func() {
			promptWaitErr = wait(ctx, health.TmuxSessionName, 60*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	fmt.Println()
	if promptWaitErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ui.PrintWarning("AGY conversation is taking longer than expected to load")
		fmt.Println("  Attaching now - AGY should appear shortly")
	} else {
		ui.PrintSuccess("AGY conversation loaded and ready!")
	}
	return nil
}

type codexResumeReadinessRuntime struct {
	waitForProcess  func(string, string, time.Duration) error
	waitForComposer func(string, time.Duration) error
}

func waitForResumedCodex(ctx context.Context, health *HealthStatus) error {
	return waitForResumedCodexWithRuntime(ctx, health, codexResumeReadinessRuntime{
		waitForProcess: func(sessionName, processName string, timeout time.Duration) error {
			return tmux.WaitForProcessReadyContext(ctx, sessionName, processName, timeout)
		},
		waitForComposer: func(sessionName string, timeout time.Duration) error {
			return tmux.WaitForCodexPromptContext(ctx, sessionName, timeout)
		},
	})
}

func waitForResumedCodexWithRuntime(ctx context.Context, health *HealthStatus, runtime codexResumeReadinessRuntime) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var processWaitErr error
	spinErr := spinner.New().
		Title("Waiting for Codex process to start...").
		Accessible(true).
		Action(func() {
			processWaitErr = runtime.waitForProcess(health.TmuxSessionName, "codex", 60*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	fmt.Println()
	if processWaitErr != nil {
		return fmt.Errorf("codex process did not become ready: %w", processWaitErr)
	}
	ui.PrintSuccess("Codex process started!")

	var promptWaitErr error
	spinErr = spinner.New().
		Title("Waiting for Codex composer to load...").
		Accessible(true).
		Action(func() {
			promptWaitErr = runtime.waitForComposer(health.TmuxSessionName, 60*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	fmt.Println()
	if promptWaitErr != nil {
		return fmt.Errorf("codex composer did not become ready: %w", promptWaitErr)
	}
	ui.PrintSuccess("Codex loaded and ready!")
	return nil
}

// restorePermissionMode replays the saved permission-mode S-Tab cycles when
// the session supports it and the saved mode differs from default.
func restorePermissionMode(harnessName string, m *manifest.Manifest, health *HealthStatus) {
	if !supportsPermissionMode(harnessName) || m.PermissionMode == "" || m.PermissionMode == "default" {
		return
	}
	shiftTabCount := calculateShiftTabCount(m.PermissionMode)
	if shiftTabCount <= 0 {
		return
	}
	canReceive := session.CheckSessionDelivery(health.TmuxSessionName)
	if canReceive != state.CanReceiveYes {
		ui.PrintWarning(fmt.Sprintf("Cannot restore permission mode: session not at idle prompt (state: %s)", canReceive))
		return
	}
	ui.PrintSuccess(fmt.Sprintf("Restoring permission mode: %s", m.PermissionMode))
	for i := 0; i < shiftTabCount; i++ {
		if err := tmux.SendKeys(health.TmuxSessionName, "S-Tab"); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to restore mode: %v", err))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// updateManifestActivity updates the updated_at field and state in manifest (v2: auto-updated by Write)
func updateManifestActivity(adapter *dolt.Adapter, sessionID, manifestPath string) error {
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return err
	}

	// v2: UpdatedAt is automatically updated when we call UpdateSession
	// Just write the manifest back to Dolt, which will update UpdatedAt

	// Write back to Dolt
	if err := adapter.UpdateSession(m); err != nil {
		return err
	}

	// Auto-commit manifest change if in git repo
	_ = git.CommitManifest(manifestPath, "resume", m.Name) // Errors logged internally

	return nil
}

// sanitizeTmuxName sanitizes a string for safe use as tmux session name
// Only allows alphanumeric, dash, and underscore
func sanitizeTmuxName(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else if r == ' ' {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// generateTmuxName generates a unique tmux session name from a project path
func generateTmuxName(project string, existingSessions []string) string {
	base := filepath.Base(project)
	base = sanitizeTmuxName(base)

	// Ensure base is not empty after sanitization
	if base == "" {
		base = "session"
	}

	name := fmt.Sprintf("claude-%s", base)

	// Check for conflicts with existing sessions
	conflict := false
	for _, existing := range existingSessions {
		if existing == name {
			conflict = true
			break
		}
	}

	if !conflict {
		return name
	}

	// Add numeric suffix if conflict
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		conflict = false
		for _, existing := range existingSessions {
			if existing == candidate {
				conflict = true
				break
			}
		}
		if !conflict {
			return candidate
		}
	}

	// Fallback to timestamp-based suffix
	return fmt.Sprintf("%s-%d", name, time.Now().Unix()%10000)
}

// offerToImportOrphanedSession checks history.jsonl for orphaned sessions
// and prompts user to import if found
func offerToImportOrphanedSession(adapter *dolt.Adapter, identifier string) (*manifest.Manifest, string, error) {
	// Defensive check: ensure cfg is initialized
	if cfg == nil {
		return nil, "", fmt.Errorf("config not initialized")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}

	historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")
	// Use configured sessions directory instead of hardcoded default
	sessionsDir := cfg.SessionsDir

	// Parse history (best effort - don't fail loudly on errors)
	entries, _, err := claude.ParseHistory(historyPath)
	if err != nil {
		// Return error to trigger normal "not found" message
		return nil, "", err
	}

	// Deduplicate to sessions
	sessions := claude.Deduplicate(entries)

	// Match by UUID or project path (NOT tmux, since we have no mapping for orphaned sessions)
	var matches []claude.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.UUID, identifier) ||
			strings.Contains(s.Project, identifier) {
			matches = append(matches, s)
		}
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("no orphaned sessions found")
	}

	// Handle multiple matches - ask user to be more specific
	var session *claude.Session
	if len(matches) > 1 {
		ui.PrintWarning(fmt.Sprintf("Found %d orphaned sessions matching %q:", len(matches), identifier))
		for i, s := range matches {
			fmt.Printf("  %d. UUID: %s | Project: %s | Messages: %d\n",
				i+1, s.UUID[:8], s.Project, s.MessageCount)
		}
		return nil, "", fmt.Errorf("multiple orphaned sessions found - please be more specific (use full UUID or project path)")
	}

	session = &matches[0]

	// Display session info
	fmt.Println()
	ui.PrintWarning(fmt.Sprintf("No manifest found for %q", identifier))
	fmt.Println()
	fmt.Println("However, I found a Claude session in history that matches:")
	fmt.Printf("  UUID:          %s\n", session.UUID)
	fmt.Printf("  Project:       %s\n", session.Project)
	fmt.Printf("  Messages:      %d\n", session.MessageCount)
	fmt.Printf("  Last Activity: %s\n", session.LastActivity.Format("2006-01-02 15:04"))

	// Get active tmux sessions to avoid name conflicts
	activeTmux, _ := tmux.ListSessions()

	// Generate unique tmux name
	tmuxName := generateTmuxName(session.Project, activeTmux)
	fmt.Printf("  Tmux:          %s (will create)\n", tmuxName)
	fmt.Println()

	// Confirm with user
	var confirm bool
	err = huh.NewConfirm().
		Title("Would you like to import this session?").
		Affirmative("Yes").
		Negative("No").
		Value(&confirm).
		WithTheme(ui.GetTheme()).
		Run()
	if err != nil || !confirm {
		return nil, "", fmt.Errorf("import declined by user")
	}

	// Ensure sessions dir exists
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Generate session ID from UUID prefix
	sessionID := fmt.Sprintf("session-%s", session.UUID[:8])

	// Create manifest
	m, err := discovery.CreateManifest(session, sessionsDir, tmuxName, sessionID, adapter)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create manifest: %w", err)
	}

	manifestPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	ui.PrintSuccess(fmt.Sprintf("Created manifest: %s", manifestPath))
	fmt.Println()

	return m, manifestPath, nil
}

// calculateShiftTabCount returns number of Shift+Tab presses needed to reach target mode
// Assumes Claude always starts in "default" mode
// Cycle order: default(0) -> auto(1) -> plan(2) -> default(0)
func calculateShiftTabCount(targetMode string) int {
	modeIndex := map[string]int{"default": 0, "auto": 1, "plan": 2}
	targetIdx, ok := modeIndex[targetMode]
	if !ok {
		return 0 // Invalid mode, stay in default
	}
	// Always starting from default(0) on fresh launch
	return targetIdx
}

// supportsPermissionMode checks if harness supports permission modes
func supportsPermissionMode(harness string) bool {
	// Currently only Claude Code supports permission modes
	return harness == "claude-code"
}

func init() {
	resumeCmd.Flags().BoolVar(&resumeDetached, "detached", false, "Resume session without attaching")
	resumeCmd.Flags().BoolVar(&resumeForceParent, "force-parent", false, "Resume planning session instead of execution session")
	resumeCmd.Flags().StringVar(&resumePrompt, "prompt", "", "Prompt to send to session after resume (for crash recovery)")
	resumeCmd.Flags().StringVar(&resumePromptFile, "prompt-file", "", "File containing prompt to send after resume (max 10KB)")
	resumeCmd.Flags().BoolVar(&resumeDeletePromptFile, "delete-prompt-file", false, "Delete --prompt-file after a successful read and validation")
	resumeCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	sessionCmd.AddCommand(resumeCmd)
}
