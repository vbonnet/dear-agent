package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/importer"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	importName      string
	importWorkspace string
	importHarness   string
)

var importCmd = &cobra.Command{
	Use:   "import <uuid>",
	Short: "Import orphaned conversation by UUID",
	Long: `Import orphaned conversation by creating an AGM manifest.

This command imports a Claude, Codex, AGY, or Pi conversation that exists in the
harness saved-session history but has no AGM manifest.

For Claude, it validates the UUID under ~/.claude/projects. For Codex, it
resolves ~/.codex/sessions/**/rollout-*.jsonl by session_meta.session_id. For
AGY, it resolves ~/.gemini/antigravity-cli/conversations/<id>.db plus AGY
cache/log metadata for the source workspace path.
For Pi, it resolves an exact JSONL header ID below the native Pi session tree,
copies the transcript into AGM-owned private storage, and preserves its cwd.
It will:
1. Validate the UUID exists in the selected harness saved-session files
2. Check that no manifest already exists for this UUID
3. Infer the working directory from harness metadata
4. Extract metadata when available (last activity, project path)
5. Create an AGM manifest with auto-sanitized tmux session name

Arguments:
  uuid - Harness conversation UUID to import

Flags:
  --name       - Name for the AGM session (optional, prompts if not provided)
  --harness    - Harness to import (claude-code, codex-cli, agy, pi-cli; default claude-code)
  --workspace  - Workspace name (e.g., "oss", "acme")
                 If omitted, uses auto-detected workspace or prompts

Examples:
  agm session import 370980e1-e16c-48a1-9d17-caca0d3910ba
  agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 --name my-session
  agm session import 019ef2af-97e0-7443-9f07-03e40636740c --harness codex-cli --name recovered-codex
  agm session import 117ff898-a964-4a9f-b460-1be4a8a49b17 --harness agy --name recovered-agy
  agm session import pi-native-session --harness pi-cli --name recovered-pi
  agm session import orphan-uuid --workspace oss --name recovered-work`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conversationUUID := args[0]
		importHarness = agent.NormalizeHarnessName(importHarness)

		// Validate UUID format (basic check)
		if importHarness == "pi-cli" {
			if err := pisession.ValidateID(conversationUUID); err != nil {
				return err
			}
		} else if len(conversationUUID) < 8 {
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
		defer func() { _ = adapter.Close() }()

		// Get sessions directory for this workspace (for YAML backward compat)
		sessionsDir := cfg.SessionsDir
		if sessionsDir == "" {
			// Fallback to legacy ~/sessions if workspace config not available
			home, _ := os.UserHomeDir()
			sessionsDir = filepath.Join(home, "sessions")
		}

		// Import the orphaned session
		fmt.Printf("Importing conversation %s...\n", conversationUUID)

		sessionID, err := importer.ImportOrphanedSessionWithOptions(conversationUUID, sessionName, workspace, adapter, sessionsDir, importer.ImportOptions{
			Harness: importHarness,
		})
		if err != nil {
			ui.PrintError(err,
				"Failed to import orphaned session",
				"  • Verify UUID exists in the selected harness saved-session store\n"+
					"  • Claude: ls ~/.claude/projects/*/<uuid>.jsonl\n"+
					"  • Codex: rg '<uuid>' ~/.codex/sessions ~/.codex/archived_sessions\n"+
					"  • AGY: ls ~/.gemini/antigravity-cli/conversations/<uuid>.db\n"+
					"  • Pi: rg '\"id\":\"<id>\"' ~/.pi/agent/sessions ~/.agm/pi/sessions\n"+
					"  • Check if already imported: agm session list\n"+
					"  • Verify workspace is correct: agm workspace list")
			return err
		}

		// Success!
		ui.PrintSuccess(fmt.Sprintf("Imported session: %s", sessionName))
		fmt.Printf("\n")
		fmt.Printf("  Session ID:       %s\n", sessionID)
		fmt.Printf("  Conversation UUID: %s\n", conversationUUID)
		fmt.Printf("  Harness:          %s\n", importHarness)
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
	importCmd.Flags().StringVar(&importHarness, "harness", "claude-code", "Harness for the imported session (claude-code, codex-cli, agy, pi-cli)")
	importCmd.Flags().StringVar(&importWorkspace, "workspace", "", "Workspace name (e.g., oss, acme)")
}
