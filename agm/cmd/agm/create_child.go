package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/backend"
	"github.com/vbonnet/dear-agent/agm/internal/db"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	inheritContext bool
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

Examples:
  agm session create-child parent-uuid                    # Prompt for child name
  agm session create-child parent-uuid child-task         # Create with specific name
  agm session create-child parent-uuid child-task --context  # Inherit context too
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
}

func runCreateChild(cmd *cobra.Command, args []string) error {
	// Get debug flag
	debugEnabled, _ := cmd.Flags().GetBool("debug")

	// Get Dolt storage adapter early (needed for lookups)
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Determine parent session ID
	var parentSessionID string
	var childSessionName string

	if len(args) == 0 {
		// Try to detect from current tmux session
		if os.Getenv("TMUX") != "" {
			currentTmuxName, err := tmux.GetCurrentSessionName()
			if err != nil {
				ui.PrintError(err,
					"Failed to get current tmux session name",
					"  • Provide parent session ID explicitly: agm session create-child <parent-id>\n"+
						"  • Verify you're inside tmux: echo $TMUX\n"+
						"  • Check tmux is running: tmux list-sessions")
				return err
			}

			// Look up parent session by tmux name
			manifest, err := findManifestByTmuxName(adapter, currentTmuxName)
			if err != nil {
				ui.PrintError(err,
					"Failed to find parent session",
					"  • Provide parent session ID explicitly: agm session create-child <parent-id>\n"+
						"  • Run 'agm session list' to see available sessions")
				return err
			}
			parentSessionID = manifest.SessionID
			fmt.Printf("Using current tmux session as parent: %s (%s)\n", currentTmuxName, parentSessionID)
		} else {
			ui.PrintError(
				fmt.Errorf("no parent session ID provided"),
				"Parent session ID required",
				"  • Provide parent session ID: agm session create-child <parent-id>\n"+
					"  • Or run from within a tmux session to auto-detect parent")
			return fmt.Errorf("parent session ID required")
		}
	} else {
		parentSessionID = args[0]
		if len(args) > 1 {
			childSessionName = args[1]
		}
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

	// Prompt for child session name if not provided
	if childSessionName == "" {
		var inputName string
		err = huh.NewInput().
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
			return err
		}
		childSessionName = inputName

		if childSessionName == "" {
			ui.PrintError(
				fmt.Errorf("session name cannot be empty"),
				"Invalid session name",
				"  • Provide a non-empty session name")
			return fmt.Errorf("empty session name")
		}
	}

	debug.Log("Child session name: %s", childSessionName)

	// Determine harness (inherit from parent unless --harness flag specified)
	selectedHarness := harnessName
	if selectedHarness == "" {
		selectedHarness = parentManifest.Harness
		debug.Log("Inheriting harness from parent: %s", selectedHarness)
	} else {
		debug.Log("Using explicit harness from flag: %s", selectedHarness)
	}

	// Validate harness
	if err := agent.ValidateHarnessName(selectedHarness); err != nil {
		ui.PrintError(err,
			"Invalid harness specified",
			"  • Valid harnesses: claude-code, gemini-cli, codex-cli, opencode-cli\n"+
				"  • Run 'agm harness list' to see available harnesses")
		return err
	}

	// Warn if harness unavailable (but allow session creation)
	if err := agent.ValidateHarnessAvailability(selectedHarness); err != nil {
		ui.PrintWarning(fmt.Sprintf("⚠️  %s", err.Error()))
	}

	// Get working directory (inherit from parent)
	workDir := parentManifest.Context.Project
	debug.Log("Inheriting working directory from parent: %s", workDir)

	// Get backend
	backendAdapter, err := backend.GetDefaultBackendAdapter()
	if err != nil {
		ui.PrintError(err,
			"Failed to get backend adapter",
			"  • Check AGM_SESSION_BACKEND environment variable\n"+
				"  • Valid backends: tmux")
		return err
	}

	// Check if child session name already exists
	exists, err := backendAdapter.HasSession(childSessionName)
	if err != nil {
		ui.PrintError(err,
			"Failed to check for existing session",
			"  • Verify backend is available (tmux)\n"+
				"  • Check session name is valid")
		return err
	}

	if exists {
		ui.PrintError(
			fmt.Errorf("session already exists: %s", childSessionName),
			"Session name conflict",
			"  • Choose a different name\n"+
				"  • Run 'agm session list' to see existing sessions")
		return fmt.Errorf("session already exists: %s", childSessionName)
	}

	// Create child session
	debug.Phase("Create Child Session Manifest")
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, childSessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintError(err,
			"Failed to create manifest directory",
			"  • Check permissions on sessions directory\n"+
				"  • Verify disk space available")
		return err
	}

	// Create child manifest with parent reference
	childSessionID := uuid.New().String()
	debug.Log("Generated child session ID: %s", childSessionID)

	childManifest := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     childSessionID,
		Name:          childSessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Empty = active/stopped
		Context: manifest.Context{
			Project: workDir,
			Purpose: fmt.Sprintf("Child session of %s", parentManifest.Name),
			Tags:    nil,
			Notes:   "",
		},
		Tmux: manifest.Tmux{
			SessionName: childSessionName,
		},
		Harness: selectedHarness,
		Claude: manifest.Claude{
			UUID: "", // Will be populated by SessionStart hook
		},
	}

	// Inherit context if --context flag set
	if inheritContext {
		debug.Log("Inheriting context from parent")
		childManifest.Context.Purpose = parentManifest.Context.Purpose
		if len(parentManifest.Context.Tags) > 0 {
			childManifest.Context.Tags = append([]string{}, parentManifest.Context.Tags...)
		}
		childManifest.Context.Notes = fmt.Sprintf("Child of %s\n\n%s",
			parentManifest.Name, parentManifest.Context.Notes)
	}

	// Write manifest to Dolt
	if err := adapter.CreateSession(childManifest); err != nil {
		ui.PrintError(err,
			"Failed to create session in Dolt",
			"  • Check Dolt server is running\n"+
				"  • Verify database connection")
		return err
	}

	// Auto-commit manifest change if in git repo
	_ = git.CommitManifest(manifestPath, "create-child", childManifest.Name) // Errors logged internally

	debug.Log("Child session created in Dolt: %s", childSessionID)
	ui.PrintSuccess(fmt.Sprintf("Created child session: %s", childSessionID))

	// Write to database with parent reference
	if err := writeSessionToDatabase(childManifest, &parentSessionID); err != nil {
		// Non-fatal - manifest already created
		ui.PrintWarning(fmt.Sprintf("Failed to write to database: %v", err))
		debug.Log("Database write failed (non-fatal): %v", err)
	} else {
		debug.Log("Child session written to database with parent_session_id: %s", parentSessionID)
		ui.PrintSuccess("Session registered in database")
	}

	// Create backend session (tmux)
	debug.Phase("Create Backend Session")
	if err := backendAdapter.CreateSession(childSessionName, workDir); err != nil {
		ui.PrintError(err,
			"Failed to create backend session",
			"  • Verify backend is available (tmux)\n"+
				"  • Check working directory exists: "+workDir)
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
		defer database.Close()

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
	defer database.Close()

	// Note: The current db.CreateSession doesn't support parent_session_id parameter
	// We need to create the session first, then update it with the parent reference
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
