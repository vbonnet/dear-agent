package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/audit"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	auditWorkspace string
	auditSeverity  string
	auditJSON      bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Comprehensive audit of AGM sessions",
	Long: `Run comprehensive health checks across all AGM sessions.

Audit checks include:
  • Orphaned conversations (sessions with .jsonl but no manifest)
  • Corrupted manifests (YAML parse errors, missing required fields)
  • Missing tmux sessions (manifest exists but tmux session doesn't)
  • Stale sessions (>30 days inactive, based on manifest.updated_at)
  • Duplicate UUIDs (multiple manifests claiming same Claude conversation UUID)
  • Chezmoi drift (managed dotfiles edited outside chezmoi; system-level)

Output is grouped by severity (Critical/Warning/Info) with actionable recommendations.

Examples:
  agm admin audit                          # Audit all workspaces
  agm admin audit --workspace oss          # Audit specific workspace
  agm admin audit --severity critical      # Show only critical issues
  agm admin audit --json                   # JSON output for scripting

Severity Levels:
  critical - Issues requiring immediate attention (corrupted data, duplicates)
  warning  - Issues that may impact functionality (orphans, missing tmux)
  info     - Informational issues (stale sessions)`,
	RunE: runAudit,
}

func init() {
	adminCmd.AddCommand(auditCmd)

	auditCmd.Flags().StringVar(&auditWorkspace, "workspace", "",
		"Filter audit to specific workspace")
	auditCmd.Flags().StringVar(&auditSeverity, "severity", "",
		"Filter by minimum severity level (critical/warning/info)")
	auditCmd.Flags().BoolVar(&auditJSON, "json", false,
		"Output results in JSON format for scripting")
}

func runAudit(cmd *cobra.Command, args []string) error {
	// Get sessions directory from config
	sessionsDir := cfg.SessionsDir
	if sessionsDir == "" {
		homeDir, _ := os.UserHomeDir()
		sessionsDir = homeDir + "/sessions"
	}

	// Validate severity filter
	var severityFilter audit.Severity
	if auditSeverity != "" {
		switch auditSeverity {
		case "critical":
			severityFilter = audit.SeverityCritical
		case "warning":
			severityFilter = audit.SeverityWarning
		case "info":
			severityFilter = audit.SeverityInfo
		default:
			return fmt.Errorf("invalid severity level: %s (must be critical/warning/info)", auditSeverity)
		}
	}

	// Get Dolt adapter for audit
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	// Run audit
	opts := audit.AuditOptions{
		SessionsDir:     sessionsDir,
		WorkspaceFilter: auditWorkspace,
		SeverityFilter:  severityFilter,
		Adapter:         adapter,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		return fmt.Errorf("audit failed: %w", err)
	}

	// Output results
	if auditJSON {
		return outputAuditJSON(report)
	}

	return outputHuman(report)
}

// outputAuditJSON outputs the audit report in JSON format
func outputAuditJSON(report *audit.AuditReport) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// outputHuman outputs the audit report in human-readable format
func outputHuman(report *audit.AuditReport) error {
	// Header
	fmt.Println(ui.Blue("═══ AGM Session Audit ═══"))
	fmt.Println()

	// Metadata
	fmt.Printf("Scan completed in %s\n", report.Duration)
	if report.WorkspaceFilter != "" {
		fmt.Printf("Workspace: %s\n", report.WorkspaceFilter)
	}
	if report.SeverityFilter != "" {
		fmt.Printf("Severity filter: %s+\n", report.SeverityFilter)
	}
	fmt.Println()

	// Statistics
	fmt.Println(ui.Blue("Statistics:"))
	fmt.Printf("  Manifests scanned: %d\n", report.ManifestsScanned)
	fmt.Printf("  History entries: %d\n", report.HistoryEntries)
	fmt.Printf("  Tmux sessions checked: %d\n", report.TmuxSessionsChecked)
	fmt.Println()

	// Show errors if any
	if len(report.Errors) > 0 {
		fmt.Println(ui.Yellow("⚠ Warnings:"))
		for _, e := range report.Errors {
			fmt.Printf("  • %s\n", e)
		}
		fmt.Println()
	}

	// Health status
	if report.Healthy {
		ui.PrintSuccess(fmt.Sprintf("✓ All checks passed! (%d sessions audited)", report.ManifestsScanned))
		return nil
	}

	// Issues summary
	fmt.Printf(ui.Yellow("Found %d issue(s):\n"), report.TotalIssues)
	fmt.Println()

	// Group issues by severity
	issuesBySeverity := groupBySeverity(report.Issues)

	// Display critical issues
	if len(issuesBySeverity[audit.SeverityCritical]) > 0 {
		fmt.Println(ui.Red("═══ CRITICAL ═══"))
		displayIssues(issuesBySeverity[audit.SeverityCritical])
		fmt.Println()
	}

	// Display warning issues
	if len(issuesBySeverity[audit.SeverityWarning]) > 0 {
		fmt.Println(ui.Yellow("═══ WARNINGS ═══"))
		displayIssues(issuesBySeverity[audit.SeverityWarning])
		fmt.Println()
	}

	// Display info issues
	if len(issuesBySeverity[audit.SeverityInfo]) > 0 {
		fmt.Println(ui.Blue("═══ INFO ═══"))
		displayIssues(issuesBySeverity[audit.SeverityInfo])
		fmt.Println()
	}

	// Summary by workspace
	if len(report.IssuesByWorkspace) > 1 {
		fmt.Println(ui.Blue("By Workspace:"))
		for workspace, count := range report.IssuesByWorkspace {
			wsName := workspace
			if wsName == "" {
				wsName = "(no workspace)"
			}
			fmt.Printf("  • %s: %d issue(s)\n", wsName, count)
		}
		fmt.Println()
	}

	// Summary by type
	fmt.Println(ui.Blue("By Issue Type:"))
	for issueType, count := range report.IssuesByType {
		fmt.Printf("  • %s: %d\n", issueType, count)
	}
	fmt.Println()

	// Recommendations summary
	if report.TotalIssues > 0 {
		fmt.Println(ui.Blue("Recommended Actions:"))
		displayRecommendationsSummary(report.Issues)
	}

	return nil
}

