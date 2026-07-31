package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/discovery"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	resumeDetached         bool
	resumeForceParent      bool
	resumePrompt           string
	resumePromptFile       string
	resumeDeletePromptFile bool
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
2. Invoke the shared resume transaction for health, tmux, harness, and persistence
3. Attach after the transaction releases its stable-session lock

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
  agmresume worker-1 --prompt "continue working on the auth module"
  agmresume worker-1 --detached --prompt "pick up where you left off"
  agmresume worker-1 --prompt-file /path/to/recovery.txt
  agmresume                       # Interactive picker (TODO)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if resumeDeletePromptFile && resumePromptFile == "" {
			return fmt.Errorf("--delete-prompt-file requires --prompt-file")
		}
		if len(args) == 0 {
			return fmt.Errorf("interactive picker not yet implemented - please provide identifier")
		}
		identifier := args[0]

		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer func() { _ = adapter.Close() }()

		sessionID, manifestPath, err := resolveSessionIdentifier(adapter, identifier)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				"  • Try: agmlist --all to see available sessions\n"+
					"  • Identifier can be UUID, tmux name, or project path pattern")
			return err
		}
		ui.PrintSuccess(fmt.Sprintf("Resolved identifier %q to session: %s", identifier, sessionID))
		return resumeResolvedSession(cmd.Context(), adapter, sessionID, manifestPath)
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		adapter, err := getStorage()
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		defer func() { _ = adapter.Close() }()

		sessions, err := adapter.ListSessions(&dolt.SessionFilter{ExcludeArchived: true})
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		var suggestions []string
		for _, m := range sessions {
			if m.Tmux.SessionName != "" {
				suggestions = append(suggestions, m.Tmux.SessionName)
			}
			if m.Name != "" && m.Name != m.Tmux.SessionName {
				suggestions = append(suggestions, m.Name)
			}
		}
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

// resolveSessionIdentifier finds the stable session ID and manifest provenance
// from the supported human-facing identifier forms.
func resolveSessionIdentifier(adapter *dolt.Adapter, identifier string) (string, string, error) {
	if cfg == nil {
		return "", "", fmt.Errorf("config not initialized")
	}
	sessionsDir := cfg.SessionsDir
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{ExcludeArchived: true})
	if err != nil {
		return "", "", fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}
	if len(manifests) == 0 {
		return "", "", fmt.Errorf("no session manifests found")
	}

	manifestPaths := make(map[string]string)
	for _, m := range manifests {
		manifestPaths[m.SessionID] = filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
	}
	tmuxMapping, _ := discovery.GetTmuxMappingWithAdapter(sessionsDir, adapter)
	matches, matchType := matchSessionIdentifier(manifests, tmuxMapping, identifier)
	if len(matches) == 0 {
		m, manifestPath, importErr := offerToImportOrphanedSession(adapter, identifier)
		if importErr == nil {
			return m.SessionID, manifestPath, nil
		}
		return "", "", fmt.Errorf("no sessions found matching %q", identifier)
	}
	if len(matches) > 1 {
		ui.PrintWarning(fmt.Sprintf("Multiple sessions matched %q by %s:", identifier, matchType))
		for i, m := range matches {
			tmuxName := tmuxMapping[m.SessionID]
			if tmuxName == "" {
				tmuxName = "-"
			}
			fmt.Printf("  %d. ID: %s | Tmux: %s | Project: %s\n", i+1, m.SessionID, tmuxName, m.Context.Project)
		}
		return "", "", fmt.Errorf("ambiguous identifier - please be more specific")
	}

	m := preferExecutionChild(adapter, matches[0])
	manifestPath, ok := manifestPaths[m.SessionID]
	if !ok {
		return "", "", fmt.Errorf("manifest path not found for session ID %s", m.SessionID)
	}
	return m.SessionID, manifestPath, nil
}

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

func filterManifests(manifests []*manifest.Manifest, pred func(*manifest.Manifest) bool) []*manifest.Manifest {
	var out []*manifest.Manifest
	for _, m := range manifests {
		if pred(m) {
			out = append(out, m)
		}
	}
	return out
}

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
	ui.PrintSuccess(fmt.Sprintf("Found planning session '%s' with execution session '%s'", m.Name, mostRecentChild.Name))
	fmt.Println(ui.Blue("  → Resuming execution session (use --force-parent to resume planning session)"))
	return mostRecentChild
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

