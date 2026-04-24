package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/pkg/sandbox"
)

var CleanupSandboxesCmd = &cobra.Command{
	Use:   "cleanup-sandboxes",
	Short: "Remove all sandboxes",
	Long: `Remove all sandboxes to reclaim disk space.

Prompts for confirmation unless --force flag is used.

Flags:
  --force    Skip confirmation prompt

Example:
  wayfinder-session cleanup-sandboxes
  wayfinder-session cleanup-sandboxes --force`,
	Args: cobra.NoArgs,
	RunE: runCleanupSandboxes,
}

func init() {
	CleanupSandboxesCmd.Flags().Bool("force", false, "Skip confirmation prompt")
}

func runCleanupSandboxes(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	force, _ := cmd.Flags().GetBool("force")

	mgr := sandbox.NewManager(projectDir)

	// Get sandbox list for count
	sandboxes, err := mgr.ListSandboxes()
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if len(sandboxes) == 0 {
		fmt.Println("No sandboxes to clean up")
		return nil
	}

	// Confirmation prompt (unless --force)
	if !force {
		fmt.Printf("Delete %d sandbox(es)? (y/N): ", len(sandboxes))
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Cleanup all sandboxes
	cleaned := 0
	for _, sb := range sandboxes {
		if err := mgr.CleanupSandbox(sb.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup sandbox %s: %v\n", sb.Name, err)
			continue
		}
		cleaned++
	}

	fmt.Printf("✅ Cleaned up %d sandbox(es)\n", cleaned)
	return nil
}
