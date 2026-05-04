package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var (
	scanInterval   time.Duration
	scanOnce       bool
	scanForceLoop  bool
	scanCrossCheck bool
)

// ScanCycleResult holds the aggregated results of one scan cycle
type ScanCycleResult struct {
	Timestamp      time.Time                     `json:"timestamp"`
	Sessions       *ops.ListSessionsResult       `json:"sessions,omitempty"`
	Metrics        *ops.MetricsResult            `json:"metrics,omitempty"`
	MetricsAlerts  []ops.Alert                   `json:"metrics_alerts,omitempty"`
	WorkerBranches map[string][]WorkerCommit     `json:"worker_branches,omitempty"`
	CrossCheck     *ops.CrossCheckReport         `json:"cross_check,omitempty"`
	Findings       ScanFindings                  `json:"findings"`
	Errors         []string                      `json:"errors,omitempty"`
}

// WorkerCommit represents a recent commit on a worker branch
type WorkerCommit struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// ScanFindings aggregates key findings from the scan
type ScanFindings struct {
	SessionsNeedingApproval []string `json:"sessions_needing_approval"`
	NewCommitsDetected      int      `json:"new_commits_detected"`
	MetricsAlertCount       int      `json:"metrics_alert_count"`
	HealthStatus            string   `json:"health_status"` // "healthy", "warning", "critical"
	CrossCheckIssues        int      `json:"cross_check_issues,omitempty"`
	UnmanagedSessions       int      `json:"unmanaged_sessions,omitempty"`
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Run an orchestrator scan cycle",
	Long: `Run one orchestrator scan cycle that:
  1. Lists sessions and identifies those in PERMISSION_PROMPT state
  2. Checks worker branches for new commits
  3. Runs metrics health check
  4. Outputs a structured JSON scan report

Use --cross-check to verify session state via tmux capture-pane.
Use --loop to continuously scan with configurable interval.
Use --once for a single scan (default).`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().DurationVar(&scanInterval, "interval", contracts.Load().ScanLoop.DefaultScanInterval.Duration, "scan interval in loop mode (e.g., 5m, 30s)")
	scanCmd.Flags().BoolVar(&scanOnce, "once", true, "run single scan (default; use --loop to disable)")
	scanCmd.Flags().BoolVar(&scanForceLoop, "loop", false, "run continuous scan loop")
	scanCmd.Flags().BoolVar(&scanCrossCheck, "cross-check", false, "verify session state via tmux capture-pane (detect STUCK, DOWN, ENTER bugs)")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	// If --loop is set, ignore --once default and run in loop mode
	loopMode := scanForceLoop

	if loopMode {
		return runScanLoop()
	}
	return runSingleScan()
}

// runSingleScan executes one scan cycle
func runSingleScan() error {
	result, err := performScanCycle()
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printScanText(result)
	})
}

// runScanLoop continuously runs scan cycles
func runScanLoop() error {
	for {
		result, err := performScanCycle()
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
		} else {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		}

		fmt.Fprintf(os.Stderr, "next scan in %v...\n", scanInterval)
		time.Sleep(scanInterval)
	}
}

// performScanCycle executes the core scan logic
func performScanCycle() (*ScanCycleResult, error) {
	result := &ScanCycleResult{
		Timestamp:      time.Now(),
		WorkerBranches: make(map[string][]WorkerCommit),
		Findings: ScanFindings{
			SessionsNeedingApproval: []string{},
		},
	}

	// Create OpContext for database operations
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create op context: %v", err))
		return result, nil // Return partial result
	}
	defer cleanup()

	// 1. List sessions (State-based PERMISSION_PROMPT detection removed —
	// it produced false positives. Use cross_check for ground truth.)
	sessionResult, err := ops.ListSessions(opCtx, &ops.ListSessionsRequest{
		Status: "active",
		Limit:  1000,
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to list sessions: %v", err))
	} else {
		result.Sessions = sessionResult
	}

	// 2. Check worker branches for new commits
	workerCommits := scanWorkerBranches()
	result.WorkerBranches = workerCommits
	for _, commits := range workerCommits {
		result.Findings.NewCommitsDetected += len(commits)
	}

	// 3. Run metrics health check
	metricsResult, err := ops.GetMetrics(opCtx, &ops.MetricsRequest{
		Window: contracts.Load().ScanLoop.MetricsWindow.Duration,
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get metrics: %v", err))
	} else {
		result.Metrics = metricsResult
		result.Findings.MetricsAlertCount = len(metricsResult.Alerts)
		result.MetricsAlerts = metricsResult.Alerts
	}

	// 4. Cross-check via tmux capture-pane (only when --cross-check is set)
	if scanCrossCheck {
		crossCheckCfg := ops.DefaultCrossCheckConfig()
		crossCheckCfg.CallerSession = os.Getenv("AGM_SESSION_NAME")
		crossCheckReport, err := ops.RunCrossCheck(opCtx, crossCheckCfg)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cross-check failed: %v", err))
		} else {
			result.CrossCheck = crossCheckReport
			// Count issues (non-HEALTHY states)
			for _, r := range crossCheckReport.Results {
				if r.State != ops.StateHealthy {
					result.Findings.CrossCheckIssues++
				}
			}
			result.Findings.UnmanagedSessions = len(crossCheckReport.UnmanagedSessions)
		}
	}

	// Determine overall health status
	result.Findings.HealthStatus = "healthy"
	if result.Findings.MetricsAlertCount > 0 {
		// Check alert levels
		for _, alert := range result.MetricsAlerts {
			if alert.Level == "critical" {
				result.Findings.HealthStatus = "critical"
				break
			}
		}
		if result.Findings.HealthStatus == "healthy" {
			result.Findings.HealthStatus = "warning"
		}
	}
	// Cross-check issues can also elevate health status
	if result.Findings.CrossCheckIssues > 0 && result.Findings.HealthStatus == "healthy" {
		result.Findings.HealthStatus = "warning"
	}

	return result, nil
}

