package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/agm/internal/verify"
)

var (
	batchVerifySessions string
	batchVerifyRepoDir  string
	batchVerifyAll      bool
)

// BatchVerifyResult holds the verification outcome for a single session.
type BatchVerifyResult struct {
	SessionName string         `json:"session_name"`
	SessionID   string         `json:"session_id"`
	Purpose     string         `json:"purpose"`
	Status      string         `json:"status"` // VERIFIED or NEEDS_REMEDIATION or SKIPPED
	PassCount   int            `json:"pass_count"`
	FailCount   int            `json:"fail_count"`
	Details     []ResultDetail `json:"details,omitempty"`
}

// ResultDetail is a single assertion result for JSON output.
type ResultDetail struct {
	Description string `json:"description"`
	Pass        bool   `json:"pass"`
	Evidence    string `json:"evidence"`
}

// BatchVerifyReport is the aggregate report for all sessions.
type BatchVerifyReport struct {
	Timestamp string              `json:"timestamp"`
	RepoDir   string              `json:"repo_dir"`
	Results   []BatchVerifyResult `json:"results"`
	Summary   BatchVerifySummary  `json:"summary"`
}

// BatchVerifySummary provides counts across all sessions.
type BatchVerifySummary struct {
	Total            int `json:"total"`
	Verified         int `json:"verified"`
	NeedsRemediation int `json:"needs_remediation"`
	Skipped          int `json:"skipped"`
}

var batchVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify session completions against original prompts",
	Long: `Verify that completed sessions actually accomplished their stated purpose.

For each session, reads the original prompt/purpose from the session manifest,
extracts verifiable assertions, and checks them against the repository state.

Reports are saved to ~/.agm/batch-reports/.

Examples:
  agm batch verify --sessions "session1,session2"
  agm batch verify --sessions "session1,session2" --repo-dir /path/to/repo
  agm batch verify --all`,
	RunE: runBatchVerify,
}

func init() {
	batchVerifyCmd.Flags().StringVar(&batchVerifySessions, "sessions", "", "Comma-separated list of session names to verify")
	batchVerifyCmd.Flags().StringVar(&batchVerifyRepoDir, "repo-dir", "", "Repository directory to verify against (default: session's working directory)")
	batchVerifyCmd.Flags().BoolVar(&batchVerifyAll, "all", false, "Verify all archived sessions")
	batchCmd.AddCommand(batchVerifyCmd)
}

func runBatchVerify(cmd *cobra.Command, args []string) error {
	if batchVerifySessions == "" && !batchVerifyAll {
		return fmt.Errorf("specify --sessions or --all")
	}

	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	var manifests []*manifest.Manifest

	if batchVerifyAll {
		manifests, err = loadAllArchivedSessions(adapter)
		if err != nil {
			return err
		}
	} else {
		names := strings.Split(batchVerifySessions, ",")
		manifests, err = loadNamedSessions(adapter, names)
		if err != nil {
			return err
		}
	}

	if len(manifests) == 0 {
		fmt.Println("No sessions found to verify.")
		return nil
	}

	report := verifyBatch(manifests)

	// Save report
	reportPath, err := saveReport(report)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to save report: %v", err))
	} else {
		fmt.Printf("\nReport saved: %s\n", reportPath)
	}

	// Print summary
	printVerifySummary(report)

	if report.Summary.NeedsRemediation > 0 {
		return fmt.Errorf("%d session(s) need remediation", report.Summary.NeedsRemediation)
	}
	return nil
}

