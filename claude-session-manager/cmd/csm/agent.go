package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

var jsonOutput bool

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agents",
	Long:  `Manage AI agents used by AGM (AI Grooming Manager).`,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available AI agents",
	Long: `List all supported AI agents with availability status and capabilities.

Availability is determined by API key presence:
  - claude: ANTHROPIC_API_KEY
  - gemini: GEMINI_API_KEY
  - gpt:    OPENAI_API_KEY

Examples:
  agm agent list          # Table output (human-readable)
  agm agent list --json   # JSON output (for scripting)`,
	RunE: runAgentList,
}

func init() {
	agentListCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON for scripting")
	agentCmd.AddCommand(agentListCmd)
	rootCmd.AddCommand(agentCmd)
}

func runAgentList(cmd *cobra.Command, args []string) error {
	agents := agent.GetAllAgents()

	if jsonOutput {
		return outputJSON(agents)
	}
	return outputTable(agents)
}

func outputTable(agents []agent.AgentInfo) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "AGENT\tSTATUS\tMODEL\tFEATURES")

	// Rows
	for _, a := range agents {
		features := formatFeatures(a.Capabilities)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			a.Name, a.Status, a.Model, features)
	}

	return nil
}

func outputJSON(agents []agent.AgentInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(agents)
}

func formatFeatures(caps agent.Capabilities) string {
	features := []string{}

	if caps.SupportsTools {
		features = append(features, "tools")
	}
	if caps.SupportsVision {
		features = append(features, "vision")
	}

	// Format context window (e.g., 200000 -> "200K context")
	contextK := caps.MaxContextWindow / 1000
	features = append(features, fmt.Sprintf("%dK context", contextK))

	return strings.Join(features, ", ")
}
