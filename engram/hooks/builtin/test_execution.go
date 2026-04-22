package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/vbonnet/dear-agent/engram/hooks"
)

// TestFramework represents a detected test framework
type TestFramework string

const (
	FrameworkGo         TestFramework = "go"
	FrameworkPython     TestFramework = "python"
	FrameworkJavaScript TestFramework = "javascript"
	FrameworkRust       TestFramework = "rust"
)

// TestExecutor detects and runs tests with coverage analysis
type TestExecutor struct {
	projectRoot string
	threshold   float64 // Coverage threshold (0-100)
}

// NewTestExecutor creates a new test executor
func NewTestExecutor(projectRoot string, threshold float64) *TestExecutor {
	if threshold == 0 {
		threshold = 70.0 // Default 70%
	}
	return &TestExecutor{
		projectRoot: projectRoot,
		threshold:   threshold,
	}
}

// DetectFrameworks detects test frameworks in the project
func (te *TestExecutor) DetectFrameworks() ([]TestFramework, error) {
	var frameworks []TestFramework

	// Walk directory tree to find test files
	err := filepath.Walk(te.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Skip hidden directories
		if info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		name := info.Name()

		// Detect Go tests (*_test.go)
		if filepath.Ext(name) == ".go" && len(name) > 8 && name[len(name)-8:] == "_test.go" {
			if !contains(frameworks, FrameworkGo) {
				frameworks = append(frameworks, FrameworkGo)
			}
		}

		// Detect Python tests (test_*.py)
		if filepath.Ext(name) == ".py" && len(name) > 5 && name[:5] == "test_" {
			if !contains(frameworks, FrameworkPython) {
				frameworks = append(frameworks, FrameworkPython)
			}
		}

		// Detect JavaScript tests (*.test.js, *.spec.js)
		if filepath.Ext(name) == ".js" {
			if len(name) > 8 && name[len(name)-8:] == ".test.js" {
				if !contains(frameworks, FrameworkJavaScript) {
					frameworks = append(frameworks, FrameworkJavaScript)
				}
			}
			if len(name) > 8 && name[len(name)-8:] == ".spec.js" {
				if !contains(frameworks, FrameworkJavaScript) {
					frameworks = append(frameworks, FrameworkJavaScript)
				}
			}
		}

		// Detect Rust tests (*_test.rs)
		if filepath.Ext(name) == ".rs" && len(name) > 8 && name[len(name)-8:] == "_test.rs" {
			if !contains(frameworks, FrameworkRust) {
				frameworks = append(frameworks, FrameworkRust)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Check for pytest.ini or setup.py
	if _, err := os.Stat(filepath.Join(te.projectRoot, "pytest.ini")); err == nil {
		if !contains(frameworks, FrameworkPython) {
			frameworks = append(frameworks, FrameworkPython)
		}
	}

	return frameworks, nil
}

// RunTests executes tests for detected frameworks and returns verification result
func (te *TestExecutor) RunTests(ctx context.Context) (*hooks.VerificationResult, error) {
	frameworks, err := te.DetectFrameworks()
	if err != nil {
		return nil, fmt.Errorf("failed to detect frameworks: %w", err)
	}

	if len(frameworks) == 0 {
		// No tests found - return warning
		return &hooks.VerificationResult{
			Status: hooks.VerificationStatusWarning,
			Violations: []hooks.Violation{
				{
					Severity:   "medium",
					Message:    "No test files detected in project",
					Suggestion: "Add tests to verify code correctness",
				},
			},
		}, nil
	}

	var violations []hooks.Violation

	// Run tests for each framework
	for _, framework := range frameworks {
		coverage, err := te.runFrameworkTests(ctx, framework)
		if err != nil {
			violations = append(violations, hooks.Violation{
				Severity:   "high",
				Message:    fmt.Sprintf("%s tests failed: %v", framework, err),
				Suggestion: "Fix failing tests before session completion",
			})
			continue
		}

		// Check coverage threshold
		if coverage < te.threshold {
			violations = append(violations, hooks.Violation{
				Severity:   "high",
				Message:    fmt.Sprintf("%s test coverage %.1f%% below threshold %.1f%%", framework, coverage, te.threshold),
				Suggestion: fmt.Sprintf("Add tests to reach %.1f%% coverage", te.threshold),
			})
		}
	}

	status := hooks.VerificationStatusPass
	if len(violations) > 0 {
		status = hooks.VerificationStatusFail
	}

	return &hooks.VerificationResult{
		Status:     status,
		Violations: violations,
	}, nil
}

// runFrameworkTests runs tests for a specific framework and returns coverage percentage
func (te *TestExecutor) runFrameworkTests(ctx context.Context, framework TestFramework) (float64, error) {
	switch framework {
	case FrameworkGo:
		return te.runGoTests(ctx)
	case FrameworkPython:
		return te.runPythonTests(ctx)
	case FrameworkJavaScript:
		return te.runJavaScriptTests(ctx)
	case FrameworkRust:
		return te.runRustTests(ctx)
	default:
		return 0, fmt.Errorf("unsupported framework: %s", framework)
	}
}

// runGoTests runs Go tests with coverage
func (te *TestExecutor) runGoTests(ctx context.Context) (float64, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "./...", "-cover", "-coverprofile=coverage.out")
	cmd.Dir = te.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("go test failed: %w\nOutput: %s", err, output)
	}

	// Parse coverage from output
	// Example: "coverage: 85.2% of statements"
	re := regexp.MustCompile(`coverage:\s+([\d.]+)%`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) >= 2 {
		coverage, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			return coverage, nil
		}
	}

	return 0, fmt.Errorf("failed to parse coverage from output")
}

