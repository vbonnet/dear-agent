package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/db"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	inheritContext bool
	childPrompt    string
	testDBPath     string // Test-only: Override database path for testing
)

var createChildCmd = &cobra.Command{
	Use:   "create-child [parent-session-id] [child-session-name]",
	Short: "Create a child session linked to a parent session",
	Long: `Create a new child session that inherits configuration from a parent session.

Child sessions automatically inherit:
  • Credentials (API keys, authentication)
  • Workspace access (project directories)

Optional inheritance (via --context flag):
  • Context files and conversation history

Arguments:
  parent-session-id  - Session ID of the parent session (or use current tmux session)
  child-session-name - Name for the new child session (optional, will prompt if omitted)

Flags:
  --context         - Inherit context and files from parent session
  --detached        - Create session without attaching (useful when inside tmux)
  --harness        - Harness to use (defaults to parent's harness)
  --prompt         - Initial prompt (required for AGY child identity creation)

Examples:
  agm session create-child parent-uuid                    # Prompt for child name
  agm session create-child parent-uuid child-task         # Create with specific name
  agm session create-child parent-uuid child-task --context  # Inherit context too
  agm session create-child parent-uuid child-task --harness agy --prompt "Inspect the failing tests"
  agm session create-child --detached                     # Create from current tmux session

Behavior:
  • Parent session must exist and be valid
  • Child session inherits harness type from parent (unless --harness specified)
  • Child session's parent_session_id field references the parent
  • Uses tmux backend`,
	RunE: runCreateChild,
}

func init() {
	sessionCmd.AddCommand(createChildCmd)
	createChildCmd.Flags().BoolVar(&inheritContext, "context", false, "Inherit context and files from parent session")
	createChildCmd.Flags().BoolVar(&detached, "detached", false, "Create detached session without attaching")
	createChildCmd.Flags().StringVar(&harnessName, "harness", "", "Harness to use (defaults to parent's harness)")
	createChildCmd.Flags().StringVar(&childPrompt, "prompt", "", "Initial prompt (required for AGY child sessions)")
}

func runCreateChild(cmd *cobra.Command, args []string) error {
	// Get debug flag
	debugEnabled, _ := cmd.Flags().GetBool("debug")

	// Get Dolt storage adapter early (needed for lookups)
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer func() { _ = adapter.Close() }()

	parentSessionID, childSessionName, err := resolveParentAndChild(adapter, args)
	if err != nil {
		return err
	}

	// Initialize debug logging
	if err := debug.Init(debugEnabled, fmt.Sprintf("create-child-%s", parentSessionID)); err != nil {
		fmt.Printf("Warning: Failed to initialize debug logging: %v\n", err)
	}
	defer debug.Close()

	debug.Phase("Create Child Session")
	debug.Log("Parent session ID: %s", parentSessionID)
	debug.Log("Inherit context: %v", inheritContext)

	// Validate parent session exists
	parentManifest, err := validateParentSession(adapter, parentSessionID)
	if err != nil {
		ui.PrintError(err,
			"Parent session validation failed",
			"  • Verify parent session exists: agm session list\n"+
				"  • Check session ID is correct\n"+
				"  • Run 'agm session list' to see all sessions")
		return err
	}

	debug.Log("Parent session found: %s (harness: %s)", parentManifest.Name, parentManifest.Harness)
	fmt.Printf("Parent session: %s (harness: %s)\n", parentManifest.Name, parentManifest.Harness)

	if childSessionName == "" {
		childSessionName, err = promptChildSessionName()
		if err != nil {
			return err
		}
	}

	debug.Log("Child session name: %s", childSessionName)

	selectedHarness, err := selectChildHarness(parentManifest)
	if err != nil {
		return err
	}

	workDir := parentManifest.Context.Project
	debug.Log("Inheriting working directory from parent: %s", workDir)

	_, err = createChildSession(cmd.Context(), adapter, parentManifest, childSessionName, selectedHarness)
	if err != nil {
		return err
	}

	debug.Log("Backend session created: %s", childSessionName)
	ui.PrintSuccess(fmt.Sprintf("Created %s session: %s", selectedHarness, childSessionName))

	// Show summary
	fmt.Printf("\nChild session created:\n")
	fmt.Printf("  Name: %s\n", childSessionName)
	fmt.Printf("  Parent: %s (%s)\n", parentManifest.Name, parentSessionID)
	fmt.Printf("  Harness: %s\n", selectedHarness)
	fmt.Printf("  Working Directory: %s\n", workDir)
	fmt.Printf("  Context Inherited: %v\n", inheritContext)

	if !detached {
		fmt.Printf("\nAttach to session with:\n  agm session resume %s\n", childSessionName)
	}

	return nil
}

