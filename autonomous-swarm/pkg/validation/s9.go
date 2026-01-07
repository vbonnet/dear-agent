package validation

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// S9ValidationResult represents the result of S9 test execution
type S9ValidationResult struct {
	Valid             bool
	TestsPassed       bool
	CoverageMet       bool
	Coverage          float64
	CoverageThreshold float64
	Failures          []string
	TestOutput        string
}

// ValidateS9 executes tests and validates coverage for S9 validation phase
func ValidateS9(packagePath string, coverageThreshold float64) (*S9ValidationResult, error) {
	result := &S9ValidationResult{
		Valid:             true,
		CoverageThreshold: coverageThreshold,
		Failures:          []string{},
	}

	// Execute go test with coverage
	cmd := exec.Command("go", "test", "-cover", packagePath)
	output, err := cmd.CombinedOutput()
	result.TestOutput = string(output)

	// Parse test results
	if err != nil {
		result.Valid = false
		result.TestsPassed = false
		result.Failures = parseTestFailures(string(output))
		return result, nil
	}

	result.TestsPassed = true

	// Parse coverage percentage
	coverage, err := parseCoverage(string(output))
	if err != nil {
		result.Valid = false
		result.Failures = append(result.Failures, fmt.Sprintf("failed to parse coverage: %v", err))
		return result, nil
	}

	result.Coverage = coverage
	result.CoverageMet = coverage >= coverageThreshold

	if !result.CoverageMet {
		result.Valid = false
		result.Failures = append(result.Failures,
			fmt.Sprintf("coverage %.1f%% below threshold %.1f%%", coverage, coverageThreshold))
	}

	return result, nil
}

// parseCoverage extracts coverage percentage from go test output
func parseCoverage(output string) (float64, error) {
	// Example: "coverage: 85.3% of statements"
	re := regexp.MustCompile(`coverage:\s+([\d.]+)%`)
	matches := re.FindStringSubmatch(output)

	if len(matches) < 2 {
		return 0, fmt.Errorf("coverage not found in output")
	}

	coverage, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse coverage value: %w", err)
	}

	return coverage, nil
}

// parseTestFailures extracts test failure messages from go test output
func parseTestFailures(output string) []string {
	failures := []string{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Capture --- FAIL: test lines (individual test failures)
		if strings.Contains(line, "--- FAIL:") {
			failures = append(failures, strings.TrimSpace(line))
		}

		// Capture panic lines
		if strings.Contains(line, "panic:") {
			failures = append(failures, strings.TrimSpace(line))
		}

		// Capture Error: lines within test output
		if strings.Contains(line, "Error:") && !strings.HasPrefix(line, "FAIL") {
			failures = append(failures, strings.TrimSpace(line))
		}
	}

	// Add generic failure message if we found FAIL but no specific messages
	if len(failures) == 0 && strings.Contains(output, "FAIL") {
		failures = append(failures, "tests failed (see output for details)")
	}

	return failures
}
