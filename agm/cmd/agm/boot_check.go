package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var bootCheckCmd = &cobra.Command{
	Use:   "boot-check",
	Short: "Verify system health on startup",
	Long: `Check that critical AGM infrastructure is running.

Verifies:
  - Orchestrator tmux session is alive
  - Overseer tmux session is alive
  - Loop heartbeats are fresh

Exit codes:
  0 = all checks passed
  1 = one or more issues found

Examples:
  agm boot-check`,
	Args: cobra.NoArgs,
	RunE: runBootCheck,
}

func init() {
	rootCmd.AddCommand(bootCheckCmd)
}

func runBootCheck(_ *cobra.Command, _ []string) error {
	checker := &BootChecker{
		ListSessions:   tmux.ListSessions,
		ListHeartbeats: monitoring.ListHeartbeats,
	}

	result := checker.Check()
	printBootCheckResult(result)

	if len(result.Errors) > 0 {
		os.Exit(1)
	}
	return nil
}

// BootCheckIssue represents a single issue found during boot check.
type BootCheckIssue struct {
	Component   string // "orchestrator", "overseer", "heartbeat"
	Message     string
	Remediation string
}

// BootCheckResult holds the outcome of a boot invariant check.
type BootCheckResult struct {
	Errors   []BootCheckIssue
	Warnings []BootCheckIssue
}

// BootChecker holds injectable dependencies for boot invariant checks.
type BootChecker struct {
	ListSessions   func() ([]string, error)
	ListHeartbeats func(dir string) ([]*monitoring.LoopHeartbeat, error)
}

// Check runs all boot invariant checks and returns the result.
func (bc *BootChecker) Check() *BootCheckResult {
	result := &BootCheckResult{}

	bc.checkSupervisorSessions(result)
	bc.checkHeartbeats(result)

	return result
}

func (bc *BootChecker) checkSupervisorSessions(result *BootCheckResult) {
	sessions, err := bc.ListSessions()
	if err != nil {
		result.Errors = append(result.Errors, BootCheckIssue{
			Component:   "tmux",
			Message:     fmt.Sprintf("Failed to list tmux sessions: %v", err),
			Remediation: "Ensure tmux is installed and running: tmux ls",
		})
		return
	}

	hasOrchestrator := false
	hasOverseer := false

	for _, name := range sessions {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "orchestrator") {
			hasOrchestrator = true
		}
		if strings.Contains(lower, "overseer") {
			hasOverseer = true
		}
	}

	if !hasOrchestrator {
		result.Errors = append(result.Errors, BootCheckIssue{
			Component:   "orchestrator",
			Message:     "No orchestrator session found",
			Remediation: "Start orchestrator: agm session new orchestrator-v2",
		})
	}

	if !hasOverseer {
		result.Errors = append(result.Errors, BootCheckIssue{
			Component:   "overseer",
			Message:     "No overseer session found",
			Remediation: "Start overseer: agm session new overseer",
		})
	}
}

func (bc *BootChecker) checkHeartbeats(result *BootCheckResult) {
	heartbeats, err := bc.ListHeartbeats("")
	if err != nil {
		result.Warnings = append(result.Warnings, BootCheckIssue{
			Component:   "heartbeat",
			Message:     fmt.Sprintf("Failed to read heartbeats: %v", err),
			Remediation: "Check heartbeat directory: ls ~/.agm/heartbeats/",
		})
		return
	}

	if len(heartbeats) == 0 {
		return
	}

	for _, hb := range heartbeats {
		status := monitoring.CheckStaleness(hb)
		if status == "stale" {
			age := time.Since(hb.Timestamp)
			result.Warnings = append(result.Warnings, BootCheckIssue{
				Component:   "heartbeat",
				Message:     fmt.Sprintf("Stale heartbeat for %s (age: %s)", hb.Session, age.Round(time.Second)),
				Remediation: fmt.Sprintf("Check session health: agm heartbeat check %s", hb.Session),
			})
		}
	}
}

func printBootCheckResult(result *BootCheckResult) {
	if len(result.Errors) == 0 && len(result.Warnings) == 0 {
		fmt.Println(ui.Green("✓ All boot checks passed"))
		return
	}

	for _, issue := range result.Errors {
		fmt.Printf("%s [%s] %s\n", ui.Red("✗"), issue.Component, issue.Message)
		fmt.Printf("  → %s\n", issue.Remediation)
	}

	for _, issue := range result.Warnings {
		fmt.Printf("%s [%s] %s\n", ui.Yellow("!"), issue.Component, issue.Message)
		fmt.Printf("  → %s\n", issue.Remediation)
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n%s\n", ui.Red(fmt.Sprintf("%d error(s), %d warning(s)", len(result.Errors), len(result.Warnings))))
	} else {
		fmt.Printf("\n%s\n", ui.Yellow(fmt.Sprintf("%d warning(s)", len(result.Warnings))))
	}
}
