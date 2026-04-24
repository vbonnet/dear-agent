package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/vbonnet/dear-agent/agm/internal/monitor/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

var (
	captureLines   int
	captureHistory bool
	captureTail    int
	captureJSON    bool
	captureYAML    bool
	captureFilter  string
)

var captureCmd = &cobra.Command{
	Use:   "capture <session-name>",
	Short: "Capture session output from tmux pane",
	Long: `Capture output from the target session's tmux pane.

This command provides access to the session's current visible content
or full scrollback history, with optional filtering and formatting.`,
	Example: `  # Capture visible content (default: 50 lines)
  agm capture my-session

  # Capture specific number of lines
  agm capture my-session --lines 100

  # Capture last N lines (tail mode)
  agm capture my-session --tail 20

  # Capture full scrollback history
  agm capture my-session --history

  # Output as JSON
  agm capture my-session --json

  # Output as YAML
  agm capture my-session --yaml

  # Filter output with regex
  agm capture my-session --filter "error|warning"`,
	Args: cobra.ExactArgs(1),
	RunE: runCapture,
}

func init() {
	rootCmd.AddCommand(captureCmd)

	captureCmd.Flags().IntVarP(&captureLines, "lines", "n", 50, "Number of lines to capture")
	captureCmd.Flags().BoolVar(&captureHistory, "history", false, "Capture full scrollback history")
	captureCmd.Flags().IntVar(&captureTail, "tail", 0, "Capture last N lines only")
	captureCmd.Flags().BoolVar(&captureJSON, "json", false, "Output as JSON")
	captureCmd.Flags().BoolVar(&captureYAML, "yaml", false, "Output as YAML")
	captureCmd.Flags().StringVar(&captureFilter, "filter", "", "Filter lines with regex pattern")

	captureCmd.MarkFlagsMutuallyExclusive("json", "yaml")
	captureCmd.MarkFlagsMutuallyExclusive("lines", "tail", "history")
}

func runCapture(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Get Dolt adapter for session resolution
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	// Resolve session
	m, _, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir, adapter)
	if err != nil {
		return err
	}

	tmuxSessionName := m.Tmux.SessionName
	if tmuxSessionName == "" {
		return fmt.Errorf("session %s has no tmux session", sessionName)
	}

	// Check if tmux session exists
	exists, err := tmux.SessionExists(tmuxSessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("tmux session %s does not exist", tmuxSessionName)
	}

	// Capture content based on mode
	var lines []string
	if captureHistory {
		lines, err = tmux.CapturePaneHistoryLines(tmuxSessionName, 0)
	} else if captureTail > 0 {
		lines, err = tmux.CapturePaneLines(tmuxSessionName, captureTail)
		// Get last N lines
		if len(lines) > captureTail {
			lines = lines[len(lines)-captureTail:]
		}
	} else {
		lines, err = tmux.CapturePaneLines(tmuxSessionName, captureLines)
	}

	if err != nil {
		return fmt.Errorf("failed to capture pane content: %w", err)
	}

	// Apply filter if specified
	if captureFilter != "" {
		filtered, err := filterLines(lines, captureFilter)
		if err != nil {
			return fmt.Errorf("failed to apply filter: %w", err)
		}
		lines = filtered
	}

	// Output in requested format
	if captureJSON {
		return outputCaptureJSON(sessionName, lines)
	} else if captureYAML {
		return outputCaptureYAML(sessionName, lines)
	} else {
		return outputCaptureText(lines)
	}
}

func filterLines(lines []string, pattern string) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	var filtered []string
	for _, line := range lines {
		if re.MatchString(line) {
			filtered = append(filtered, line)
		}
	}
	return filtered, nil
}

func outputCaptureText(lines []string) error {
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

func outputCaptureJSON(sessionName string, lines []string) error {
	output := map[string]interface{}{
		"session":   sessionName,
		"lines":     lines,
		"count":     len(lines),
		"timestamp": tmux.Now(),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputCaptureYAML(sessionName string, lines []string) error {
	output := map[string]interface{}{
		"session":   sessionName,
		"lines":     lines,
		"count":     len(lines),
		"timestamp": tmux.Now(),
	}

	encoder := yaml.NewEncoder(os.Stdout)
	defer encoder.Close()
	return encoder.Encode(output)
}
