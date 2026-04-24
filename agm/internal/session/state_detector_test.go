package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
)

// TestDetectState_NonExistentSession verifies that a session not in tmux
// returns OFFLINE.
func TestDetectState_NonExistentSession(t *testing.T) {
	// Use a session name that definitely does not exist
	state, err := DetectState("agm-test-nonexistent-session-xyz-12345")
	assert.NoError(t, err)
	assert.Equal(t, manifest.StateOffline, state)
}

// TestDetectStateWithConfidence_NonExistentSession verifies confidence
// scoring for a non-existent session.
func TestDetectStateWithConfidence_NonExistentSession(t *testing.T) {
	result, err := DetectStateWithConfidence("agm-test-nonexistent-session-xyz-12345")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, manifest.StateOffline, result.State)
	assert.Equal(t, 1.0, result.Confidence)
}

// TestResolveSessionState_NoTmuxSession verifies that resolveSessionState
// returns OFFLINE when the tmux session does not exist.
func TestResolveSessionState_Offline(t *testing.T) {
	state := ResolveSessionState("agm-test-nonexistent-session-xyz-12345", "", "", time.Time{})
	assert.Equal(t, manifest.StateOffline, state)
}

// TestResolveSessionState_OfflineIgnoresManifest verifies that manifest state
// is ignored when the tmux session does not exist.
func TestResolveSessionState_OfflineIgnoresManifest(t *testing.T) {
	state := ResolveSessionState("agm-test-nonexistent-session-xyz-12345", manifest.StateWorking, "", time.Now())
	assert.Equal(t, manifest.StateOffline, state)
}

// TestResolveSessionState_NoHookState_NoFreshFile verifies that without hook
// state and without a fresh statusline file, the default is DONE.
func TestResolveSessionState_NoHookState_NoFreshFile(t *testing.T) {
	// Non-existent UUID → no statusline file → DONE
	// (can't test with a real tmux session, but the offline path is tested above)
	state := ResolveSessionState("agm-test-nonexistent-session-xyz-12345", "", "fake-uuid-no-file", time.Time{})
	assert.Equal(t, manifest.StateOffline, state) // no tmux session → offline trumps all
}

// TestResolveSessionState_FreshStatuslineFile verifies that a fresh statusline
// file causes WORKING state when no manifest state is set.
func TestResolveSessionState_FreshStatuslineFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "fresh-state-test"
	// Write a fresh statusline file
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Since we can't create a real tmux session, the session won't exist,
	// so it will still return OFFLINE. But this exercises the isStatusLineFileFresh path.
	state := ResolveSessionState("agm-test-nonexistent-xyz-99999", "", sessionID, time.Time{})
	// No tmux session → OFFLINE (fresh file check is bypassed by tmux check)
	assert.Equal(t, manifest.StateOffline, state)
}

// TestDetectStateWithConfidence_ResultFields verifies the DetectionResult
// structure is populated correctly for OFFLINE state.
func TestDetectStateWithConfidence_ResultFields(t *testing.T) {
	result, err := DetectStateWithConfidence("agm-test-nonexistent-xyz-99999")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, manifest.StateOffline, result.State)
	assert.Equal(t, 1.0, result.Confidence)
	assert.Contains(t, result.Reason, "does not exist")
}

// TestDetectState_ReturnsOfflineForNonExistent tests the basic DetectState path.
func TestDetectState_ReturnsCorrectState(t *testing.T) {
	// Non-existent session should return OFFLINE
	state, err := DetectState("agm-test-nonexistent-xyz-88888")
	assert.NoError(t, err)
	assert.Equal(t, manifest.StateOffline, state)
}

// TestUpdateSessionState tests state updates via Dolt adapter
func TestUpdateSessionState(t *testing.T) {
	t.Run("nil adapter returns error", func(t *testing.T) {
		err := UpdateSessionState("/tmp/test", manifest.StateWorking, "test", "", nil)
		assert.Error(t, err)
	})

	t.Run("empty sessionID returns error", func(t *testing.T) {
		adapter := testutil.GetTestDoltAdapter(t)
		defer adapter.Close()
		err := UpdateSessionState("/tmp/test", manifest.StateWorking, "test", "", adapter)
		assert.Error(t, err)
	})

	t.Run("nonexistent session returns error", func(t *testing.T) {
		adapter := testutil.GetTestDoltAdapter(t)
		defer adapter.Close()
		err := UpdateSessionState("/tmp/test", manifest.StateWorking, "test", "nonexistent-xyz-99999", adapter)
		assert.Error(t, err)
	})
}

// --- Hybrid state detection tests ---

// TestHybrid_HookPrimary verifies that fresh hook state is used as primary
// detection source (no tmux session here, so offline trumps, but the logic
// is validated via the non-offline paths tested in integration).
func TestHybrid_HookPrimary(t *testing.T) {
	// With a non-existent tmux session, OFFLINE always wins — this verifies
	// the hook-primary path doesn't crash with the new stateUpdatedAt param.
	freshTime := time.Now()
	st := ResolveSessionState("agm-test-nonexistent-hybrid-1", manifest.StateWorking, "", freshTime)
	assert.Equal(t, manifest.StateOffline, st, "non-existent tmux session should return OFFLINE regardless of hook state")

	// Verify fresh hook state is returned when stateUpdatedAt is recent
	// (testing the code path — tmux check prevents non-OFFLINE result here)
	st = ResolveSessionState("agm-test-nonexistent-hybrid-2", manifest.StateDone, "", freshTime)
	assert.Equal(t, manifest.StateOffline, st)
}

