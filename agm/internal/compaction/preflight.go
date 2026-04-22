package compaction

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// PreflightResult holds the outcome of pre-flight checks.
type PreflightResult struct {
	OK       bool
	Errors   []string
	Warnings []string
}

// RunPreflight validates that compaction is safe to proceed.
func RunPreflight(currentState string, compactionState *CompactionState, force bool) *PreflightResult {
	result := &PreflightResult{OK: true}

	// Check 1: Not already compacting
	if currentState == manifest.StateCompacting {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("session is already compacting (state: %s)", currentState))
		return result
	}

	// Check 2: Session is at prompt (DONE), not mid-inference
	switch currentState {
	case manifest.StateWorking:
		result.OK = false
		result.Errors = append(result.Errors, "session is busy (state: WORKING). Wait for current work to complete")
	case manifest.StateUserPrompt:
		result.OK = false
		result.Errors = append(result.Errors, "session is waiting for user input (state: USER_PROMPT). Resolve the prompt first")
	case manifest.StateOffline:
		result.OK = false
		result.Errors = append(result.Errors, "session is offline")
	case manifest.StateDone:
		// Ready
	}

	if !result.OK {
		return result
	}

	// Check 3: Anti-loop safety
	if err := CheckAntiLoop(compactionState, force); err != nil {
		result.OK = false
		result.Errors = append(result.Errors, err.Error())
	} else if force {
		result.Warnings = append(result.Warnings, "anti-loop safety bypassed with --force")
	}

	return result
}
