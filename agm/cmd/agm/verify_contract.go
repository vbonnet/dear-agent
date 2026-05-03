package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	verifyContractDrift bool
	verifySpecsDir      string
	verifyContractsFile string
)

func init() {
	verifyWorkerCmd.Flags().BoolVar(&verifyContractDrift, "contract-drift", false, "Detect drift between SPECs, SLO contracts, and source code")
	verifyWorkerCmd.Flags().StringVar(&verifySpecsDir, "specs-dir", "", "Path to SPEC files directory (default: agm/docs/specs/ relative to binary)")
	verifyWorkerCmd.Flags().StringVar(&verifyContractsFile, "contracts-file", "", "Path to slo-contracts.yaml (default: embedded contracts)")
}

// runVerifyContractDrift runs contract drift detection.
func runVerifyContractDrift() error {
	specsDir := verifySpecsDir
	if specsDir == "" {
		specsDir = resolveDefaultSpecsDir()
	}

	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		return fmt.Errorf("specs directory not found at %s; use --specs-dir to specify", specsDir)
	}

	result, err := ops.ContractDrift(nil, &ops.ContractDriftRequest{
		SpecsDir:      specsDir,
		ContractsFile: verifyContractsFile,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		fmt.Fprintf(os.Stderr, "Specs dir:  %s\n", specsDir)
		if verifyContractsFile != "" {
			fmt.Fprintf(os.Stderr, "Contracts:  %s\n", verifyContractsFile)
		} else {
			fmt.Fprintf(os.Stderr, "Contracts:  embedded defaults\n")
		}
		fmt.Fprintf(os.Stderr, "SPECs:      %d\n", result.TotalSpecs)
		fmt.Fprintln(os.Stderr)

		// Group findings by SPEC file
		bySpec := make(map[string][]ops.DriftFinding)
		var specOrder []string
		for _, f := range result.Findings {
			if _, seen := bySpec[f.SPECFile]; !seen {
				specOrder = append(specOrder, f.SPECFile)
			}
			bySpec[f.SPECFile] = append(bySpec[f.SPECFile], f)
		}

		for _, spec := range specOrder {
			findings := bySpec[spec]
			fmt.Fprintf(os.Stderr, "  %s\n", spec)
			for _, f := range findings {
				icon := "PASS"
				//nolint:exhaustive // intentional partial: handles the relevant subset
				switch f.Severity {
				case ops.DriftWarn:
					icon = "WARN"
				case ops.DriftFail:
					icon = "FAIL"
				}
				fmt.Fprintf(os.Stderr, "    [%s] %s: %s", icon, f.Section, f.Metric)
				if f.Severity == ops.DriftFail && f.Expected != "" {
					fmt.Fprintf(os.Stderr, " (spec=%s, contract=%s)", f.Expected, f.Actual)
				}
				fmt.Fprintln(os.Stderr)
			}
			fmt.Fprintln(os.Stderr)
		}

		// Summary
		fmt.Fprintln(os.Stderr, strings.Repeat("=", 60))
		switch result.OverallStatus {
		case ops.DriftPass:
			ui.PrintSuccess(fmt.Sprintf("No drift detected (%d checks passed)", result.PassCount))
		case ops.DriftWarn:
			ui.PrintWarning(fmt.Sprintf("%d warning(s), %d passed", result.WarnCount, result.PassCount))
		case ops.DriftFail:
			ui.PrintWarning(fmt.Sprintf("%d FAIL, %d warnings, %d passed", result.FailCount, result.WarnCount, result.PassCount))
		}
	})
}

// resolveDefaultSpecsDir finds the specs directory relative to the binary location
// or falls back to common locations.
func resolveDefaultSpecsDir() string {
	// Try relative to the binary
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		// binary is in agm/cmd/agm, specs are in agm/docs/specs
		root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
		candidate := filepath.Join(root, "docs", "specs")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Try relative to CWD
	for _, candidate := range []string{
		"agm/docs/specs",
		"docs/specs",
	} {
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
	}

	// Fallback: look for the repo via GOPATH or common locations
	home, _ := os.UserHomeDir()
	for _, candidate := range []string{
		filepath.Join(home, "src/ws/oss/repos/ai-tools/agm/docs/specs"),
		filepath.Join(home, "go/src/github.com/vbonnet/dear-agent/agm/docs/specs"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return "agm/docs/specs"
}
