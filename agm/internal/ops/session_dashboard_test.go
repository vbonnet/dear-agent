package ops

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestEffectiveState_NilManifest(t *testing.T) {
	tmuxSet := map[string]bool{}
	state := effectiveState(nil, tmuxSet)
	if state != "UNKNOWN" {
		t.Errorf("expected UNKNOWN for nil manifest, got %s", state)
	}
}

func TestEffectiveState_TmuxGone(t *testing.T) {
	m := newManifest("d-1", "session-1", "~/project")
	m.State = manifest.StateWorking
	tmuxSet := map[string]bool{} // session not in tmux

	state := effectiveState(m, tmuxSet)
	if state != manifest.StateOffline {
		t.Errorf("expected OFFLINE when tmux session gone, got %s", state)
	}
}

func TestEffectiveState_TmuxAlive(t *testing.T) {
	m := newManifest("d-2", "session-2", "~/project")
	m.State = manifest.StateWorking
	tmuxSet := map[string]bool{"session-2": true}

	state := effectiveState(m, tmuxSet)
	if state != manifest.StateWorking {
		t.Errorf("expected WORKING, got %s", state)
	}
}

func TestEffectiveState_EmptyState(t *testing.T) {
	m := newManifest("d-3", "session-3", "~/project")
	m.State = ""
	tmuxSet := map[string]bool{"session-3": true}

	state := effectiveState(m, tmuxSet)
	if state != "UNKNOWN" {
		t.Errorf("expected UNKNOWN for empty state, got %s", state)
	}
}

func TestEffectiveState_CustomTmuxName(t *testing.T) {
	m := newManifest("d-4", "session-4", "~/project")
	m.State = manifest.StateDone
	m.Tmux.SessionName = "custom-tmux-name"
	tmuxSet := map[string]bool{"custom-tmux-name": true}

	state := effectiveState(m, tmuxSet)
	if state != manifest.StateDone {
		t.Errorf("expected DONE with custom tmux name, got %s", state)
	}
}

func TestIsTmuxAlive_True(t *testing.T) {
	m := newManifest("d-5", "alive-session", "~/project")
	tmuxSet := map[string]bool{"alive-session": true}

	if !isTmuxAlive(m, tmuxSet) {
		t.Error("expected isTmuxAlive to be true")
	}
}

func TestIsTmuxAlive_False(t *testing.T) {
	m := newManifest("d-6", "dead-session", "~/project")
	tmuxSet := map[string]bool{}

	if isTmuxAlive(m, tmuxSet) {
		t.Error("expected isTmuxAlive to be false")
	}
}

func TestIsTmuxAlive_Nil(t *testing.T) {
	tmuxSet := map[string]bool{"anything": true}
	if isTmuxAlive(nil, tmuxSet) {
		t.Error("expected isTmuxAlive to be false for nil manifest")
	}
}

func TestPermMode_Default(t *testing.T) {
	m := newManifest("d-7", "pm-session", "~/project")
	m.PermissionMode = ""
	if got := permMode(m); got != "default" {
		t.Errorf("expected 'default', got %s", got)
	}
}

func TestPermMode_Set(t *testing.T) {
	m := newManifest("d-8", "pm-session", "~/project")
	m.PermissionMode = "dangerously-skip-permissions"
	if got := permMode(m); got != "dangerously-skip-permissions" {
		t.Errorf("expected 'dangerously-skip-permissions', got %s", got)
	}
}

func TestPermMode_Nil(t *testing.T) {
	if got := permMode(nil); got != "default" {
		t.Errorf("expected 'default' for nil manifest, got %s", got)
	}
}

func TestModel_Fallback(t *testing.T) {
	m := newManifest("d-9", "model-session", "~/project")
	m.Model = "claude-opus-4-6"
	m.LastKnownModel = ""
	if got := model(m); got != "claude-opus-4-6" {
		t.Errorf("expected claude-opus-4-6, got %s", got)
	}
}

func TestModel_LastKnownPreferred(t *testing.T) {
	m := newManifest("d-10", "model-session", "~/project")
	m.Model = "claude-opus-4-6"
	m.LastKnownModel = "claude-sonnet-4-6"
	if got := model(m); got != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6 (LastKnownModel), got %s", got)
	}
}

func TestModel_Nil(t *testing.T) {
	if got := model(nil); got != "" {
		t.Errorf("expected empty string for nil manifest, got %s", got)
	}
}

func TestTimeInState_Nil(t *testing.T) {
	if got := timeInState(nil); got != "-" {
		t.Errorf("expected '-' for nil manifest, got %s", got)
	}
}

func TestTimeInState_ZeroTime(t *testing.T) {
	m := newManifest("d-11", "time-session", "~/project")
	m.StateUpdatedAt = time.Time{}
	if got := timeInState(m); got != "-" {
		t.Errorf("expected '-' for zero time, got %s", got)
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	if got := formatDuration(30 * time.Second); got != "30s" {
		t.Errorf("expected '30s', got %s", got)
	}
}

func TestFormatDuration_Minutes(t *testing.T) {
	if got := formatDuration(5 * time.Minute); got != "5m" {
		t.Errorf("expected '5m', got %s", got)
	}
}

func TestFormatDuration_Hours(t *testing.T) {
	if got := formatDuration(2 * time.Hour); got != "2h" {
		t.Errorf("expected '2h', got %s", got)
	}
}

func TestFormatDuration_HoursAndMinutes(t *testing.T) {
	if got := formatDuration(2*time.Hour + 30*time.Minute); got != "2h30m" {
		t.Errorf("expected '2h30m', got %s", got)
	}
}

func TestFormatDuration_Days(t *testing.T) {
	if got := formatDuration(48 * time.Hour); got != "2d" {
		t.Errorf("expected '2d', got %s", got)
	}
}

func TestBuildDashboardFilter_Default(t *testing.T) {
	req := &DashboardRequest{}
	f := buildDashboardFilter(req)
	if f.Limit != 1000 {
		t.Errorf("expected limit 1000, got %d", f.Limit)
	}
	if !f.ExcludeArchived {
		t.Error("expected ExcludeArchived to be true by default")
	}
}

func TestBuildDashboardFilter_IncludeArchived(t *testing.T) {
	req := &DashboardRequest{IncludeArchived: true}
	f := buildDashboardFilter(req)
	if f.ExcludeArchived {
		t.Error("expected ExcludeArchived to be false when IncludeArchived is set")
	}
}

func TestDashboard_Empty(t *testing.T) {
	ctx := testCtx(nil)
	result, err := Dashboard(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operation != "dashboard" {
		t.Errorf("expected operation 'dashboard', got %s", result.Operation)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 entries, got %d", result.Total)
	}
	if result.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestDashboard_WithSessions(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("d-20", "worker-1", "~/project"),
		newManifest("d-21", "worker-2", "~/project"),
	}
	sessions[0].State = manifest.StateWorking
	sessions[1].State = manifest.StateDone

	ctx := testCtx(sessions, "worker-1") // only worker-1 in tmux

	result, err := Dashboard(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("expected 2 entries, got %d", result.Total)
	}

	for _, e := range result.Entries {
		switch e.Name {
		case "worker-1":
			if !e.TmuxAlive {
				t.Error("worker-1 should have TmuxAlive=true")
			}
		case "worker-2":
			if e.TmuxAlive {
				t.Error("worker-2 should have TmuxAlive=false")
			}
			if e.State != manifest.StateOffline {
				t.Errorf("worker-2 should be OFFLINE (no tmux), got %s", e.State)
			}
		}
	}
}
