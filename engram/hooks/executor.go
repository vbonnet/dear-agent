package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

var (
	// ErrTimeout is returned when hook execution exceeds timeout
	ErrTimeout = errors.New("hook execution timeout")
	// ErrOutputTooLarge is returned when hook output exceeds size limit
	ErrOutputTooLarge = errors.New("hook output too large")
)

const (
	// MaxOutputSize is the maximum size of hook output (1MB)
	MaxOutputSize = 1024 * 1024
	// DefaultTimeout is the default hook execution timeout
	DefaultTimeout = 60 * time.Second
)

// HookExecutor defines the interface for executing hooks
type HookExecutor interface {
	Execute(ctx context.Context, hook Hook) (*VerificationResult, error)
}

// Executor handles subprocess execution for hooks
type Executor struct {
	validator *CommandValidator
}

// NewExecutor creates a new hook executor
func NewExecutor(validator *CommandValidator) *Executor {
	return &Executor{
		validator: validator,
	}
}

// Execute runs a hook command and returns the verification result
func (e *Executor) Execute(ctx context.Context, hook Hook) (*VerificationResult, error) {
	// Validate command against allowlist
	if err := e.validator.ValidateCommand(hook.Command); err != nil {
		return &VerificationResult{
			HookName: hook.Name,
			Status:   VerificationStatusFail,
			Violations: []Violation{{
				Severity:   "high",
				Message:    fmt.Sprintf("Security violation: %v", err),
				Suggestion: "Command not in allowlist. Add to ~/.engram/allowed-commands.toml if trusted.",
			}},
			ExitCode: 1,
		}, err
	}

	// Verify command hash if configured
	if hook.CommandHash != "" {
		if err := e.validator.VerifyCommandHash(hook.Command, hook.CommandHash); err != nil {
			return &VerificationResult{
				HookName: hook.Name,
				Status:   VerificationStatusFail,
				Violations: []Violation{{
					Severity:   "high",
					Message:    fmt.Sprintf("Security violation: %v", err),
					Suggestion: "Hook binary has been modified. Recalculate hash or investigate tampering.",
				}},
				ExitCode: 1,
			}, err
		}
	}

	// Set timeout
	timeout := time.Duration(hook.Timeout) * time.Second
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute command with no shell (prevents injection)
	cmd := exec.CommandContext(timeoutCtx, hook.Command, hook.Args...)

	// Capture stdout with size limit
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Check for timeout
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return &VerificationResult{
			HookName: hook.Name,
			Status:   VerificationStatusWarning,
			Violations: []Violation{{
				Severity:   "medium",
				Message:    fmt.Sprintf("Hook execution timeout after %v", timeout),
				Suggestion: "Increase timeout in hook configuration or optimize hook performance.",
			}},
			Duration: duration,
			ExitCode: -1,
		}, ErrTimeout
	}

	// Check output size
	if stdout.Len() >= MaxOutputSize {
		return &VerificationResult{
			HookName: hook.Name,
			Status:   VerificationStatusWarning,
			Violations: []Violation{{
				Severity:   "medium",
				Message:    fmt.Sprintf("Hook output exceeded %d bytes limit", MaxOutputSize),
				Suggestion: "Reduce hook output or increase output limit.",
			}},
			Duration: duration,
			ExitCode: -1,
		}, ErrOutputTooLarge
	}

	// Parse output as JSON
	result, parseErr := parseHookOutput(stdout.Bytes())
	if parseErr != nil {
		// Not valid JSON - create result from exit code and stderr
		exitCode := 0
		if err != nil {
			exitCode = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}

		status := VerificationStatusPass
		var violations []Violation

		if exitCode != 0 {
			status = VerificationStatusFail
			violations = []Violation{{
				Severity:   "high",
				Message:    fmt.Sprintf("Hook failed with exit code %d", exitCode),
				Suggestion: "Check hook logs for details.",
			}}
			if stderr.Len() > 0 {
				violations[0].Message += fmt.Sprintf(": %s", stderr.String())
			}
		}

		return &VerificationResult{
			HookName:   hook.Name,
			Status:     status,
			Violations: violations,
			Duration:   duration,
			ExitCode:   exitCode,
		}, nil
	}

	// Update result with hook name and duration
	result.HookName = hook.Name
	result.Duration = duration

	return result, nil
}

// parseHookOutput parses JSON output from a hook
func parseHookOutput(output []byte) (*VerificationResult, error) {
	if len(output) == 0 {
		return nil, errors.New("empty output")
	}

	var result VerificationResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &result, nil
}
