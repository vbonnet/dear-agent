package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/importer"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	registerName       string
	registerWorkspace  string
	registerProjectDir string
	registerQuiet      bool
)

var registerCmd = &cobra.Command{
	Use:   "register <uuid>",
	Short: "Idempotently track a Claude conversation by UUID (hook-safe import)",
	Long: `Idempotently ensure an AGM manifest exists for a Claude conversation UUID.

register is the non-interactive, repeatable sibling of "agm session import":

  • Idempotent  - re-running on an already-tracked UUID is a no-op that returns
                  the existing session, instead of erroring or creating a
                  duplicate manifest with a fresh ID.
  • Non-interactive - never prompts; if no --name is given it derives one from
                  the conversation's project directory. This is what makes it
                  safe to call from a SessionStart hook.
  • Correct workspace - when --workspace is omitted, the workspace label is
                  inferred from the conversation's own project directory (its
                  nearest git/WORKSPACE.yaml root) rather than defaulting to the
                  active config workspace, which mis-attributes every session to
                  one workspace.

Arguments:
  uuid - Claude conversation UUID to register

Flags:
  --name        - Name for the AGM session (optional; derived from project if omitted)
  --workspace   - Workspace label (optional; inferred from the project if omitted)
  --project-dir - Project directory to use directly (skips conversation file scan;
                  use in SessionStart hooks where the .jsonl has not been written yet)
  --quiet       - Suppress success output (print only the session ID); useful in hooks

Examples:
  agm session register 370980e1-e16c-48a1-9d17-caca0d3910ba
  agm session register <uuid> --workspace oss --name recovered-work
  agm session register <uuid> --quiet   # hook-friendly
  agm session register <uuid> --project-dir /Users/me/src/my-repo --quiet  # SessionStart hook`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conversationUUID := args[0]
		if len(conversationUUID) < 8 {
			return fmt.Errorf("invalid UUID format: %s (too short)", conversationUUID)
		}

		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer func() { _ = adapter.Close() }()

		result, err := importer.RegisterSession(conversationUUID, registerName, registerWorkspace, cfg.Workspace, registerProjectDir, adapter)
		if err != nil {
			ui.PrintError(err,
				"Failed to register session",
				"  • Verify UUID exists: ls ~/.claude/projects/*/<uuid>.jsonl\n"+
					"  • Provide a workspace explicitly: --workspace=oss\n"+
					"  • Check existing sessions: agm session list")
			return err
		}

		if registerQuiet {
			fmt.Println(result.SessionID)
			return nil
		}

		if result.AlreadyTracked {
			ui.PrintSuccess(fmt.Sprintf("Already tracked: %s", result.Name))
		} else {
			ui.PrintSuccess(fmt.Sprintf("Registered session: %s", result.Name))
		}
		fmt.Printf("\n")
		fmt.Printf("  Session ID:        %s\n", result.SessionID)
		fmt.Printf("  Conversation UUID: %s\n", conversationUUID)
		fmt.Printf("  Workspace:         %s\n", result.Workspace)
		fmt.Printf("  Project:           %s\n", result.Project)
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(registerCmd)

	registerCmd.Flags().StringVar(&registerName, "name", "", "Name for the session (derived from project if omitted)")
	registerCmd.Flags().StringVar(&registerWorkspace, "workspace", "", "Workspace label (inferred from project if omitted)")
	registerCmd.Flags().StringVar(&registerProjectDir, "project-dir", "", "Project directory (skips .jsonl scan; pass cwd from SessionStart hook payload)")
	registerCmd.Flags().BoolVar(&registerQuiet, "quiet", false, "Print only the session ID (hook-friendly)")
}
