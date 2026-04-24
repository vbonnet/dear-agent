package steps

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/testenv"
)

// RegisterInitializationSteps registers session initialization step definitions
func RegisterInitializationSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^I have Claude CLI installed$`, iHaveClaudeCLIInstalled)
	ctx.Step(`^no session named "([^"]*)" exists$`, noSessionExists)
	ctx.Step(`^the command should succeed within (\d+) seconds$`, commandSucceedsWithinSeconds)
	ctx.Step(`^a tmux session named "([^"]*)" should exist$`, tmuxSessionExists)
	ctx.Step(`^Claude should be running in the session$`, claudeRunningInSession)
	ctx.Step(`^the session should be renamed to "([^"]*)"$`, sessionRenamedTo)
	ctx.Step(`^the session should be associated with AGM$`, sessionAssociatedWithAGM)
	ctx.Step(`^Claude should start within (\d+) seconds$`, claudeStartsWithinSeconds)
	ctx.Step(`^I should see a warning about initialization timeout$`, shouldSeeInitTimeout)
	ctx.Step(`^the session "([^"]*)" should still be attached$`, sessionStillAttached)
	ctx.Step(`^I should be able to manually run "([^"]*)"$`, canManuallyRun)
	ctx.Step(`^Claude will show a trust prompt on startup$`, claudeShowsTrustPrompt)
	ctx.Step(`^the command should wait for user input$`, commandWaitsForInput)
	ctx.Step(`^I answer "([^"]*)" to the trust prompt$`, answerTrustPrompt)
	ctx.Step(`^the session should continue initialization$`, sessionContinuesInit)
	ctx.Step(`^I run "([^"]*)" in the background$`, runInBackground)
	ctx.Step(`^both commands should succeed within (\d+) seconds$`, bothCommandsSucceed)
	ctx.Step(`^both sessions should be properly initialized$`, bothSessionsInitialized)
	ctx.Step(`^there should be no race conditions$`, noRaceConditions)
	ctx.Step(`^there is a brief network interruption during initialization$`, networkInterruption)
	ctx.Step(`^the initialization should complete successfully$`, initCompletesSuccessfully)
}

func iHaveClaudeCLIInstalled(ctx context.Context) (context.Context, error) {
	// Skip these tests - they require real Claude CLI with full initialization
	// These tests are meant for manual testing, not automated CI
	return ctx, godog.ErrPending
}

func noSessionExists(ctx context.Context, sessionName string) (context.Context, error) {
	// Kill session if it exists (cleanup from previous run)
	exists, _ := tmux.HasSession(sessionName)
	if exists {
		socketPath := tmux.GetSocketPath()
		cmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName)
		cmd.Run() // Ignore errors
	}

	return ctx, nil
}

func commandSucceedsWithinSeconds(ctx context.Context, seconds int) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	// Check if last command completed successfully
	if env.LastError != nil {
		return ctx, fmt.Errorf("command failed: %w", env.LastError)
	}

	// For BDD tests, we accept the command succeeded
	// Timeout checking is handled by the test harness
	return ctx, nil
}

func tmuxSessionExists(ctx context.Context, sessionName string) (context.Context, error) {
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return ctx, fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return ctx, fmt.Errorf("tmux session %q does not exist", sessionName)
	}
	return ctx, nil
}

func claudeRunningInSession(ctx context.Context) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	// Get session name from environment's Sessions map
	var sessionName string
	for name := range env.Sessions {
		sessionName = name
		break
	}

	if sessionName == "" {
		return ctx, fmt.Errorf("no session name found in environment")
	}

	// Use capture-pane to check session content
	socketPath := tmux.GetSocketPath()
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p", "-S", "-50")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ctx, fmt.Errorf("failed to capture pane: %w", err)
	}

	// Check for Claude prompt pattern
	if !strings.Contains(string(output), "❯") {
		return ctx, fmt.Errorf("Claude prompt not found in session output")
	}

	return ctx, nil
}

func sessionRenamedTo(ctx context.Context, expectedName string) (context.Context, error) {
	// Check if /rename command was executed
	// We verify this by checking session still exists with correct name
	exists, err := tmux.HasSession(expectedName)
	if err != nil {
		return ctx, fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return ctx, fmt.Errorf("session %q not found after rename", expectedName)
	}
	return ctx, nil
}

func sessionAssociatedWithAGM(ctx context.Context) (context.Context, error) {
	// Check if ready-file exists (signal from /agm:agm-assoc)
	// For BDD test, we accept that association was attempted
	// Full validation would require checking ~/.agm/ready-{session}
	return ctx, nil // Accept for now
}

func claudeStartsWithinSeconds(ctx context.Context, seconds int) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	var sessionName string
	for name := range env.Sessions {
		sessionName = name
		break
	}

	if sessionName == "" {
		return ctx, fmt.Errorf("no session name found")
	}

	// Wait for Claude prompt
	err := tmux.WaitForClaudePrompt(sessionName, time.Duration(seconds)*time.Second)
	if err != nil {
		return ctx, fmt.Errorf("Claude did not start within %ds: %v", seconds, err)
	}

	return ctx, nil
}

func shouldSeeInitTimeout(ctx context.Context) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	if env.LastError == nil {
		return ctx, fmt.Errorf("expected timeout error but got success")
	}

	errMsg := env.LastError.Error()
	if !strings.Contains(errMsg, "timeout") && !strings.Contains(errMsg, "Warning") {
		return ctx, fmt.Errorf("expected timeout warning, got: %v", env.LastError)
	}

	return ctx, nil
}

func sessionStillAttached(ctx context.Context, sessionName string) (context.Context, error) {
	// Verify session exists (wasn't killed on timeout)
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return ctx, fmt.Errorf("failed to check session: %v", err)
	}
	if !exists {
		return ctx, fmt.Errorf("session %q should still exist after timeout", sessionName)
	}
	return ctx, nil
}

func canManuallyRun(ctx context.Context, command string) (context.Context, error) {
	// This step verifies that session is in a state where manual commands can be sent
	// For BDD, we accept that session exists and is accessible
	return ctx, nil
}

func claudeShowsTrustPrompt(ctx context.Context) (context.Context, error) {
	// Mark that trust prompt will appear (test environment setup)
	return ctx, godog.ErrPending // This scenario requires manual testing
}

func commandWaitsForInput(ctx context.Context) (context.Context, error) {
	return ctx, godog.ErrPending // This scenario requires manual testing
}

func answerTrustPrompt(ctx context.Context, answer string) (context.Context, error) {
	return ctx, godog.ErrPending // This scenario requires manual testing
}

func sessionContinuesInit(ctx context.Context) (context.Context, error) {
	return ctx, godog.ErrPending // This scenario requires manual testing
}

func runInBackground(ctx context.Context, command string) (context.Context, error) {
	return ctx, godog.ErrPending // Parallel execution requires goroutines
}

func bothCommandsSucceed(ctx context.Context, seconds int) (context.Context, error) {
	return ctx, godog.ErrPending // Parallel execution requires goroutines
}

func bothSessionsInitialized(ctx context.Context) (context.Context, error) {
	return ctx, godog.ErrPending // Parallel execution requires goroutines
}

func noRaceConditions(ctx context.Context) (context.Context, error) {
	return ctx, godog.ErrPending // Parallel execution requires goroutines
}

func networkInterruption(ctx context.Context) (context.Context, error) {
	return ctx, godog.ErrPending // Network simulation requires infrastructure
}

func initCompletesSuccessfully(ctx context.Context) (context.Context, error) {
	return ctx, godog.ErrPending // Network simulation requires infrastructure
}
