package dod

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// BeadDoD represents the Definition of Done for a bead.
// It defines machine-checkable criteria for completion.
type BeadDoD struct {
	FilesMustExist      []string       `yaml:"files_must_exist"`
	TestsMustPass       bool           `yaml:"tests_must_pass"`
	CommandsMustSucceed []CommandCheck `yaml:"commands_must_succeed"`
	// Phase 2 placeholder for benchmarking:
	// BenchmarksMustImprove []BenchmarkCheck `yaml:"benchmarks_must_improve,omitempty"`
}

// CommandCheck represents a command that must succeed with a specific exit code.
type CommandCheck struct {
	Cmd      string `yaml:"cmd"`
	ExitCode int    `yaml:"exit_code"`
}

// ValidationResult contains the outcome of DoD validation.
type ValidationResult struct {
	Success  bool
	Checks   []CheckResult
	Error    string
	Duration time.Duration
}

// CheckResult represents the result of a single check.
type CheckResult struct {
	Type    string // "file", "test", "command"
	Name    string // file path or command
	Success bool
	Error   string
}

// LoadDoD parses a DoD YAML file and returns a BeadDoD.
func LoadDoD(path string) (*BeadDoD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read DoD file: %w", err)
	}

	var dod BeadDoD
	if err := yaml.Unmarshal(data, &dod); err != nil {
		return nil, fmt.Errorf("failed to parse DoD YAML: %w", err)
	}

	return &dod, nil
}

// Validate runs all DoD checks and returns the aggregated result.
func (d *BeadDoD) Validate() (*ValidationResult, error) {
	start := time.Now()

	var allChecks []CheckResult

	// Run all checks
	allChecks = append(allChecks, d.checkFilesExist()...)
	allChecks = append(allChecks, d.checkTestsPass()...)
	allChecks = append(allChecks, d.checkCommandsSucceed()...)

	// Determine overall success
	success := true
	var errorMsg string
	for _, check := range allChecks {
		if !check.Success {
			success = false
			if errorMsg == "" {
				errorMsg = check.Error
			}
			break
		}
	}

	return &ValidationResult{
		Success:  success,
		Checks:   allChecks,
		Error:    errorMsg,
		Duration: time.Since(start),
	}, nil
}

// checkFilesExist verifies that all required files exist.
func (d *BeadDoD) checkFilesExist() []CheckResult {
	var results []CheckResult

	for _, path := range d.FilesMustExist {
		expanded := expandPath(path)
		check := CheckResult{
			Type:    "file",
			Name:    path,
			Success: true,
		}

		if _, err := os.Stat(expanded); err != nil {
			check.Success = false
			check.Error = fmt.Sprintf("file does not exist: %s (expanded: %s)", path, expanded)
		}

		results = append(results, check)
	}

	return results
}

// checkTestsPass runs tests if required.
func (d *BeadDoD) checkTestsPass() []CheckResult {
	var results []CheckResult

	if !d.TestsMustPass {
		// Tests not required, skip
		return results
	}

	check := CheckResult{
		Type:    "test",
		Name:    "go test ./...",
		Success: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	output, err := cmd.CombinedOutput()

	if err != nil {
		check.Success = false
		if ctx.Err() == context.DeadlineExceeded {
			check.Error = "tests timed out after 60 seconds"
		} else {
			check.Error = fmt.Sprintf("tests failed: %s\nOutput: %s", err, string(output))
		}
	}

	results = append(results, check)
	return results
}

// checkCommandsSucceed executes commands and verifies exit codes.
func (d *BeadDoD) checkCommandsSucceed() []CheckResult {
	var results []CheckResult

	for _, cmdCheck := range d.CommandsMustSucceed {
		check := CheckResult{
			Type:    "command",
			Name:    cmdCheck.Cmd,
			Success: true,
		}

		exitCode, output, err := executeCommand(cmdCheck.Cmd, 30*time.Second)

		if err != nil {
			check.Success = false
			check.Error = fmt.Sprintf("command execution failed: %s", err)
		} else if exitCode != cmdCheck.ExitCode {
			check.Success = false
			check.Error = fmt.Sprintf("command exit code mismatch: expected %d, got %d\nCommand: %s\nOutput: %s",
				cmdCheck.ExitCode, exitCode, cmdCheck.Cmd, output)
		}

		results = append(results, check)
	}

	return results
}

// expandPath expands tilde and environment variables in a path.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return os.ExpandEnv(path)
}

// executeCommand runs a command with a timeout and returns the exit code and output.
func executeCommand(cmdStr string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return -1, string(output), fmt.Errorf("command timed out after %v", timeout)
	}

	exitCode := 0
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, string(output), fmt.Errorf("command execution error: %w", err)
		}
	}

	return exitCode, string(output), nil
}