// groupBySeverity groups issues by severity level
func groupBySeverity(issues []*audit.AuditIssue) map[audit.Severity][]*audit.AuditIssue {
	grouped := make(map[audit.Severity][]*audit.AuditIssue)
	for _, issue := range issues {
		grouped[issue.Severity] = append(grouped[issue.Severity], issue)
	}
	return grouped
}

// displayIssues displays a list of issues in table format
func displayIssues(issues []*audit.AuditIssue) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "Session ID\tType\tWorkspace\tMessage\tRecommendation")
	fmt.Fprintln(w, "----------\t----\t---------\t-------\t--------------")

	// Sort by session ID for consistent output
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].SessionID < issues[j].SessionID
	})

	// Rows
	for _, issue := range issues {
		sessionID := issue.SessionID
		if sessionID == "" {
			sessionID = "-"
		}
		if len(sessionID) > 20 {
			sessionID = sessionID[:17] + "..."
		}

		issueType := string(issue.Type)

		workspace := issue.Workspace
		if workspace == "" {
			workspace = "-"
		}

		message := issue.Message
		if len(message) > 40 {
			message = message[:37] + "..."
		}

		recommendation := issue.Recommendation
		if len(recommendation) > 50 {
			recommendation = recommendation[:47] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sessionID, issueType, workspace, message, recommendation)
	}
}

// displayRecommendationsSummary shows high-level recommendations
func displayRecommendationsSummary(issues []*audit.AuditIssue) {
	// Count issue types
	orphanCount := 0
	corruptedCount := 0
	missingTmuxCount := 0
	staleCount := 0
	duplicateCount := 0
	chezmoiCount := 0

	for _, issue := range issues {
		switch issue.Type {
		case audit.IssueOrphanedConversation:
			orphanCount++
		case audit.IssueCorruptedManifest:
			corruptedCount++
		case audit.IssueMissingTmuxSession:
			missingTmuxCount++
		case audit.IssueStaleSession:
			staleCount++
		case audit.IssueDuplicateUUID:
			duplicateCount++
		case audit.IssueChezmoiDrift:
			chezmoiCount++
		}
	}

	// Display recommendations based on issue types
	if corruptedCount > 0 {
		fmt.Printf("  • Fix %d corrupted manifest(s) by restoring from backup or regenerating\n", corruptedCount)
	}
	if duplicateCount > 0 {
		fmt.Printf("  • Resolve %d duplicate UUID(s) using 'agm admin fix-uuid'\n", duplicateCount)
	}
	if orphanCount > 0 {
		fmt.Printf("  • Import %d orphaned conversation(s) using 'agm session import'\n", orphanCount)
	}
	if missingTmuxCount > 0 {
		fmt.Printf("  • Resume or archive %d session(s) with missing tmux sessions\n", missingTmuxCount)
	}
	if staleCount > 0 {
		fmt.Printf("  • Archive %d stale session(s) to clean up inactive sessions\n", staleCount)
	}
	if chezmoiCount > 0 {
		fmt.Printf("  • Reconcile %d drifted dotfile(s) with `chezmoi re-add` (or edit the source for templates)\n", chezmoiCount)
	}
}
