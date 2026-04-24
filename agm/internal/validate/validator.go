// Package validate provides session resumability validation for AGM.
//
// This package implements functional testing of AGM sessions by actually
// attempting to resume each session in a temporary tmux session, detecting
// errors, and providing auto-fix capabilities for common issues.
package validate

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Constants for validation
const (
	sessionCleanupWait = 100 * time.Millisecond
	testInterval       = 1 * time.Second
)

// Regular expressions for input validation
var (
	// UUID v4 format: 8-4-4-4-12 hexadecimal characters
	uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// Safe session name: alphanumeric, dash, underscore only
	safeNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// validateUUID checks if the UUID is in valid v4 format to prevent command injection.
func validateUUID(uuid string) error {
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format: %s (expected UUID v4 format)", uuid)
	}
	return nil
}

// sanitizeSessionName creates a safe session name for tmux.
// Returns an error if the name contains unsafe characters.
func sanitizeSessionName(name string) (string, error) {
	if !safeNameRegex.MatchString(name) {
		return "", fmt.Errorf("unsafe session name: %s (only alphanumeric, dash, underscore allowed)", name)
	}
	return "agm-validate-" + name, nil
}

// testSessionResume creates a temporary tmux session and attempts to resume
// the given session, returning the tmux output and any errors encountered.
//
// The test session is automatically cleaned up via defer, even if errors occur.
func testSessionResume(m *manifest.Manifest, timeout time.Duration) (string, error) {
	// Validate inputs to prevent command injection
	if err := validateUUID(m.Claude.UUID); err != nil {
		return "", &ValidationError{
			Session: m.Name,
			Phase:   "input_validation",
			Cause:   err,
		}
	}

	sessionName, err := sanitizeSessionName(m.Name)
	if err != nil {
		return "", &ValidationError{
			Session: m.Name,
			Phase:   "input_validation",
			Cause:   err,
		}
	}

	// 1. Create test session
	if err := tmux.NewSession(sessionName, m.Context.Project); err != nil {
		return "", fmt.Errorf("failed to create test session: %w", err)
	}

	// 2. Ensure cleanup happens
	defer killSession(sessionName)

	// 3. Send resume command (UUID is validated, safe to use)
	resumeCmd := fmt.Sprintf("claude --resume %s", m.Claude.UUID)
	if err := tmux.SendCommand(sessionName, resumeCmd); err != nil {
		output, captureErr := capturePane(sessionName)
		if captureErr != nil {
			return "", fmt.Errorf("failed to send resume command: %w (capture error: %v)", err, captureErr)
		}
		return output, fmt.Errorf("failed to send resume command: %w", err)
	}

	// 4. Wait for process with timeout
	if timeout > 0 {
		if err := tmux.WaitForClaudeReady(sessionName, timeout); err != nil {
			output, captureErr := capturePane(sessionName)
			if captureErr != nil {
				return "", fmt.Errorf("process not ready: %w (capture error: %v)", err, captureErr)
			}
			return output, err
		}
	}

	// 5. Capture pane output
	output, err := capturePane(sessionName)
	if err != nil {
		return "", fmt.Errorf("failed to capture pane output: %w", err)
	}

	return output, nil
}

// killSession kills a tmux session by name.
func killSession(sessionName string) error {
	ctx := context.Background()
	_, err := exec.CommandContext(ctx, "tmux", "kill-session", "-t", sessionName).CombinedOutput()
	if err != nil {
		// Ignore error if session doesn't exist (exit code 1)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("failed to kill session %s: %w", sessionName, err)
	}
	// Wait briefly for session to terminate
	time.Sleep(sessionCleanupWait)
	return nil
}

// capturePane captures the current content of a tmux pane.
func capturePane(sessionName string) (string, error) {
	ctx := context.Background()
	output, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", sessionName, "-p").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane: %w", err)
	}
	return string(output), nil
}

// RunValidation validates all sessions and returns a report.
func RunValidation(manifests []*manifest.Manifest, opts *Options) (*Report, error) {
	report := &Report{
		ValidatedAt:   time.Now(),
		TotalSessions: len(manifests),
		Sessions:      make([]SessionResult, 0, len(manifests)),
	}

	timeout := time.Duration(opts.TimeoutPerSession) * time.Second

	for i, m := range manifests {
		// 1. Check lock status before testing
		lockPath, err := lock.DefaultLockPath()
		if err != nil {
			report.Unknown++
			report.Sessions = append(report.Sessions, SessionResult{
				Name:   m.Name,
				UUID:   m.Claude.UUID,
				Path:   m.Context.Project,
				Status: "unknown",
				Issues: []Issue{
					{
						Type:        IssueEnvironment,
						Message:     fmt.Sprintf("Failed to get lock path: %v", err),
						Fix:         "Check system configuration and permissions",
						AutoFixable: false,
					},
				},
				Manifest: m,
			})
			continue
		}

		lockInfo, err := lock.CheckLock(lockPath)
		if err != nil {
			report.Unknown++
			report.Sessions = append(report.Sessions, SessionResult{
				Name:   m.Name,
				UUID:   m.Claude.UUID,
				Path:   m.Context.Project,
				Status: "unknown",
				Issues: []Issue{
					{
						Type:        IssueEnvironment,
						Message:     fmt.Sprintf("Failed to check lock status: %v", err),
						Fix:         "Check lock file permissions and system state",
						AutoFixable: false,
					},
				},
				Manifest: m,
			})
			continue
		}

		if lockInfo != nil && lockInfo.IsStale {
			// TODO: Prompt user to unlock stale lock
			// For now, skip this session
			report.Unknown++
			report.Sessions = append(report.Sessions, SessionResult{
				Name:   m.Name,
				UUID:   m.Claude.UUID,
				Path:   m.Context.Project,
				Status: "unknown",
				Issues: []Issue{
					{
						Type:        IssueLockContention,
						Message:     "Stale lock detected",
						Fix:         "Run agm admin unlock to remove stale lock",
						AutoFixable: false,
					},
				},
				Manifest: m,
			})
			continue
		}

		// 2. Test session resume
		output, err := testSessionResume(m, timeout)

		// 3. Classify result
		var issues []Issue
		var status string

		if err != nil {
			issue := classifyResumeError(output, err)
			issues = append(issues, *issue)
			status = "failed"
			report.Failed++
		} else {
			status = "resumable"
			report.Resumable++
		}

		// 4. Add to report
		report.Sessions = append(report.Sessions, SessionResult{
			Name:     m.Name,
			UUID:     m.Claude.UUID,
			Path:     m.Context.Project,
			Status:   status,
			Issues:   issues,
			Fixed:    false,
			Manifest: m,
		})

		// 5. Wait between tests to avoid lock contention
		if i < len(manifests)-1 {
			time.Sleep(testInterval)
		}
	}

	return report, nil
}

