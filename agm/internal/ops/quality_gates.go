package ops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// QualityGate defines a single holdout quality check.
type QualityGate struct {
	Name        string `yaml:"name" json:"name"`
	Check       string `yaml:"check" json:"check"`
	ExpectExit  *int   `yaml:"expect_exit,omitempty" json:"expect_exit,omitempty"`
	ExpectEmpty *bool  `yaml:"expect_empty,omitempty" json:"expect_empty,omitempty"`
}

// QualityGatesConfig is the top-level YAML structure for quality gates.
type QualityGatesConfig struct {
	Gates []QualityGate `yaml:"gates" json:"gates"`
}

// QualityGateCheckResult holds the outcome of a single gate check.
type QualityGateCheckResult struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output,omitempty"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// RunQualityGatesRequest is the input for running quality gates.
type RunQualityGatesRequest struct {
	SessionName string `json:"session_name"`
	ConfigPath  string `json:"config_path"`
	RepoDir     string `json:"repo_dir"`
	Branch      string `json:"branch"`
	RecordTrust bool   `json:"record_trust"`
}

// RunQualityGatesResult is the output of running quality gates.
type RunQualityGatesResult struct {
	SessionName string                   `json:"session_name"`
	Passed      bool                     `json:"passed"`
	TotalGates  int                      `json:"total_gates"`
	PassedCount int                      `json:"passed_count"`
	FailedCount int                      `json:"failed_count"`
	Gates       []QualityGateCheckResult `json:"gates"`
}

// LoadQualityGatesConfig reads and parses a quality gates YAML config file.
func LoadQualityGatesConfig(path string) (*QualityGatesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read quality gates config: %w", err)
	}

	var cfg QualityGatesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse quality gates config: %w", err)
	}

	if len(cfg.Gates) == 0 {
		return nil, fmt.Errorf("quality gates config has no gates defined")
	}

	for i, g := range cfg.Gates {
		if g.Name == "" {
			return nil, fmt.Errorf("gate %d: name is required", i)
		}
		if g.Check == "" {
			return nil, fmt.Errorf("gate %q: check command is required", g.Name)
		}
		if g.ExpectExit == nil && g.ExpectEmpty == nil {
			return nil, fmt.Errorf("gate %q: must specify expect_exit or expect_empty", g.Name)
		}
	}

	return &cfg, nil
}

// RunQualityGates executes all quality gates against a session's working directory.
// The repo dir should already be on the correct branch.
func RunQualityGates(ctx *OpContext, req *RunQualityGatesRequest) (*RunQualityGatesResult, error) {
	if req.SessionName == "" {
		return nil, ErrInvalidInput("session_name", "session name is required")
	}
	if req.ConfigPath == "" {
		return nil, ErrInvalidInput("config_path", "quality gates config path is required")
	}
	if req.RepoDir == "" {
		return nil, ErrInvalidInput("repo_dir", "repository directory is required")
	}

	cfg, err := LoadQualityGatesConfig(req.ConfigPath)
	if err != nil {
		return nil, ErrInvalidInput("config_path", err.Error())
	}

	result := &RunQualityGatesResult{
		SessionName: req.SessionName,
		TotalGates:  len(cfg.Gates),
	}

	for _, gate := range cfg.Gates {
		checkResult := runSingleGate(gate, req.RepoDir, req.Branch)
		result.Gates = append(result.Gates, checkResult)
		if checkResult.Passed {
			result.PassedCount++
		} else {
			result.FailedCount++
		}
	}

	result.Passed = result.FailedCount == 0

	// Record failed gates to error memory
	if !result.Passed {
		var failedNames []string
		for _, g := range result.Gates {
			if !g.Passed {
				failedNames = append(failedNames, g.Name)
			}
		}
		pattern := fmt.Sprintf("%d/%d quality gates failed: %s", result.FailedCount, result.TotalGates, strings.Join(failedNames, ", "))
		recordErrorMemory(
			pattern,
			ErrMemCatQualityGate,
			"",
			"Review gate output and fix failing checks",
			SourceAGMQualityGate,
			req.SessionName,
		)
	}

	if req.RecordTrust {
		if result.Passed {
			_, _ = TrustRecord(ctx, &TrustRecordRequest{
				SessionName: req.SessionName,
				EventType:   string(TrustEventSuccess),
				Detail:      fmt.Sprintf("all %d quality gates passed", result.TotalGates),
			})
		} else {
			var failedNames []string
			for _, g := range result.Gates {
				if !g.Passed {
					failedNames = append(failedNames, g.Name)
				}
			}
			_, _ = TrustRecord(ctx, &TrustRecordRequest{
				SessionName: req.SessionName,
				EventType:   string(TrustEventQualityGateFailure),
				Detail:      fmt.Sprintf("%d/%d gates failed: %s", result.FailedCount, result.TotalGates, strings.Join(failedNames, ", ")),
			})
		}
	}

	return result, nil
}

// runSingleGate executes a single quality gate check and returns the result.
func runSingleGate(gate QualityGate, repoDir, branch string) QualityGateCheckResult {
	start := time.Now()

	// Substitute $BRANCH in check commands
	checkCmd := gate.Check
	if branch != "" {
		checkCmd = strings.ReplaceAll(checkCmd, "$BRANCH", branch)
	}

	cmd := exec.Command("bash", "-c", checkCmd)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return QualityGateCheckResult{
				Name:       gate.Name,
				Passed:     false,
				ExitCode:   -1,
				Output:     stderr.String(),
				DurationMs: durationMs,
				Error:      err.Error(),
			}
		}
	}

	output := stdout.String() + stderr.String()

	passed := gate.ExpectExit == nil || exitCode == *gate.ExpectExit

	if gate.ExpectEmpty != nil && *gate.ExpectEmpty && len(strings.TrimSpace(output)) > 0 {
		passed = false
	}

	return QualityGateCheckResult{
		Name:       gate.Name,
		Passed:     passed,
		ExitCode:   exitCode,
		Output:     output,
		DurationMs: durationMs,
	}
}