func loadAllArchivedSessions(adapter *dolt.Adapter) ([]*manifest.Manifest, error) {
	all, err := adapter.ListSessions(&dolt.SessionFilter{
		Lifecycle: manifest.LifecycleArchived,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list archived sessions: %w", err)
	}
	// Filter to only those with a purpose
	var filtered []*manifest.Manifest
	for _, m := range all {
		if m.Context.Purpose != "" {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

func loadNamedSessions(adapter *dolt.Adapter, names []string) ([]*manifest.Manifest, error) {
	var manifests []*manifest.Manifest
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		m, err := adapter.GetSessionByName(name)
		if err != nil {
			// Try by session ID
			m, err = adapter.GetSession(name)
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("Session not found: %s", name))
				continue
			}
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

func verifyBatch(manifests []*manifest.Manifest) *BatchVerifyReport {
	report := &BatchVerifyReport{
		Timestamp: time.Now().Format(time.RFC3339),
		RepoDir:   batchVerifyRepoDir,
	}

	for i, m := range manifests {
		fmt.Printf("[%d/%d] Verifying: %s\n", i+1, len(manifests), sessionDisplayName(m))

		result := verifySingleSession(m)
		report.Results = append(report.Results, result)

		switch result.Status {
		case "VERIFIED":
			report.Summary.Verified++
		case "NEEDS_REMEDIATION":
			report.Summary.NeedsRemediation++
		case "SKIPPED":
			report.Summary.Skipped++
		}
		report.Summary.Total++
	}

	return report
}

func verifySingleSession(m *manifest.Manifest) BatchVerifyResult {
	displayName := sessionDisplayName(m)

	result := BatchVerifyResult{
		SessionName: displayName,
		SessionID:   m.SessionID,
		Purpose:     m.Context.Purpose,
	}

	if m.Context.Purpose == "" {
		result.Status = "SKIPPED"
		return result
	}

	// Determine repo directory
	repoDir := batchVerifyRepoDir
	if repoDir == "" {
		repoDir = m.Context.Project
	}
	if repoDir == "" || !dirExists(repoDir) {
		result.Status = "SKIPPED"
		return result
	}

	// Extract assertions from purpose
	assertions := verify.ExtractAssertions(m.Context.Purpose)
	if len(assertions) == 0 {
		result.Status = "SKIPPED"
		return result
	}

	// Run verification
	vReport := verify.Verify(m.SessionID, m.Context.Purpose, repoDir, assertions)

	result.PassCount = vReport.PassCount()
	result.FailCount = vReport.FailCount()

	for _, r := range vReport.Results {
		result.Details = append(result.Details, ResultDetail{
			Description: r.Assertion.Description,
			Pass:        r.Pass,
			Evidence:    r.Evidence,
		})
	}

	if vReport.Passed() {
		result.Status = "VERIFIED"
	} else {
		result.Status = "NEEDS_REMEDIATION"
	}

	return result
}

func sessionDisplayName(m *manifest.Manifest) string {
	if m.Name != "" {
		return m.Name
	}
	if m.Tmux.SessionName != "" {
		return m.Tmux.SessionName
	}
	return m.SessionID
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func saveReport(report *BatchVerifyReport) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	reportsDir := filepath.Join(homeDir, ".agm", "batch-reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create reports directory: %w", err)
	}

	filename := fmt.Sprintf("verify-%s.json", time.Now().Format("20060102-150405"))
	reportPath := filepath.Join(reportsDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		return "", err
	}

	return reportPath, nil
}

func printVerifySummary(report *BatchVerifyReport) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Batch Verification Summary (%d sessions)\n", report.Summary.Total)
	fmt.Println(strings.Repeat("=", 60))

	for _, r := range report.Results {
		icon := "PASS"
		if r.Status == "NEEDS_REMEDIATION" {
			icon = "FAIL"
		} else if r.Status == "SKIPPED" {
			icon = "SKIP"
		}
		fmt.Printf("  [%s] %s", icon, r.SessionName)
		if r.Status != "SKIPPED" {
			fmt.Printf(" (%d/%d assertions passed)", r.PassCount, r.PassCount+r.FailCount)
		}
		fmt.Println()

		// Show failed assertions
		if r.Status == "NEEDS_REMEDIATION" {
			for _, d := range r.Details {
				if !d.Pass {
					fmt.Printf("         - %s\n", d.Description)
				}
			}
		}
	}

	fmt.Println()
	if report.Summary.Verified > 0 {
		ui.PrintSuccess(fmt.Sprintf("%d session(s) verified", report.Summary.Verified))
	}
	if report.Summary.NeedsRemediation > 0 {
		ui.PrintWarning(fmt.Sprintf("%d session(s) need remediation", report.Summary.NeedsRemediation))
	}
	if report.Summary.Skipped > 0 {
		fmt.Printf("  %d session(s) skipped (no purpose or assertions)\n", report.Summary.Skipped)
	}
}
