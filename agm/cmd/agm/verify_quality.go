package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	verifyQualityGate       bool
	verifyQualityConfigPath string
)

func init() {
	verifyWorkerCmd.Flags().BoolVar(&verifyQualityGate, "quality-gate", false, "Run holdout quality gates instead of commit verification")
	verifyWorkerCmd.Flags().StringVar(&verifyQualityConfigPath, "quality-config", "", "Path to quality-gates.yaml (default: .agm/quality-gates.yaml in repo)")
}

// runVerifyQualityGate runs quality gates against a session's branch.
func runVerifyQualityGate(sessionName string) error {
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	m, err := adapter.GetSessionByName(sessionName)
	if err != nil {
		m, err = adapter.GetSession(sessionName)
		if err != nil {
			ui.PrintError(err, "Failed to find session",
				"  * Session may not exist: "+sessionName+"\n"+
					"  * Try: agm session list")
			return err
		}
	}

	displayName := sessionDisplayNameVerify(m)

	// Resolve repo directory
	repoDir, err := resolveRepoDir(verifyRepoDir2, m.Context.Project, m.WorkingDirectory, displayName)
	if err != nil {
		return err
	}

	// Resolve branch
	branch, err := resolveAndValidateBranch(verifyBranch, m.SessionID, m.Name, repoDir)
	if err != nil {
		if verifyRecordTrust {
			recordTrustEvent(displayName, "quality_gate_failure", "branch not found")
		}
		return err
	}

	// Find quality gates config
	configPath := verifyQualityConfigPath
	if configPath == "" {
		configPath = filepath.Join(repoDir, ".agm", "quality-gates.yaml")
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("quality gates config not found at %s; use --quality-config to specify", configPath)
	}

	opCtx := newOpContext()
	result, err := ops.RunQualityGates(opCtx, &ops.RunQualityGatesRequest{
		SessionName: displayName,
		ConfigPath:  configPath,
		RepoDir:     repoDir,
		Branch:      branch,
		RecordTrust: verifyRecordTrust,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printQualityGateReport(result, displayName, branch, repoDir, configPath)
	})
}

// printQualityGateReport prints the human-readable quality-gate report
// (header, per-gate PASS/FAIL lines, summary).
func printQualityGateReport(result *ops.RunQualityGatesResult, displayName, branch, repoDir, configPath string) {
	fmt.Fprintf(os.Stderr, "Session:  %s\n", displayName)
	fmt.Fprintf(os.Stderr, "Branch:   %s\n", branch)
	fmt.Fprintf(os.Stderr, "Repo:     %s\n", repoDir)
	fmt.Fprintf(os.Stderr, "Config:   %s\n", configPath)
	fmt.Fprintln(os.Stderr)

	for _, g := range result.Gates {
		status := "PASS"
		if !g.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "  [%s] %s (%dms)\n", status, g.Name, g.DurationMs)
		if !g.Passed && g.Output != "" {
			out := g.Output
			if len(out) > 200 {
				out = out[:200] + "..."
			}
			fmt.Fprintf(os.Stderr, "         %s\n", out)
		}
	}

	fmt.Fprintln(os.Stderr)
	if result.Passed {
		ui.PrintSuccess(fmt.Sprintf("All %d quality gates passed", result.TotalGates))
	} else {
		ui.PrintWarning(fmt.Sprintf("%d/%d quality gates failed", result.FailedCount, result.TotalGates))
	}
}
