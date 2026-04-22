package compaction

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestRunPreflight_AllClear(t *testing.T) {
	state := &CompactionState{SessionName: "test"}
	result := RunPreflight(manifest.StateDone, state, false)
	if !result.OK {
		t.Errorf("should be OK, errors: %v", result.Errors)
	}
}

func TestRunPreflight_MidInference(t *testing.T) {
	state := &CompactionState{SessionName: "test"}
	result := RunPreflight(manifest.StateWorking, state, false)
	if result.OK {
		t.Error("should not be OK when WORKING")
	}
	if len(result.Errors) == 0 {
		t.Error("should have errors")
	}
}

func TestRunPreflight_AlreadyCompacting(t *testing.T) {
	state := &CompactionState{SessionName: "test"}
	result := RunPreflight(manifest.StateCompacting, state, false)
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
	result := RunPreflight(manifest.StateDone, state, false)
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
	result := RunPreflight(manifest.StateDone, state, true)
	if !result.OK {
		t.Errorf("should be OK with force, errors: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Error("should have warnings when force-bypassing")
	}
}
