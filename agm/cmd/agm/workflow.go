package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/workflow"
)

var (
	workflowListAgent string
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage AI agent workflows",
	Long: `Manage AI agent workflows (specialized execution modes).

Workflows define how agents should behave for specific tasks:
- deep-research: Research URLs and synthesize insights
- code-review: Analyze code changes and provide feedback
- architect: Design system architectures

Examples:
  agm workflow list                       # List all workflows
  agm workflow list --harness=gemini-cli  # List workflows compatible with Gemini CLI`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available workflows",
	Long: `List available workflows and their compatibility with harnesses.

By default, lists all workflows. Use --harness flag to filter by harness compatibility.

Examples:
  agm workflow list                        # List all workflows
  agm workflow list --harness=gemini-cli   # List workflows compatible with Gemini CLI
  agm workflow list --harness=claude-code  # List workflows compatible with Claude Code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var workflows []workflow.Workflow

		if workflowListAgent != "" {
			// Filter by harness
			workflows = workflow.ListForHarness(workflowListAgent)
			if len(workflows) == 0 {
				fmt.Printf("No workflows compatible with harness '%s'\n", workflowListAgent)
				fmt.Println("\nAvailable workflows:")
				allWorkflows := workflow.List()
				for _, w := range allWorkflows {
					fmt.Printf("  - %s: %s\n", w.Name(), strings.Join(w.SupportedHarnesses(), ", "))
				}
				return nil
			}
		} else {
			// List all workflows
			workflows = workflow.List()
			if len(workflows) == 0 {
				fmt.Println("No workflows registered")
				return nil
			}
		}

		// Display header
		if workflowListAgent != "" {
			fmt.Printf("Workflows compatible with %s:\n\n", workflowListAgent)
		} else {
			fmt.Println("Available workflows:")
		}

		// Display workflows
		for _, w := range workflows {
			fmt.Printf("  %s\n", w.Name())
			fmt.Printf("    Description: %s\n", w.Description())
			fmt.Printf("    Supported harnesses: %s\n", strings.Join(w.SupportedHarnesses(), ", "))
			fmt.Println()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(workflowCmd)
	workflowCmd.AddCommand(workflowListCmd)

	workflowListCmd.Flags().StringVar(&workflowListAgent, "harness", "", "Filter workflows by harness compatibility")
}