func resolveResumePrompt() (string, error) {
	if resumePrompt != "" {
		return resumePrompt, nil
	}
	if resumePromptFile == "" {
		return "", nil
	}
	return readResumePromptFile(resumePromptFile, resumeDeletePromptFile)
}

// resumeResolvedSession adapts CLI inputs and presentation to the shared
// lifecycle operation. Attachment deliberately occurs only after the operation
// has returned and released the stable session lock.
func resumeResolvedSession(ctx context.Context, adapter *dolt.Adapter, sessionID, manifestPath string) error {
	prompt, err := resolveResumePrompt()
	if err != nil {
		return err
	}
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("load session policy for resume: %w", err)
	}
	trustedAddDirs, guardPath, err := trustedAddDirsForSession(m.Name, manifestRole(m))
	if err != nil {
		return err
	}
	currentAddDirs := collectExtraAddDirsForHarness(m.Sandbox, m.Harness, manifestRole(m), trustedAddDirs)
	writeRoots := append([]string{}, currentAddDirs...)
	if m.Sandbox != nil {
		for _, dir := range m.Sandbox.ExtraAddDirs {
			if configuredSourceRepo(dir) {
				continue
			}
			writeRoots = appendUnique(writeRoots, dir)
		}
	}
	if guardPath == "" && m.Harness == "codex-cli" && manifestRole(m) == "worker" {
		guardPath = defaultWorkerGuardPath
	}
	if err := configureWorkerWriteBoundary(m.Harness, manifestRole(m), guardPath, writeRoots); err != nil {
		return err
	}
	tmuxAdapter := session.NewRealTmux()
	req := &ops.ResumeSessionRequest{
		SessionID:       sessionID,
		ManifestPath:    manifestPath,
		Prompt:          prompt,
		CurrentAddDirs:  currentAddDirs,
		ExcludedAddDirs: append([]string{}, cfg.Sandbox.Repos...),
		OnEvent:         presentResumeEvent,
	}
	result, err := ops.ResumeSession(&ops.OpContext{
		Context: ctx,
		Storage: adapter,
		Tmux:    tmuxAdapter,
	}, req)
	if result != nil {
		displayResumeHealthStatus(result.Health)
	}
	if err != nil {
		ui.PrintError(err,
			"Failed to resume session",
			"  • Check tmux is running: tmux list-sessions\n"+
				"  • Verify session health: agmdoctor")
		return err
	}

	updateVSCodeTabTitle(result.TmuxSessionName)
	if err := finishResumeAttachment(ctx, tmuxAdapter, result); err != nil {
		if result.PromptMayHaveStarted {
			ui.PrintWarning(fmt.Sprintf("Post-prompt resume attachment failed after prompt submission may have started work: %v", err))
		} else {
			ui.PrintError(err,
				"Failed to attach to resumed session",
				"  • Try manual attach: tmux attach -t "+result.TmuxSessionName)
			return err
		}
	}
	ui.PrintSuccess(fmt.Sprintf("Successfully resumed session %s", sessionID))
	return nil
}