// classifyResumeError analyzes tmux output and error to determine the specific issue type.
func classifyResumeError(output string, err error) *Issue {
	// Check for specific error patterns in tmux output
	switch {
	// Empty session env - missing or malformed session-env manifest
	case strings.Contains(output, "TypeError: Cannot read properties of undefined"):
		return &Issue{
			Type:        IssueEmptySessionEnv,
			Message:     "Empty or malformed session-env manifest",
			Fix:         "Run 'agm admin doctor --validate --fix' to backfill from JSONL metadata",
			AutoFixable: true,
		}

	// JSONL missing or UUID mismatch
	case strings.Contains(output, "No conversation found"):
		return &Issue{
			Type:        IssueJSONLMissing,
			Message:     "JSONL file missing or UUID mismatch",
			Fix:         "Verify JSONL file exists and UUID matches manifest",
			AutoFixable: false,
		}

	// Version mismatch
	case strings.Contains(output, "No messages returned"):
		return &Issue{
			Type:        IssueVersionMismatch,
			Message:     "Version mismatch between manifest and Claude Code",
			Fix:         "Run 'agm admin doctor --validate --fix' to update manifest version",
			AutoFixable: true,
		}

	// Compacted JSONL - summaries blocking resume
	case strings.Contains(output, "summary entries") || strings.Contains(output, "summaries"):
		return &Issue{
			Type:        IssueCompactedJSONL,
			Message:     "JSONL file has summaries at start, blocking resume",
			Fix:         "Run 'agm admin doctor --validate --fix' to reorder JSONL (creates backup)",
			AutoFixable: true,
		}

	// CWD mismatch - blank screen
	case output == "" || strings.TrimSpace(output) == "":
		return &Issue{
			Type:        IssueCwdMismatch,
			Message:     "Session resumed but no output (possible working directory mismatch)",
			Fix:         "Run 'agm admin doctor --validate --fix' to update working directory from JSONL",
			AutoFixable: true,
		}

	// Lock contention
	case strings.Contains(output, "Another agm command is currently running"):
		return &Issue{
			Type:        IssueLockContention,
			Message:     "AGM lock held by another process",
			Fix:         "Run 'agm admin unlock' to remove stale locks",
			AutoFixable: false,
		}

	// Permissions issues
	case strings.Contains(output, "Permission denied") || strings.Contains(output, "EACCES"):
		return &Issue{
			Type:        IssuePermissions,
			Message:     "File or directory access denied",
			Fix:         "Check file permissions on session directory and JSONL file",
			AutoFixable: false,
		}

	// Corrupted data
	case strings.Contains(output, "invalid JSON") || strings.Contains(output, "SyntaxError"):
		return &Issue{
			Type:        IssueCorruptedData,
			Message:     "Invalid YAML or JSONL data detected",
			Fix:         "Manual inspection required - check manifest.yaml and conversation JSONL",
			AutoFixable: false,
		}

	// Missing dependencies
	case strings.Contains(output, "command not found") || strings.Contains(output, "not installed"):
		return &Issue{
			Type:        IssueMissingDependency,
			Message:     "Required dependency missing (tmux, claude, or other)",
			Fix:         "Install missing dependencies: tmux, Claude Code CLI",
			AutoFixable: false,
		}

	// Environment issues
	case strings.Contains(output, "PATH") || strings.Contains(output, "shell"):
		return &Issue{
			Type:        IssueEnvironment,
			Message:     "Shell or environment configuration issue",
			Fix:         "Check shell configuration and PATH settings",
			AutoFixable: false,
		}

	// Session conflict
	case strings.Contains(output, "already exists") || strings.Contains(output, "UUID collision"):
		return &Issue{
			Type:        IssueSessionConflict,
			Message:     "Session name or UUID conflicts with existing session",
			Fix:         "Resolve duplicate session names or UUIDs",
			AutoFixable: false,
		}

	// Unknown error
	default:
		return &Issue{
			Type:        IssueUnknown,
			Message:     fmt.Sprintf("Unknown error: %v", err),
			Fix:         "Manual investigation required. Check tmux output: " + output,
			AutoFixable: false,
		}
	}
}
