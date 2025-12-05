package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and configuration",
	Long: `Verify that Claude, tmux, and all sessions are healthy.

Examples:
  csm doctor    # Run health checks`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Blue("=== Claude Session Manager Health Check ===\n"))

		allHealthy := true

		// Check Claude installation (verify history.jsonl exists)
		homeDir, _ := os.UserHomeDir()
		historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")
		if _, err := os.Stat(historyPath); err != nil {
			ui.PrintError(err, "Claude history not found", "  • Install Claude from https://claude.com\n  • Run Claude at least once")
			allHealthy = false
		} else {
			ui.PrintSuccess("Claude history found")
		}

		// Check tmux installation
		tmuxVersion, err := tmux.Version()
		if err != nil {
			ui.PrintError(err, "tmux not found or not working", "  • Install tmux: sudo apt install tmux")
			allHealthy = false
		} else {
			ui.PrintSuccess(fmt.Sprintf("tmux installed: %s", tmuxVersion))
		}

		// Check sessions directory
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Sessions directory not found: %s", cfg.SessionsDir))
			ui.PrintSuccess("Run 'csm sync' to create manifests")
		} else {
			ui.PrintSuccess(fmt.Sprintf("Found %d session manifests", len(manifests)))

			// Check health of each session
			unhealthyCount := 0
			for _, m := range manifests {
				health, err := session.CheckHealth(m)
				if err != nil {
					ui.PrintError(err, fmt.Sprintf("Failed to check health of %s", m.SessionID), "")
					unhealthyCount++
					continue
				}

				if !health.IsHealthy() {
					ui.PrintWarning(fmt.Sprintf("Unhealthy session: %s", m.SessionID))
					fmt.Println(health.Summary())
					unhealthyCount++
				}
			}

			if unhealthyCount > 0 {
				ui.PrintWarning(fmt.Sprintf("%d unhealthy sessions found", unhealthyCount))
				allHealthy = false
			} else if len(manifests) > 0 {
				ui.PrintSuccess("All sessions are healthy")
			}
		}

		// Overall status
		fmt.Println()
		if allHealthy {
			ui.PrintSuccess("✓ System is healthy")
		} else {
			ui.PrintWarning("⚠ Some issues found")
			return fmt.Errorf("health check failed")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