func createChildSession(ctx context.Context, adapter *dolt.Adapter, parentManifest *manifest.Manifest, childSessionName, selectedHarness string) (*ops.CreateSessionResult, error) {
	request := buildChildCreateRequest(parentManifest, childSessionName, selectedHarness, childPrompt, inheritContext)
	manifestDir := filepath.Join(getSessionsDir(), childSessionName)
	request.ManifestDir = manifestDir
	result, err := ops.CreateSessionWithContext(ctx, &ops.OpContext{
		Tmux:    session.NewRealTmux(),
		Storage: adapter,
	}, request)
	if err != nil {
		ui.PrintError(err,
			"Failed to create child session",
			"  • Verify tmux and the selected harness are available\n"+
				"  • Check the working directory exists: "+parentManifest.Context.Project)
		return nil, err
	}

	childManifest, getErr := adapter.GetSession(result.SessionID)
	if getErr != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to load registered child session for compatibility storage: %v", getErr))
		debug.Log("Registered child session lookup failed (non-fatal): %v", getErr)
		return result, nil
	}
	if err := writeSessionToDatabase(childManifest, childManifest.ParentSessionID); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to write to compatibility database: %v", err))
		debug.Log("Compatibility database write failed (non-fatal): %v", err)
	} else {
		debug.Log("Child session written to compatibility database with parent_session_id: %s", parentManifest.SessionID)
	}
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	if err := git.CommitManifest(manifestPath, "create-child", childManifest.Name); err != nil {
		debug.Log("manifest commit skipped: %v", err)
	}
	return result, nil
}

func buildChildCreateRequest(parentManifest *manifest.Manifest, childSessionName, selectedHarness, initialPrompt string, withContext bool) *ops.CreateSessionRequest {
	parentSessionID := parentManifest.SessionID
	purpose := fmt.Sprintf("Child session of %s", parentManifest.Name)
	var tags []string
	var notes string
	if withContext {
		purpose = parentManifest.Context.Purpose
		if len(parentManifest.Context.Tags) > 0 {
			tags = append([]string{}, parentManifest.Context.Tags...)
		}
		notes = fmt.Sprintf("Child of %s\n\n%s",
			parentManifest.Name, parentManifest.Context.Notes)
	}
	model := ""
	if selectedHarness == parentManifest.Harness {
		model = parentManifest.Model
	}
	return &ops.CreateSessionRequest{
		Cwd:                parentManifest.Context.Project,
		Prompt:             initialPrompt,
		Title:              childSessionName,
		Model:              model,
		Harness:            selectedHarness,
		Caller:             ops.CreateSessionCaller{Surface: ops.CreateSurfaceCLI, Source: "session.create-child"},
		ForwardClaudeOAuth: true,
		AllowEmptyPrompt:   true,
		AllowUnsafeTitle:   true,
		RequireStorage:     true,
		Metadata: ops.CreateSessionMetadata{
			Tags:            tags,
			ContextPurpose:  purpose,
			ContextNotes:    notes,
			ParentSessionID: &parentSessionID,
		},
	}
}

// selectChildHarness picks the harness for the child session: --harness flag
// (if set), else inherited from the parent. Validates the name and warns if
// the binary is unavailable.
func selectChildHarness(parentManifest *manifest.Manifest) (string, error) {
	selectedHarness := harnessName
	if selectedHarness == "" {
		selectedHarness = parentManifest.Harness
		debug.Log("Inheriting harness from parent: %s", selectedHarness)
	} else {
		debug.Log("Using explicit harness from flag: %s", selectedHarness)
	}
	if err := agent.ValidateHarnessName(selectedHarness); err != nil {
		ui.PrintError(err,
			"Invalid harness specified",
			"  • Valid harnesses: claude-code, codex-cli, agy, opencode-cli, pi-cli; deprecated: gemini-cli\n"+
				"  • Run 'agm harness list' to see available harnesses")
		return "", err
	}
	if err := agent.ValidateHarnessAvailability(selectedHarness); err != nil {
		ui.PrintWarning(fmt.Sprintf("⚠️  %s", err.Error()))
	}
	return selectedHarness, nil
}