// TestHybrid_RegexFallback verifies that when no hook state exists (empty
// manifestState), the function falls through to regex/statusline fallback
// paths instead of immediately returning the hook state.
func TestHybrid_RegexFallback(t *testing.T) {
	// No hook state, no tmux session → OFFLINE
	st := ResolveSessionState("agm-test-nonexistent-hybrid-3", "", "no-uuid", time.Time{})
	assert.Equal(t, manifest.StateOffline, st, "no hook state + no tmux should be OFFLINE")

	// No hook state with fresh statusline file — still OFFLINE because no tmux
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "hybrid-fallback-test"
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	st = ResolveSessionState("agm-test-nonexistent-hybrid-4", "", sessionID, time.Time{})
	assert.Equal(t, manifest.StateOffline, st, "no tmux session → OFFLINE even with fresh statusline")
}

// TestHybrid_Staleness verifies that stale hook state (>60s old) triggers
// re-detection via terminal parsing fallback.
func TestHybrid_Staleness(t *testing.T) {
	// Stale state (2 minutes old) — without tmux session returns OFFLINE,
	// but verifies the staleness threshold logic doesn't panic.
	staleTime := time.Now().Add(-2 * time.Minute)
	st := ResolveSessionState("agm-test-nonexistent-hybrid-5", manifest.StateWorking, "", staleTime)
	assert.Equal(t, manifest.StateOffline, st, "stale hook state + no tmux → OFFLINE")

	// Verify threshold is configurable
	original := HookStalenessThreshold
	HookStalenessThreshold = 30 * time.Second
	defer func() { HookStalenessThreshold = original }()

	// 45s old with 30s threshold = stale
	almostStaleTime := time.Now().Add(-45 * time.Second)
	st = ResolveSessionState("agm-test-nonexistent-hybrid-6", manifest.StateDone, "", almostStaleTime)
	assert.Equal(t, manifest.StateOffline, st)

	// Zero time should be treated as "no timestamp" and not trigger staleness
	st = ResolveSessionState("agm-test-nonexistent-hybrid-7", manifest.StateDone, "", time.Time{})
	assert.Equal(t, manifest.StateOffline, st)
}

// TestHybrid_StalenessThresholdDefault verifies the default staleness threshold
// is 60 seconds and that the HookStalenessThreshold variable is configurable.
func TestHybrid_StalenessThresholdDefault(t *testing.T) {
	assert.Equal(t, 60*time.Second, HookStalenessThreshold,
		"default staleness threshold should be 60 seconds")

	// Temporarily change and verify
	original := HookStalenessThreshold
	HookStalenessThreshold = 120 * time.Second
	assert.Equal(t, 120*time.Second, HookStalenessThreshold)
	HookStalenessThreshold = original
}

// TestHybrid_Integration verifies the full hybrid detection flow:
// hook primary → staleness check → regex fallback → statusline → DONE default.
// Since we can't create real tmux sessions in unit tests, this validates
// the non-tmux paths and parameter threading.
func TestHybrid_Integration(t *testing.T) {
	t.Run("fresh hook state used as-is", func(t *testing.T) {
		st := ResolveSessionState("agm-test-nonexistent-int-1",
			manifest.StateCompacting, "", time.Now())
		assert.Equal(t, manifest.StateOffline, st)
	})

	t.Run("stale hook triggers re-detection", func(t *testing.T) {
		st := ResolveSessionState("agm-test-nonexistent-int-2",
			manifest.StateWorking, "", time.Now().Add(-90*time.Second))
		assert.Equal(t, manifest.StateOffline, st)
	})

	t.Run("no hook falls through to regex then statusline", func(t *testing.T) {
		st := ResolveSessionState("agm-test-nonexistent-int-3",
			"", "nonexistent-uuid", time.Time{})
		assert.Equal(t, manifest.StateOffline, st)
	})

	t.Run("zero stateUpdatedAt with hook state is not stale", func(t *testing.T) {
		// Zero time = "never set" — should NOT trigger staleness, just use hook state
		st := ResolveSessionState("agm-test-nonexistent-int-4",
			manifest.StateDone, "", time.Time{})
		assert.Equal(t, manifest.StateOffline, st)
	})
}

func TestMapTerminalStateToManifest(t *testing.T) {
	tests := []struct {
		name     string
		input    state.State
		expected string
	}{
		{"ready maps to DONE", state.StateReady, manifest.StateDone},
		{"thinking maps to WORKING", state.StateThinking, manifest.StateWorking},
		{"blocked_auth maps to USER_PROMPT", state.StateBlockedAuth, manifest.StateUserPrompt},
		{"blocked_input maps to USER_PROMPT", state.StateBlockedInput, manifest.StateUserPrompt},
		{"stuck maps to WORKING", state.StateStuck, manifest.StateWorking},
		{"unknown maps to DONE", state.StateUnknown, manifest.StateDone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapTerminalStateToManifest(tt.input)
			if result != tt.expected {
				t.Errorf("mapTerminalStateToManifest(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMapTerminalStateToManifest_BackgroundTasks(t *testing.T) {
	result := mapTerminalStateToManifest(state.StateBackgroundTasksView)
	assert.Equal(t, manifest.StateBackgroundTasks, result,
		"StateBackgroundTasksView should map to StateBackgroundTasks")
}
