package compaction

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

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
