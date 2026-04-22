package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/importer"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	importName      string
	importWorkspace string
)

var importCmd = &cobra.Command{
	Use:   "import <uuid>",
	Short: "Import orphaned conversation by UUID",
	Long: `Import orphaned conversation by creating an AGM manifest.

This command imports a Claude conversation that exists in history.jsonl
but has no AGM manifest. It will:
1. Validate the UUID exists in Claude's conversation files
2. Check that no manifest already exists for this UUID
3. Infer the project directory from the conversation file location
4. Extract metadata from history.jsonl (last activity, project path)
5. Create an AGM manifest with auto-sanitized tmux session name

Arguments:
  uuid - Claude conversation UUID to import

Flags:
  --name      - Name for the AGM session (optional, prompts if not provided)
  --workspace - Workspace name (e.g., "oss", "acme")
                If omitted, uses auto-detected workspace or prompts

Examples:
  agm session import 370980e1-e16c-48a1-9d17-caca0d3910ba
  agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 --name my-session
  agm session import orphan-uuid --workspace oss --name recovered-work`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conversationUUID := args[0]

		// Validate UUID format (basic check)
		if len(conversationUUID) < 8 {
			return fmt.Errorf("invalid UUID format: %s (too short)", conversationUUID)
		}

		// Get session name (prompt if not provided)
		sessionName := importName
		if sessionName == "" {
			var inputName string
			err := huh.NewInput().
				Title("Enter session name for imported conversation:").
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
					"  • Provide name with --name flag: agm session import <uuid> --name <name>\n"+
						"  • Check terminal is interactive (TTY)")
				return err
			}
			sessionName = inputName
		}

		// Get workspace (use detected or prompt)
		workspace := importWorkspace
		if workspace == "" {
			workspace = cfg.Workspace
		}
		if workspace == "" {
			return fmt.Errorf("workspace not specified and auto-detection failed\n\n" +
				"Solutions:\n" +
				"  • Provide workspace explicitly: --workspace=oss\n" +
				"  • Run from within a workspace directory\n" +
				"  • Check ~/.agm/config.yaml for workspace configuration")
		}

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer adapter.Close()

		// Get sessions directory for this workspace (for YAML backward compat)
		sessionsDir := cfg.SessionsDir
		if sessionsDir == "" {
			// Fallback to legacy ~/sessions if workspace config not available
			home, _ := os.UserHomeDir()
			sessionsDir = filepath.Join(home, "sessions")
		}

		// Import the orphaned session
		fmt.Printf("Importing conversation %s...\n", conversationUUID)

		sessionID, err := importer.ImportOrphanedSession(conversationUUID, sessionName, workspace, adapter, sessionsDir)
		if err != nil {
			ui.PrintError(err,
				"Failed to import orphaned session",
				"  • Verify UUID exists: ls ~/.claude/projects/*/<uuid>.jsonl\n"+
					"  • Check if already imported: agm session list\n"+
					"  • Verify workspace is correct: agm workspace list")
			return err
		}

		// Success!
		ui.PrintSuccess(fmt.Sprintf("Imported session: %s", sessionName))
		fmt.Printf("\n")
		fmt.Printf("  Session ID:       %s\n", sessionID)
		fmt.Printf("  Conversation UUID: %s\n", conversationUUID)
		fmt.Printf("  Workspace:        %s\n", workspace)
		fmt.Printf("  Manifest:         %s\n", filepath.Join(sessionsDir, sessionID, "manifest.yaml"))
		fmt.Printf("\n")
		fmt.Printf("Next steps:\n")
		fmt.Printf("  • Resume session: agm session resume %s\n", sessionName)
		fmt.Printf("  • View details:   agm session list\n")

		return nil
	},
}

func init() {
	sessionCmd.AddCommand(importCmd)

	importCmd.Flags().StringVar(&importName, "name", "", "Name for the imported session")
	importCmd.Flags().StringVar(&importWorkspace, "workspace", "", "Workspace name (e.g., oss, acme)")
}