// scanWorkerBranches checks for new commits on worker branches
func scanWorkerBranches() map[string][]WorkerCommit {
	branches := make(map[string][]WorkerCommit)

	// Get list of worker branches (those prefixed with impl- or agm/)
	out, err := exec.Command("git", "branch", "-a").Output()
	if err != nil {
		return branches
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	workerBranches := []string{}

	for _, line := range lines {
		branch := strings.TrimSpace(line)
		branch = strings.TrimPrefix(branch, "* ") // Remove current branch marker
		// Only track local worker branches (impl-* pattern)
		if strings.HasPrefix(branch, "impl-") || strings.HasPrefix(branch, "agm/") {
			workerBranches = append(workerBranches, branch)
		}
	}

	// For each worker branch, get recent commits
	since := time.Now().Add(-contracts.Load().ScanLoop.WorkerCommitLookback.Duration).Format("2006-01-02")
	for _, branch := range workerBranches {
		commits := getRecentCommits(branch, since)
		if len(commits) > 0 {
			branches[branch] = commits
		}
	}

	return branches
}

// getRecentCommits retrieves recent commits from a branch
func getRecentCommits(branch string, since string) []WorkerCommit {
	var commits []WorkerCommit

	// Get commits from this branch since date
	out, err := exec.Command(
		"git", "log",
		branch,
		"--since="+since,
		"--format=%H|%an|%s|%ai",
	).Output()
	if err != nil {
		return commits
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		// Parse timestamp
		timeStr := strings.TrimSpace(parts[3])
		t, err := time.Parse("2006-01-02 15:04:05 -0700", timeStr)
		if err != nil {
			t = time.Now()
		}

		commits = append(commits, WorkerCommit{
			Hash:    parts[0][:8],
			Author:  parts[1],
			Message: parts[2],
			Time:    t,
		})
	}

	return commits
}

// printScanText renders scan results in human-readable format
func printScanText(r *ScanCycleResult) {
	fmt.Printf("\n=== AGM Orchestrator Scan — %s ===\n\n", r.Timestamp.Format("2006-01-02 15:04:05"))
	printScanApprovals(r.Findings.SessionsNeedingApproval)
	printScanSessions(r.Sessions)
	printScanWorkerCommits(r.Findings.NewCommitsDetected, r.WorkerBranches)
	printScanCrossCheck(r.CrossCheck)
	printScanMetrics(r.Findings.HealthStatus, r.Findings.MetricsAlertCount, r.MetricsAlerts)
	printScanErrors(r.Errors)
	fmt.Println()
}

func printScanApprovals(needApproval []string) {
	fmt.Printf("Sessions Needing Approval: %d\n", len(needApproval))
	if len(needApproval) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, name := range needApproval {
		fmt.Printf("  • %s (PERMISSION_PROMPT)\n", name)
	}
}

func printScanSessions(s *ops.ListSessionsResult) {
	if s == nil {
		return
	}
	fmt.Printf("\nSessions: %d total", s.Total)
	if s.Total > 0 {
		active := 0
		for _, sess := range s.Sessions {
			if sess.Status == "active" {
				active++
			}
		}
		fmt.Printf(" (%d active)", active)
	}
	fmt.Println()
}

func printScanWorkerCommits(newCommits int, branches map[string][]WorkerCommit) {
	fmt.Printf("\nWorker Commits (24h): %d total\n", newCommits)
	if len(branches) == 0 {
		return
	}
	for branch, commits := range branches {
		fmt.Printf("  %s: %d commits\n", branch, len(commits))
		for i, c := range commits {
			if i >= 2 {
				break
			}
			fmt.Printf("    • %s %s (%s)\n", c.Hash, c.Message, c.Author)
		}
	}
}

func printScanCrossCheck(cc *ops.CrossCheckReport) {
	if cc == nil {
		return
	}
	fmt.Printf("\nCross-Check Results:\n")
	for _, cr := range cc.Results {
		icon := "✓"
		if cr.State != ops.StateHealthy {
			icon = "✗"
		}
		fmt.Printf("  %s %s: %s", icon, cr.SessionName, cr.StateStr)
		if cr.Action != "" {
			fmt.Printf(" → %s", cr.Action)
		}
		fmt.Println()
	}
	if len(cc.UnmanagedSessions) > 0 {
		fmt.Printf("\n  Unmanaged Sessions: %s\n", strings.Join(cc.UnmanagedSessions, ", "))
	}
}

func printScanMetrics(healthStatus string, alertCount int, alerts []ops.Alert) {
	fmt.Printf("\nHealth Status: %s\n", strings.ToUpper(healthStatus))
	fmt.Printf("Metrics Alerts: %d\n", alertCount)
	for _, alert := range alerts {
		icon := "!"
		if alert.Level == "critical" {
			icon = "X"
		}
		fmt.Printf("  [%s] %s: %s\n", icon, strings.ToUpper(alert.Type), alert.Message)
	}
}

func printScanErrors(errs []string) {
	if len(errs) == 0 {
		return
	}
	fmt.Printf("\nErrors: %d\n", len(errs))
	for _, e := range errs {
		fmt.Printf("  ! %s\n", e)
	}
}