// resolveParentAndChild determines parentSessionID (from args[0] or by detecting
// the current tmux session) and optional childSessionName (from args[1]).
func resolveParentAndChild(adapter *dolt.Adapter, args []string) (string, string, error) {
	if len(args) > 0 {
		parentSessionID := args[0]
		var childSessionName string
		if len(args) > 1 {
			childSessionName = args[1]
		}
		return parentSessionID, childSessionName, nil
	}
	if os.Getenv("TMUX") == "" {
		ui.PrintError(
			fmt.Errorf("no parent session ID provided"),
			"Parent session ID required",
			"  • Provide parent session ID: agm session create-child <parent-id>\n"+
				"  • Or run from within a tmux session to auto-detect parent")
		return "", "", fmt.Errorf("parent session ID required")
	}
	currentTmuxName, err := tmux.GetCurrentSessionName()
	if err != nil {
		ui.PrintError(err,
			"Failed to get current tmux session name",
			"  • Provide parent session ID explicitly: agm session create-child <parent-id>\n"+
				"  • Verify you're inside tmux: echo $TMUX\n"+
				"  • Check tmux is running: tmux list-sessions")
		return "", "", err
	}
	m, err := findManifestByTmuxName(adapter, currentTmuxName)
	if err != nil {
		ui.PrintError(err,
			"Failed to find parent session",
			"  • Provide parent session ID explicitly: agm session create-child <parent-id>\n"+
				"  • Run 'agm session list' to see available sessions")
		return "", "", err
	}
	fmt.Printf("Using current tmux session as parent: %s (%s)\n", currentTmuxName, m.SessionID)
	return m.SessionID, "", nil
}

// promptChildSessionName interactively prompts for a non-empty child session name.
func promptChildSessionName() (string, error) {
	var inputName string
	err := huh.NewInput().
		Title("Enter child session name:").
		Value(&inputName).
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("session name cannot be empty")
			}
			return nil
		}).
		Run()
	if err != nil {
		ui.PrintError(err,
			"Failed to read session name from prompt",
			"  • Provide name as argument: agm session create-child <parent-id> <child-name>\n"+
				"  • Check terminal is interactive (TTY)")
		return "", err
	}
	if inputName == "" {
		ui.PrintError(
			fmt.Errorf("session name cannot be empty"),
			"Invalid session name",
			"  • Provide a non-empty session name")
		return "", fmt.Errorf("empty session name")
	}
	return inputName, nil
}

// validateParentSession validates that the parent session exists and returns its manifest
func validateParentSession(adapter *dolt.Adapter, parentSessionID string) (*manifest.Manifest, error) {
	// Try Dolt first
	parentManifest, err := adapter.GetSession(parentSessionID)
	if err == nil && parentManifest != nil {
		return parentManifest, nil
	}

	debug.Log("Parent not found in Dolt: %v", err)

	// Try database fallback (SQLite)
	database, dbErr := openDatabase()
	if dbErr == nil {
		defer func() { _ = database.Close() }()

		parentManifest, err := database.GetSession(parentSessionID)
		if err == nil {
			return parentManifest, nil
		}
		debug.Log("Parent not found in database either: %v", err)
	}

	return nil, fmt.Errorf("parent session not found: %s", parentSessionID)
}

// findManifestByTmuxName finds a manifest by tmux session name
func findManifestByTmuxName(adapter *dolt.Adapter, tmuxName string) (*manifest.Manifest, error) {
	if tmuxName == "" {
		return nil, fmt.Errorf("session not found for tmux session: %s", tmuxName)
	}

	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	for _, m := range manifests {
		if m.Tmux.SessionName == tmuxName {
			return m, nil
		}
	}

	return nil, fmt.Errorf("session not found for tmux session: %s", tmuxName)
}

// writeSessionToDatabase writes a session to the database with parent reference
func writeSessionToDatabase(session *manifest.Manifest, parentSessionID *string) error {
	database, err := openDatabase()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = database.Close() }()

	// The legacy SQLite store does not persist Manifest.ParentSessionID during
	// CreateSession, so preserve the compatibility write as a follow-up update.
	if err := database.CreateSession(session); err != nil {
		return fmt.Errorf("failed to create session in database: %w", err)
	}

	// Update parent_session_id if provided
	if parentSessionID != nil && *parentSessionID != "" {
		// Use raw SQL to update parent_session_id since Manifest struct doesn't have this field yet
		query := `UPDATE sessions SET parent_session_id = ? WHERE session_id = ?`
		_, err := database.Conn().Exec(query, *parentSessionID, session.SessionID)
		if err != nil {
			return fmt.Errorf("failed to set parent_session_id: %w", err)
		}
	}

	return nil
}

// openDatabase opens the AGM database
func openDatabase() (*db.DB, error) {
	var dbPath string

	// Test mode: use test database path if set
	if testDBPath != "" {
		dbPath = testDBPath
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dbPath = filepath.Join(homeDir, ".agm", "agm.db")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return database, nil
}
