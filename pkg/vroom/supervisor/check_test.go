package supervisor

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHeartbeatCheckSkill_FreshPeerOK(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	h := &HeartbeatCheckSkill{
		Threshold: 5 * time.Second,
		Now:       func() time.Time { return now },
	}
	peer := &fakePeerStatus{role: RoleOverseer, beat: now.Add(-2 * time.Second)}
	if err := h.Check(context.Background(), peer); err != nil {
		t.Errorf("fresh peer check returned err = %v, want nil", err)
	}
}

func TestHeartbeatCheckSkill_StalePeerBlocked(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	h := &HeartbeatCheckSkill{
		Threshold: 5 * time.Second,
		Now:       func() time.Time { return now },
	}
	peer := &fakePeerStatus{role: RoleOverseer, beat: now.Add(-60 * time.Second)}
	err := h.Check(context.Background(), peer)
	if err == nil {
		t.Fatal("stale peer check returned nil error")
	}
	if !strings.Contains(err.Error(), "overseer") || !strings.Contains(err.Error(), "threshold") {
		t.Errorf("error message %q missing expected peer/threshold info", err)
	}
}

func TestHeartbeatCheckSkill_ZeroHeartbeatBlocked(t *testing.T) {
	h := &HeartbeatCheckSkill{Threshold: 5 * time.Second}
	peer := &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Time{}}
	if err := h.Check(context.Background(), peer); err == nil {
		t.Error("zero-heartbeat peer returned nil error — startup case not handled")
	}
}

func TestHeartbeatCheckSkill_ZeroThresholdRejected(t *testing.T) {
	h := &HeartbeatCheckSkill{Threshold: 0}
	peer := &fakePeerStatus{role: RoleOverseer, beat: time.Now()}
	if err := h.Check(context.Background(), peer); err == nil {
		t.Error("Check with zero Threshold returned nil — silent misconfiguration")
	}
}

func TestCheckSkillFunc_Adapter(t *testing.T) {
	called := false
	var skill CheckSkill = CheckSkillFunc(func(context.Context, LoopStatus) error {
		called = true
		return nil
	})
	_ = skill.Check(context.Background(), &fakePeerStatus{role: RoleOrchestrator, beat: time.Now()})
	if !called {
		t.Error("CheckSkillFunc adapter did not invoke wrapped function")
	}
}

// --- ProcessLivenessCheckSkill tests (ce-axsr/ce-qkf7: a fresh heartbeat ---
// --- alone must not prove liveness) ---

func TestProcessLivenessCheckSkill(t *testing.T) {
	now := time.Date(2026, 7, 3, 23, 0, 0, 0, time.UTC)
	freshHeartbeat := &HeartbeatCheckSkill{
		Threshold: time.Minute,
		Now:       func() time.Time { return now },
	}
	freshPeer := &fakePeerStatus{role: RoleMetaOrchestrator, beat: now.Add(-5 * time.Second)}
	stalePeer := &fakePeerStatus{role: RoleMetaOrchestrator, beat: now.Add(-time.Hour)}

	tests := []struct {
		name    string
		skill   *ProcessLivenessCheckSkill
		peer    LoopStatus
		wantErr bool
		wantMsg string // substring; "" means no assertion
	}{
		{
			name: "fresh heartbeat AND live process passes",
			skill: &ProcessLivenessCheckSkill{
				Inner: freshHeartbeat,
				Probe: func(context.Context, LoopStatus) (bool, string, error) {
					return true, "zsh,claude", nil
				},
			},
			peer:    freshPeer,
			wantErr: false,
		},
		{
			name: "fresh heartbeat with DEAD process is blocked (zombie writer)",
			skill: &ProcessLivenessCheckSkill{
				Inner: freshHeartbeat,
				Probe: func(context.Context, LoopStatus) (bool, string, error) {
					return false, "pane tree: zsh,agm", nil
				},
			},
			peer:    freshPeer,
			wantErr: true,
			wantMsg: "does not prove liveness",
		},
		{
			name: "stale heartbeat with live process is still blocked by inner check",
			skill: &ProcessLivenessCheckSkill{
				Inner: freshHeartbeat,
				Probe: func(context.Context, LoopStatus) (bool, string, error) {
					return true, "zsh,claude", nil
				},
			},
			peer:    stalePeer,
			wantErr: true,
			wantMsg: "threshold",
		},
		{
			name: "probe error fails open to the inner check",
			skill: &ProcessLivenessCheckSkill{
				Inner: freshHeartbeat,
				Probe: func(context.Context, LoopStatus) (bool, string, error) {
					return false, "", context.DeadlineExceeded
				},
			},
			peer:    freshPeer,
			wantErr: false,
		},
		{
			name: "nil probe reduces to the inner check",
			skill: &ProcessLivenessCheckSkill{
				Inner: freshHeartbeat,
			},
			peer:    freshPeer,
			wantErr: false,
		},
		{
			name: "nil inner with dead process is still blocked",
			skill: &ProcessLivenessCheckSkill{
				Probe: func(context.Context, LoopStatus) (bool, string, error) {
					return false, "zsh", nil
				},
			},
			peer:    freshPeer,
			wantErr: true,
			wantMsg: "DEAD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.skill.Check(context.Background(), tt.peer)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q missing substring %q", err, tt.wantMsg)
			}
		})
	}
}
