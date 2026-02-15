package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/mcp"
)

var mcpStatusCmd = &cobra.Command{
	Use:   "mcp-status",
	Short: "Check status of global MCP servers",
	Long: `Check the status of all configured global MCP servers.

This command performs health checks on all global MCP servers configured in
~/.config/agm/mcp.yaml and reports their availability.

Global MCPs are HTTP/SSE MCP servers that can be shared across multiple AGM
sessions. If a global MCP is available, AGM will use it instead of spawning
a new stdio MCP process for each session.

Examples:
  agm mcp-status              # Check all global MCPs
  agm mcp-status --json       # Output in JSON format`,
	RunE: runMCPStatus,
}

var (
	jsonOutput bool
)

func init() {
	mcpStatusCmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	rootCmd.AddCommand(mcpStatusCmd)
}

func runMCPStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get MCP status
	results, err := mcp.GetGlobalMCPStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to check MCP status: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No global MCPs configured.")
		fmt.Println("")
		fmt.Println("To configure global MCPs, create ~/.config/agm/mcp.yaml:")
		fmt.Println("")
		fmt.Println("mcp_servers:")
		fmt.Println("  - name: googledocs")
		fmt.Println("    url: http://localhost:8001")
		fmt.Println("    type: mcp")
		fmt.Println("")
		return nil
	}

	if jsonOutput {
		return outputJSON(results)
	}

	return outputTable(results)
}

func outputTable(results map[string]mcp.DetectionResult) error {
	fmt.Println("Global MCP Server Status:")
	fmt.Println("")

	// Calculate column widths
	nameWidth := 15
	statusWidth := 12
	urlWidth := 40

	// Header
	fmt.Printf("%-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", statusWidth, "STATUS", urlWidth, "URL", "ERROR")
	fmt.Printf("%-*s  %-*s  %-*s  %s\n", nameWidth, "----", statusWidth, "------", urlWidth, "---", "-----")

	// Rows
	for name, result := range results {
		status := "UNAVAILABLE"
		if result.Available {
			status = "AVAILABLE"
		}

		errorMsg := ""
		if result.Error != nil {
			errorMsg = result.Error.Error()
			// Truncate long error messages
			if len(errorMsg) > 50 {
				errorMsg = errorMsg[:47] + "..."
			}
		}

		// Truncate URL if too long
		url := result.URL
		if len(url) > urlWidth {
			url = url[:urlWidth-3] + "..."
		}

		fmt.Printf("%-*s  %-*s  %-*s  %s\n", nameWidth, name, statusWidth, status, urlWidth, url, errorMsg)
	}

	fmt.Println("")

	// Summary
	available := 0
	for _, result := range results {
		if result.Available {
			available++
		}
	}

	fmt.Printf("Summary: %d/%d global MCPs available\n", available, len(results))

	return nil
}

func outputJSON(results map[string]mcp.DetectionResult) error {
	// Convert results to JSON-serializable format
	jsonResults := make(map[string]interface{})

	for name, result := range results {
		entry := map[string]interface{}{
			"name":      name,
			"available": result.Available,
			"url":       result.URL,
			"status":    result.Status,
		}

		if result.Error != nil {
			entry["error"] = result.Error.Error()
		}

		jsonResults[name] = entry
	}

	// Pretty print JSON
	fmt.Printf("{\n")
	i := 0
	for name, entry := range jsonResults {
		if i > 0 {
			fmt.Printf(",\n")
		}
		fmt.Printf("  \"%s\": {", name)

		entryMap := entry.(map[string]interface{})
		j := 0
		for key, value := range entryMap {
			if j > 0 {
				fmt.Printf(",")
			}
			fmt.Printf("\n    \"%s\": ", key)
			switch v := value.(type) {
			case string:
				fmt.Printf("\"%s\"", v)
			case bool:
				fmt.Printf("%t", v)
			default:
				fmt.Printf("%v", v)
			}
			j++
		}
		fmt.Printf("\n  }")
		i++
	}
	fmt.Printf("\n}\n")

	return nil
}
