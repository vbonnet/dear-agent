package compaction

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// ObserveExpectedHarnessSession returns an initial fail-closed readiness
// observation for the active pane. Atomic delivery must still re-prove the
// same condition at the mutation boundary.
func ObserveExpectedHarnessSession(ctx context.Context, sessionName, harness string) (*session.DetectionResult, error) {
	readiness, err := tmux.CheckExpectedHarnessInput(ctx, sessionName, harness)
	if err != nil {
		return nil, fmt.Errorf("observe expected harness readiness: %w", err)
	}
	result := detectionFromHarnessReadiness(readiness)
	return &result, nil
}

// ObserveVerificationTarget inspects only the pane and process identity that
// received the compaction command. Focus changes and other panes cannot
// contribute to the completion proof.
func ObserveVerificationTarget(ctx context.Context, target VerificationTarget) (*session.DetectionResult, error) {
	if err := target.validate(); err != nil {
		return nil, err
	}
	readiness, err := tmux.CheckExpectedHarnessInputForPane(
		ctx, target.PaneID, target.TargetPID, target.Harness,
	)
	if err != nil {
		return nil, fmt.Errorf("observe verified compaction pane: %w", err)
	}
	if err := validateVerificationReadinessIdentity(target, readiness); err != nil {
		return nil, err
	}
	result := detectionFromHarnessReadiness(readiness)
	return &result, nil
}

func validateVerificationReadinessIdentity(target VerificationTarget, readiness tmux.HarnessInputReadiness) error {
	// A deleted pane has no replacement identity to compare and remains a
	// typed absence. If tmux did resolve an incarnation, it must be the exact
	// stable-bound session that accepted delivery.
	if readiness.State == tmux.HarnessInputNotFound &&
		readiness.TargetSessionID == "" && readiness.StableSessionID == "" {
		return nil
	}
	if readiness.TargetPanePID != target.PanePID ||
		readiness.HarnessStartTime != target.HarnessStartTime ||
		readiness.TargetSessionID != target.TargetSessionID ||
		readiness.StableSessionID != target.StableSessionID {
		return fmt.Errorf(
			"verified compaction tmux identity changed: expected pane pid %d harness start %q session %q stable %q, observed pane pid %d harness start %q session %q stable %q",
			target.PanePID, target.HarnessStartTime, target.TargetSessionID, target.StableSessionID,
			readiness.TargetPanePID, readiness.HarnessStartTime, readiness.TargetSessionID, readiness.StableSessionID,
		)
	}
	return nil
}

func detectionFromHarnessReadiness(readiness tmux.HarnessInputReadiness) session.DetectionResult {
	result := session.DetectionResult{
		Confidence: 0.95,
		Reason:     fmt.Sprintf("expected harness readiness: %s", readiness.State),
	}
	switch readiness.State {
	case tmux.HarnessInputReady:
		if readiness.Ready {
			result.State = manifest.StateReady
			result.Evidence = session.EvidenceLive
			return result
		}
	case tmux.HarnessInputProcessing:
		result.State = manifest.StateWorking
		result.Evidence = session.EvidenceLive
		return result
	case tmux.HarnessInputQueuedAGM:
		// A queued AGM paste is positively live, but it is unsent composer
		// content rather than proof that compaction started.
		result.State = manifest.StateUserPrompt
		result.Evidence = session.EvidenceLive
		return result
	case tmux.HarnessInputBusy:
		// Generic busy includes human drafts and unrecognized tails. Exact
		// harness liveness does not turn that ambiguity into active-work proof.
		result.State = manifest.StateDone
		result.Evidence = session.EvidenceUnknown
		return result
	case tmux.HarnessInputPermission:
		result.State = manifest.StatePermissionPrompt
		result.Evidence = session.EvidenceLive
		return result
	case tmux.HarnessInputOverlay, tmux.HarnessInputOnboarding, tmux.HarnessInputReviewRequired:
		result.State = manifest.StateUserPrompt
		result.Evidence = session.EvidenceLive
		return result
	case tmux.HarnessInputNotFound:
		result.State = manifest.StateOffline
		result.Evidence = session.EvidenceAbsent
		return result
	case tmux.HarnessInputWrongHarness:
		result.State = manifest.StateDone
		result.Evidence = session.EvidenceTerminal
		return result
	}
	result.State = manifest.StateDone
	result.Evidence = session.EvidenceUnknown
	return result
}
