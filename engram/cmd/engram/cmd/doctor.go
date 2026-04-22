package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/health"
)

var (
	autoFixFlag bool
	quietFlag   bool
	jsonFlag    bool
	noCacheFlag bool
	noColorFlag bool
	hooksFlag   bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run Engram health checks",
	Long: `Run comprehensive health checks for Engram infrastructure.

The doctor command checks:
- Core infrastructure (workspace, config, logs)
- Dependencies (yq, jq, python3)
- Hooks (configuration and scripts)
- File permissions
- Plugin health (if plugins implement health-check.sh)

Exit codes:
  0 - All checks passed (healthy)
  1 - Warnings present (degraded)
  2 - Errors present (critical)

Examples:
  # Run health checks
  engram doctor

  # Apply safe auto-fixes
  engram doctor --auto-fix

  # Show only issues (silent if healthy)
  engram doctor --quiet

  # Output JSON for automation
  engram doctor --json

  # Check only hooks
  engram doctor --hooks`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	// Flags (from D4 FR4)
	doctorCmd.Flags().BoolVar(&autoFixFlag, "auto-fix", false, "Apply safe auto-fixes (Tier 1 operations)")
	doctorCmd.Flags().BoolVar(&quietFlag, "quiet", false, "Only show issues, silent if healthy")
	doctorCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output JSON for automation")
	doctorCmd.Flags().BoolVar(&noCacheFlag, "no-cache", false, "Force fresh checks, bypass cache read")
	doctorCmd.Flags().BoolVar(&noColorFlag, "no-color", false, "Disable color output")
	doctorCmd.Flags().BoolVar(&hooksFlag, "hooks", false, "Run only hook-related checks")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	if noColorFlag {
		cli.DisableColor()
	}

	ctx := context.Background()
	startTime := time.Now()
	checker := health.NewHealthChecker(ctx)

	showHeader()
	progress := setupProgressIndicator()

	results, err := runHealthChecks(checker, progress)
	if err != nil {
		return err
	}

	duration := time.Since(startTime)
	formatter := health.NewFormatter(results, duration)

	writeCacheIfNeeded(results, formatter)

	fixesApplied, updatedResults := handleAutoFix(checker, results)
	if updatedResults != nil {
		results = updatedResults
		formatter = health.NewFormatter(results, duration)
	}

	displayResults(formatter)
	logResults(formatter, duration, fixesApplied, len(results))

	os.Exit(formatter.GetExitCode())
	return nil
}

func showHeader() {
	if !quietFlag && !jsonFlag {
		fmt.Println("Engram Doctor - Health Check Report")
		fmt.Println()
	}
}

func setupProgressIndicator() *cli.ProgressIndicator {
	if quietFlag || jsonFlag {
		return nil
	}

	msg := "Running health checks..."
	if noCacheFlag {
		msg = "Running health checks (bypassing cache)..."
	}
	progress := cli.NewProgress(msg)
	progress.Start()
	return progress
}

func runHealthChecks(checker *health.HealthChecker, progress *cli.ProgressIndicator) ([]health.CheckResult, error) {
	if progress != nil {
		defer progress.Stop()
	}

	var results []health.CheckResult
	var err error
	if hooksFlag {
		results, err = checker.RunHookChecks()
	} else {
		results, err = checker.RunAllChecks()
	}
	if err != nil {
		if progress != nil {
			progress.Fail("Health checks failed")
		}
		return nil, fmt.Errorf("health checks failed: %w", err)
	}

	if progress != nil {
		progress.Stop()
		fmt.Println()
	}

	return results, nil
}

func writeCacheIfNeeded(results []health.CheckResult, formatter *health.Formatter) {
	if noCacheFlag {
		return
	}

	cache := health.BuildCacheFromResults(results, formatter.GetSummary())
	if err := health.WriteCache(cache); err != nil {
		cli.PrintWarning(fmt.Sprintf("Failed to write cache: %v", err))
	}
}

func handleAutoFix(checker *health.HealthChecker, results []health.CheckResult) (int, []health.CheckResult) {
	if !autoFixFlag {
		return 0, nil
	}

	fixer := health.NewTier1Fixer(checker.Workspace())
	fixes := fixer.PreviewFixes(results)

	if len(fixes) == 0 {
		cli.PrintInfo("No fixable issues found")
		os.Exit(0)
	}

	showFixPreview(fixes)

	if !confirmFixes() {
		cli.PrintInfo("Auto-fix cancelled")
		os.Exit(0)
	}

	return applyFixes(fixer, fixes, results, checker)
}

func showFixPreview(fixes []health.FixOperation) {
	cli.PrintInfo("Preview of fixes to apply:")
	for _, fix := range fixes {
		fmt.Printf("  %s %s\n", cli.SuccessIcon(), fix.Description)
	}
	fmt.Println()
}

func confirmFixes() bool {
	fmt.Print("Apply these fixes? (Y/n): ")
	var response string
	fmt.Scanln(&response)
	return response == "" || response == "Y" || response == "y"
}

func applyFixes(fixer *health.Tier1Fixer, fixes []health.FixOperation, results []health.CheckResult, checker *health.HealthChecker) (int, []health.CheckResult) {
	fmt.Println("\nApplying fixes...")
	applied, err := fixer.ApplyFixes(fixes, results)
	if err != nil {
		cli.PrintWarning(fmt.Sprintf("Some fixes failed: %v", err))
	}

	if applied > 0 {
		cli.PrintSuccess(fmt.Sprintf("Applied %d fixes successfully!", applied))
		fmt.Println()
		updatedResults, _ := checker.RunAllChecks()
		return applied, updatedResults
	}

	return applied, nil
}

func displayResults(formatter *health.Formatter) {
	if jsonFlag {
		output, err := formatter.FormatJSON()
		if err != nil {
			cli.PrintWarning(fmt.Sprintf("format JSON: %v", err))
			return
		}
		fmt.Println(output)
		return
	}

	if quietFlag {
		output := formatter.FormatQuiet()
		if output != "" {
			fmt.Print(output)
		}
		return
	}

	fmt.Print(formatter.FormatDefault())
}

func logResults(formatter *health.Formatter, duration time.Duration, fixesApplied, checksRun int) {
	if noCacheFlag && checksRun == 0 {
		return
	}

	logger := health.NewLogger()
	err := logger.AppendToLog(&health.HealthCheckLog{
		Timestamp:      time.Now().Format(time.RFC3339),
		SessionID:      generateSessionID(),
		Trigger:        "manual",
		DurationMS:     duration.Milliseconds(),
		ChecksRun:      checksRun,
		PluginsChecked: 0,
		Summary:        formatter.GetSummary(),
		Issues:         formatter.GetIssues(),
		FixesApplied:   fixesApplied,
		CacheUsed:      false,
		Version:        "0.1.0",
	})

	if err != nil {
		cli.PrintWarning(fmt.Sprintf("Failed to write JSONL log: %v", err))
	}
}

// generateSessionID creates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