func configuredSourceRepo(path string) bool {
	clean := filepath.Clean(path)
	for _, repo := range cfg.Sandbox.Repos {
		rel, err := filepath.Rel(filepath.Clean(repo), clean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func manifestRole(m *manifest.Manifest) string {
	if m == nil {
		return ""
	}
	for _, tag := range m.Context.Tags {
		if role, ok := strings.CutPrefix(tag, "role:"); ok {
			return role
		}
	}
	return ""
}

func presentResumeEvent(event ops.ResumeSessionEvent) {
	switch event.Kind {
	case ops.ResumeEventTmuxExisting:
		ui.PrintSuccess(fmt.Sprintf("Using existing tmux session: %s", event.Message))
	case ops.ResumeEventTmuxCreated:
		ui.PrintSuccess(fmt.Sprintf("Created tmux session: %s", event.Message))
	case ops.ResumeEventHarnessReady:
		ui.PrintSuccess(fmt.Sprintf("%s session loaded and ready", event.Message))
	case ops.ResumeEventPromptSubmitted:
		ui.PrintSuccess("Post-resume prompt delivered.")
	case ops.ResumeEventWarning:
		ui.PrintWarning(event.Message)
	}
}

func displayResumeHealthStatus(health ops.ResumeSessionHealth) {
	fmt.Println("\nSession Health Check:")
	fmt.Println("────────────────────────────────────────────────")
	if health.WorktreeExists {
		fmt.Printf("✓ Worktree:      %s\n", health.WorktreePath)
	} else {
		fmt.Printf("✗ Worktree:      %s (NOT FOUND)\n", health.WorktreePath)
	}
	if health.TmuxExists {
		fmt.Printf("✓ Tmux:          %s (EXISTS)\n", health.TmuxSessionName)
	} else {
		fmt.Printf("○ Tmux:          %s (created for resume)\n", health.TmuxSessionName)
	}
	if len(health.Issues) > 0 {
		fmt.Printf("\n%s Critical Issues:\n", ui.Red("✗"))
		for _, issue := range health.Issues {
			fmt.Printf("  • %s\n", issue)
		}
	}
	fmt.Println()
}

func finishResumeAttachment(ctx context.Context, tmuxAdapter session.TmuxInterface, result *ops.ResumeSessionResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if resumeDetached {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' resumed (detached)", result.TmuxSessionName))
		fmt.Printf("  • To attach later: tmux attach -t %s\n", result.TmuxSessionName)
		fmt.Printf("  • To view logs: agm logs %s\n", result.SessionID)
		return nil
	}
	debug.Log("Attaching to tmux session: %s (socket: %s)", result.TmuxSessionName, tmux.GetSocketPath())
	ui.PrintSuccess(fmt.Sprintf("Attaching to tmux session: %s", result.TmuxSessionName))
	if result.StartedHarness {
		fmt.Println("\nNote: You will be attached to the tmux session. Press Ctrl+B then D to detach.")
	}
	fmt.Println()
	if err := tmuxAdapter.AttachSession(result.TmuxSessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

func activeHarnessHasTmuxResumeCommand(harnessName string) bool {
	switch agent.NormalizeHarnessName(harnessName) {
	case "claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli":
		return true
	default:
		return false
	}
}

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

func generateTmuxName(project string, existingSessions []string) string {
	base := sanitizeTmuxName(filepath.Base(project))
	if base == "" {
		base = "session"
	}
	name := fmt.Sprintf("claude-%s", base)
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
	return fmt.Sprintf("%s-%d", name, time.Now().Unix()%10000)
}

func offerToImportOrphanedSession(adapter *dolt.Adapter, identifier string) (*manifest.Manifest, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config not initialized")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")
	sessionsDir := cfg.SessionsDir
	entries, _, err := claude.ParseHistory(historyPath)
	if err != nil {
		return nil, "", err
	}
	sessions := claude.Deduplicate(entries)
	var matches []claude.Session
	for _, candidate := range sessions {
		if strings.HasPrefix(candidate.UUID, identifier) || strings.Contains(candidate.Project, identifier) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return nil, "", fmt.Errorf("no orphaned sessions found")
	}
	if len(matches) > 1 {
		ui.PrintWarning(fmt.Sprintf("Found %d orphaned sessions matching %q:", len(matches), identifier))
		for i, candidate := range matches {
			fmt.Printf("  %d. UUID: %s | Project: %s | Messages: %d\n", i+1, candidate.UUID[:8], candidate.Project, candidate.MessageCount)
		}
		return nil, "", fmt.Errorf("multiple orphaned sessions found - please be more specific (use full UUID or project path)")
	}

	orphan := &matches[0]
	fmt.Println()
	ui.PrintWarning(fmt.Sprintf("No manifest found for %q", identifier))
	fmt.Println()
	fmt.Println("However, I found a Claude session in history that matches:")
	fmt.Printf("  UUID:          %s\n", orphan.UUID)
	fmt.Printf("  Project:       %s\n", orphan.Project)
	fmt.Printf("  Messages:      %d\n", orphan.MessageCount)
	fmt.Printf("  Last Activity: %s\n", orphan.LastActivity.Format("2006-01-02 15:04"))

	activeTmux, _ := tmux.ListSessions()
	tmuxName := generateTmuxName(orphan.Project, activeTmux)
	fmt.Printf("  Tmux:          %s (will create)\n", tmuxName)
	fmt.Println()

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
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("failed to create sessions directory: %w", err)
	}
	sessionID := fmt.Sprintf("session-%s", orphan.UUID[:8])
	m, err := discovery.CreateManifest(orphan, sessionsDir, tmuxName, sessionID, adapter)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create manifest: %w", err)
	}
	manifestPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	ui.PrintSuccess(fmt.Sprintf("Created manifest: %s", manifestPath))
	fmt.Println()
	return m, manifestPath, nil
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
