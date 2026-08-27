package compaction

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func liveObservation(state string) session.DetectionResult {
	return session.DetectionResult{State: state, Evidence: session.EvidenceLive}
}

func TestRunPreflight_AllClear(t *testing.T) {
	state := &CompactionState{SessionName: "test"}
	result := RunPreflight(liveObservation(manifest.StateReady), state, false)
	if !result.OK {
		t.Errorf("should be OK, errors: %v", result.Errors)
	}
}

func TestRunPreflight_MidInference(t *testing.T) {
	state := &CompactionState{SessionName: "test"}
	result := RunPreflight(liveObservation(manifest.StateWorking), state, false)
	if result.OK {
		t.Error("should not be OK when WORKING")
	}
	if len(result.Errors) == 0 {
		t.Error("should have errors")
	}
}

func TestRunPreflight_AlreadyCompacting(t *testing.T) {
	state := &CompactionState{SessionName: "test"}
	result := RunPreflight(liveObservation(manifest.StateCompacting), state, false)
	if result.OK {
		t.Error("should not be OK when COMPACTING")
	}
}

func TestRunPreflight_AntiLoopBlockedNoForce(t *testing.T) {
	state := &CompactionState{
		SessionName:     "test",
		LastCompaction:  time.Now().Add(-30 * time.Minute),
		CompactionCount: 1,
	}
	result := RunPreflight(liveObservation(manifest.StateReady), state, false)
	if result.OK {
		t.Error("should not be OK when within cooldown without force")
	}
}

func TestRunPreflight_AntiLoopBlockedWithForce(t *testing.T) {
	state := &CompactionState{
		SessionName:     "test",
		LastCompaction:  time.Now().Add(-30 * time.Minute),
		CompactionCount: 1,
	}
	result := RunPreflight(liveObservation(manifest.StateReady), state, true)
	if !result.OK {
		t.Errorf("should be OK with force, errors: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Error("should have warnings when force-bypassing")
	}
}

func TestRunPreflight_RequiresPositiveLiveReadyEvidenceEvenWithForce(t *testing.T) {
	for _, evidence := range []session.ObservationEvidence{
		session.EvidenceTerminal,
		session.EvidenceUnknown,
		session.EvidenceUnreadable,
		session.EvidenceAbsent,
	} {
		t.Run(string(evidence), func(t *testing.T) {
			result := RunPreflight(session.DetectionResult{
				State:    manifest.StateReady,
				Evidence: evidence,
			}, &CompactionState{SessionName: "test"}, true)
			if result.OK {
				t.Fatalf("RunPreflight() OK with evidence %q and force, want rejection", evidence)
			}
		})
	}
}

func TestValidateReadyRejectsCompatibilityDone(t *testing.T) {
	err := ValidateReady(session.DetectionResult{
		State:    manifest.StateDone,
		Evidence: session.EvidenceLive,
	})
	if err == nil {
		t.Fatal("ValidateReady() error = nil for DONE display projection")
	}
}
