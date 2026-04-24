package retrospective

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// CaptureContext captures git state, deliverable summaries, and phase state concurrently
//
// Uses parallel goroutines with sync.WaitGroup for performance (<500ms target).
// Git operations have 500ms timeout to prevent blocking on large repos.
func CaptureContext(projectDir string, st *status.Status) ContextSnapshot {
	var wg sync.WaitGroup
	snapshot := ContextSnapshot{}

	// Error channel for collecting errors from goroutines
	errChan := make(chan error, 3)

	// Capture git context (with timeout)
	wg.Add(1)
	go func() {
		defer wg.Done()
		gitCtx, err := captureGitContext(projectDir)
		if err != nil {
			errChan <- fmt.Errorf("git context: %w", err)
			gitCtx.Error = err.Error()
		}
		snapshot.Git = gitCtx
	}()

	// Capture deliverables (scan for {PHASE}-*.md files)
	wg.Add(1)
	go func() {
		defer wg.Done()
		deliverables, err := captureDeliverables(projectDir)
		if err != nil {
			errChan <- fmt.Errorf("deliverables: %w", err)
			// Partial results acceptable
		}
		snapshot.Deliverables = deliverables
	}()

	// Capture phase state (from status.Status)
	wg.Add(1)
	go func() {
		defer wg.Done()
		phaseCtx := capturePhaseContext(st)
		snapshot.PhaseState = phaseCtx
	}()

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Log errors to stderr (non-blocking)
	for err := range errChan {
		fmt.Fprintf(os.Stderr, "Warning: context capture error: %v\n", err)
	}

	return snapshot
}

// captureGitContext captures git repository state with 5s timeout
func captureGitContext(projectDir string) (GitContext, error) {
	gitCtx := GitContext{}

	// Create context with 5s timeout (500ms was too tight for CI/container environments)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get current branch
	branchCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "--abbrev-ref", "HEAD")
	branchOutput, err := branchCmd.Output()
	if err != nil {
		return gitCtx, fmt.Errorf("failed to get git branch: %w", err)
	}
	gitCtx.Branch = strings.TrimSpace(string(branchOutput))

	// Get current commit (short SHA)
	commitCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "--short", "HEAD")
	commitOutput, err := commitCmd.Output()
	if err != nil {
		return gitCtx, fmt.Errorf("failed to get git commit: %w", err)
	}
	gitCtx.Commit = strings.TrimSpace(string(commitOutput))

	// Check for uncommitted changes (git status --porcelain)
	statusCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "status", "--porcelain")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return gitCtx, fmt.Errorf("failed to get git status: %w", err)
	}
	gitCtx.UncommittedChanges = len(strings.TrimSpace(string(statusOutput))) > 0

	return gitCtx, nil
}

// captureDeliverables scans for phase deliverable files ({PHASE}-*.md)
func captureDeliverables(projectDir string) ([]string, error) {
	var deliverables []string

	// All Wayfinder phases
	phases := status.AllPhases()

	for _, phase := range phases {
		// Glob pattern: {PHASE}-*.md (e.g., D1-*.md, S6-*.md)
		pattern := filepath.Join(projectDir, fmt.Sprintf("%s-*.md", phase))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return deliverables, fmt.Errorf("failed to glob phase %s: %w", phase, err)
		}

		// Extract just filenames (not full paths)
		for _, match := range matches {
			deliverables = append(deliverables, filepath.Base(match))
		}
	}

	return deliverables, nil
}

// capturePhaseContext extracts phase state from status.Status
func capturePhaseContext(st *status.Status) PhaseContext {
	phaseCtx := PhaseContext{
		CurrentPhase:    st.CurrentPhase,
		SessionID:       st.SessionID,
		CompletedPhases: []string{},
	}

	// Collect completed phases
	for _, phase := range st.Phases {
		if phase.Status == status.PhaseStatusCompleted {
			phaseCtx.CompletedPhases = append(phaseCtx.CompletedPhases, phase.Name)
		}
	}

	return phaseCtx
}
