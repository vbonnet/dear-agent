package compaction

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// PreflightResult holds the outcome of pre-flight checks.
type PreflightResult struct {
	OK       bool
	Errors   []string
	Warnings []string
}

// ValidateReady requires positive evidence that the target session is live and
// ready. Compatibility display states are not readiness authority.
func ValidateReady(observation session.DetectionResult) error {
	if observation.Evidence != session.EvidenceLive {
		return fmt.Errorf("session readiness is unverified (state: %s, evidence: %s)", observation.State, observation.Evidence)
	}

	switch observation.State {
	case manifest.StateReady:
		return nil
	case manifest.StateCompacting:
		return fmt.Errorf("session is already compacting (state: %s)", observation.State)
	case manifest.StateWorking:
		return fmt.Errorf("session is busy (state: WORKING). Wait for current work to complete")
	case manifest.StateUserPrompt, manifest.StatePermissionPrompt:
		return fmt.Errorf("session is waiting for user input (state: %s). Resolve the prompt first", observation.State)
	case manifest.StateOffline:
		return fmt.Errorf("session is offline")
	default:
		return fmt.Errorf("session is not ready for compaction (state: %s)", observation.State)
	}
}

// RunPreflight validates that compaction is safe to proceed.
func RunPreflight(observation session.DetectionResult, compactionState *CompactionState, force bool) *PreflightResult {
	result := &PreflightResult{OK: true}

	// Check 1: Positive live-ready evidence. Force never bypasses this gate.
	if err := ValidateReady(observation); err != nil {
		result.OK = false
		result.Errors = append(result.Errors, err.Error())
		return result
	}

	// Check 2: Anti-loop safety
	if err := CheckAntiLoop(compactionState, force); err != nil {
		result.OK = false
		result.Errors = append(result.Errors, err.Error())
	} else if force {
		result.Warnings = append(result.Warnings, "anti-loop safety bypassed with --force")
	}

	return result
}
