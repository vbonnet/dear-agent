package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/pkg/sandbox"
)

var CreateSandboxCmd = &cobra.Command{
	Use:   "create-sandbox <name>",
	Short: "Create a new isolated sandbox",
	Long: `Create a new isolated sandbox for concurrent project execution.

Sandboxes provide directory + git worktree isolation (95% coverage).

Example:
  wayfinder-session create-sandbox test-sandbox`,
	Args: cobra.ExactArgs(1),
	RunE: runCreateSandbox,
}

func runCreateSandbox(cmd *cobra.Command, args []string) error {
	name := args[0]
	projectDir := GetProjectDirectory()

	mgr := sandbox.NewManager(projectDir)
	sb, err := mgr.CreateSandbox(name)
	if err != nil {
		return fmt.Errorf("failed to create sandbox '%s': %w", name, err)
	}

	fmt.Printf("✅ Sandbox created: %s\n", sb.Name)
	fmt.Printf("   ID: %s\n", sb.ID)
	if sb.WorktreePath != "" {
		fmt.Printf("   Worktree: %s\n", sb.WorktreePath)
	}
	return nil
}
