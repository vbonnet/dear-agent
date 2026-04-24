package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/mcp"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
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
		return outputJSONMCP(cmd, results)
	}

	return outputTableMCP(cmd, results)
}

func outputTableMCP(cmd *cobra.Command, results map[string]mcp.DetectionResult) error {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "Global MCP Server Status:")
	fmt.Fprintln(out, "")

	// Calculate column widths
	nameWidth := 15
	statusWidth := 12
	urlWidth := 40

	// Header
	fmt.Fprintf(out, "%-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", statusWidth, "STATUS", urlWidth, "URL", "ERROR")
	fmt.Fprintf(out, "%-*s  %-*s  %-*s  %s\n", nameWidth, "----", statusWidth, "------", urlWidth, "---", "-----")

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

		fmt.Fprintf(out, "%-*s  %-*s  %-*s  %s\n", nameWidth, name, statusWidth, status, urlWidth, url, errorMsg)
	}

	fmt.Fprintln(out, "")

	// Summary
	available := 0
	for _, result := range results {
		if result.Available {
			available++
		}
	}

	fmt.Fprintf(out, "Summary: %d/%d global MCPs available\n", available, len(results))

	return nil
}

func outputJSONMCP(cmd *cobra.Command, results map[string]mcp.DetectionResult) error {
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

	// Use cliframe JSON formatter
	formatter, err := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
	if err != nil {
		return err
	}

	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer = writer.WithFormatter(formatter)
	return writer.Output(jsonResults)
}