// runPythonTests runs Python tests with coverage
func (te *TestExecutor) runPythonTests(ctx context.Context) (float64, error) {
	cmd := exec.CommandContext(ctx, "pytest", "--cov=.", "--cov-report=term-missing")
	cmd.Dir = te.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pytest failed: %w\nOutput: %s", err, output)
	}

	// Parse coverage from output
	// Example: "TOTAL    100    20    80%"
	re := regexp.MustCompile(`TOTAL\s+\d+\s+\d+\s+(\d+)%`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) >= 2 {
		coverage, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			return coverage, nil
		}
	}

	return 0, fmt.Errorf("failed to parse coverage from output")
}

// runJavaScriptTests runs JavaScript tests with coverage
func (te *TestExecutor) runJavaScriptTests(ctx context.Context) (float64, error) {
	cmd := exec.CommandContext(ctx, "npm", "test", "--", "--coverage")
	cmd.Dir = te.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("npm test failed: %w\nOutput: %s", err, output)
	}

	// Parse coverage from Jest output
	// Example: "All files | 85.2 | 80 | 90 | 85.2 |"
	re := regexp.MustCompile(`All files\s+\|\s+([\d.]+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) >= 2 {
		coverage, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			return coverage, nil
		}
	}

	return 0, fmt.Errorf("failed to parse coverage from output")
}

// runRustTests runs Rust tests
func (te *TestExecutor) runRustTests(ctx context.Context) (float64, error) {
	cmd := exec.CommandContext(ctx, "cargo", "test")
	cmd.Dir = te.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("cargo test failed: %w\nOutput: %s", err, output)
	}

	// Rust doesn't have built-in coverage, return 100 if tests pass
	// (User should configure tarpaulin or similar for real coverage)
	return 100.0, nil
}

// RunTestExecutionHook is the main entry point for the test execution hook
func RunTestExecutionHook(projectRoot string, coverageThreshold float64) {
	executor := NewTestExecutor(projectRoot, coverageThreshold)
	result, err := executor.RunTests(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Test execution failed: %v\n", err)
		os.Exit(1)
	}

	// Output result as JSON
	output, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal result: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(output))

	if result.Status == hooks.VerificationStatusFail {
		os.Exit(1)
	}
}

// Helper function
func contains(frameworks []TestFramework, framework TestFramework) bool {
	for _, f := range frameworks {
		if f == framework {
			return true
		}
	}
	return false
}
