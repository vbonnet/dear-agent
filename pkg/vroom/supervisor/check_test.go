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
