package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/pkg/sandbox"
)

// ListSandboxesCmd is the cobra command that lists all sandboxes.
var ListSandboxesCmd = &cobra.Command{
	Use:   "list-sandboxes",
	Short: "List all sandboxes",
	Long: `List all sandboxes with ID, name, and creation time.

Flags:
  --format    Output format: table (default), json

Example:
  wayfinder-session list-sandboxes
  wayfinder-session list-sandboxes --format json`,
	Args: cobra.NoArgs,
	RunE: runListSandboxes,
}

func init() {
	ListSandboxesCmd.Flags().String("format", "table", "Output format (table, json)")
}

func runListSandboxes(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	format, _ := cmd.Flags().GetString("format")

	mgr := sandbox.NewManager(projectDir)
	sandboxes, err := mgr.ListSandboxes()
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if len(sandboxes) == 0 {
		fmt.Println("No sandboxes found")
		return nil
	}

	if format == "json" {
		output, _ := json.MarshalIndent(sandboxes, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	// Table format (default)
	fmt.Printf("%-20s %-40s %-20s\n", "Name", "ID", "Created")
	fmt.Printf("%-20s %-40s %-20s\n", "----", "--", "-------")
	for _, sb := range sandboxes {
		fmt.Printf("%-20s %-40s %-20s\n", sb.Name, sb.ID, sb.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
