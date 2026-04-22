package ops

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// --- computeSessionStatus tests ---

func TestComputeSessionStatus_Archived(t *testing.T) {
	m := newManifest("cs-1", "archived", "~/project")
	m.Lifecycle = manifest.LifecycleArchived

	status := computeSessionStatus(m, newMockTmux("archived"))
	if status != "archived" {
		t.Errorf("expected 'archived', got %s", status)
	}
}

func TestComputeSessionStatus_NilTmux(t *testing.T) {
	m := newManifest("cs-2", "no-tmux", "~/project")
	status := computeSessionStatus(m, nil)
	if status != "unknown" {
		t.Errorf("expected 'unknown' with nil tmux, got %s", status)
	}
}

func TestComputeSessionStatus_TmuxRunning(t *testing.T) {
	m := newManifest("cs-3", "running", "~/project")
	status := computeSessionStatus(m, newMockTmux("running"))
	if status != "active" {
		t.Errorf("expected 'active', got %s", status)
	}
}

func TestComputeSessionStatus_TmuxStopped(t *testing.T) {
	m := newManifest("cs-4", "stopped", "~/project")
	status := computeSessionStatus(m, newMockTmux()) // no sessions
	if status != "stopped" {
		t.Errorf("expected 'stopped', got %s", status)
	}
}

func TestComputeSessionStatus_CustomTmuxName(t *testing.T) {
	m := newManifest("cs-5", "session-name", "~/project")
	m.Tmux.SessionName = "custom-tmux"
	status := computeSessionStatus(m, newMockTmux("custom-tmux"))
	if status != "active" {
		t.Errorf("expected 'active' via custom tmux name, got %s", status)
	}
}

func TestComputeSessionStatus_CustomTmuxName_NotRunning(t *testing.T) {
	m := newManifest("cs-6", "session-name", "~/project")
	m.Tmux.SessionName = "custom-tmux"
	status := computeSessionStatus(m, newMockTmux("session-name")) // has session-name but not custom-tmux
	if status != "stopped" {
		t.Errorf("expected 'stopped' (custom tmux name not found), got %s", status)
	}
}

func TestComputeSessionStatus_IncompatibleTmux(t *testing.T) {
	m := newManifest("cs-7", "session", "~/project")
	// Pass a type that doesn't implement HasSession
	status := computeSessionStatus(m, "not-a-tmux-interface")
	if status != "unknown" {
		t.Errorf("expected 'unknown' for incompatible tmux type, got %s", status)
	}
}

// --- toSessionDetail tests ---

func TestToSessionDetail_BasicFields(t *testing.T) {
	m := newManifest("td-1", "detail-test", "~/myproject")
	m.Harness = "claude-code"
	m.Model = "claude-opus-4-6"
	m.Workspace = "/ws/oss"
	m.WorkingDirectory = "~/myproject/src"
	m.Context.Purpose = "testing"
	m.Context.Tags = []string{"test", "unit"}
	m.PermissionMode = "auto-approve"

	detail := toSessionDetail(m, "active")

	if detail.ID != "td-1" {
		t.Errorf("expected ID td-1, got %s", detail.ID)
	}
	if detail.Name != "detail-test" {
		t.Errorf("expected name detail-test, got %s", detail.Name)
	}
	if detail.Status != "active" {
		t.Errorf("expected status active, got %s", detail.Status)
	}
	if detail.Harness != "claude-code" {
		t.Errorf("expected harness claude-code, got %s", detail.Harness)
	}
	if detail.Model != "claude-opus-4-6" {
		t.Errorf("expected model claude-opus-4-6, got %s", detail.Model)
	}
	if detail.Project != "~/myproject" {
		t.Errorf("expected project ~/myproject, got %s", detail.Project)
	}
	if detail.Purpose != "testing" {
		t.Errorf("expected purpose 'testing', got %s", detail.Purpose)
	}
	if detail.Workspace != "/ws/oss" {
		t.Errorf("expected workspace /ws/oss, got %s", detail.Workspace)
	}
	if detail.WorkingDirectory != "~/myproject/src" {
		t.Errorf("expected working dir ~/myproject/src, got %s", detail.WorkingDirectory)
	}
	if detail.PermissionMode != "auto-approve" {
		t.Errorf("expected permission mode auto-approve, got %s", detail.PermissionMode)
	}
	if len(detail.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(detail.Tags))
	}
}

func TestToSessionDetail_ParentSessionID(t *testing.T) {
	m := newManifest("td-2", "child-session", "~/project")
	parentID := "parent-123"
	m.ParentSessionID = &parentID

	detail := toSessionDetail(m, "active")
	if detail.ParentSessionID != "parent-123" {
		t.Errorf("expected parent session ID parent-123, got %s", detail.ParentSessionID)
	}
}

func TestToSessionDetail_NoParentSessionID(t *testing.T) {
	m := newManifest("td-3", "root-session", "~/project")
	m.ParentSessionID = nil

	detail := toSessionDetail(m, "active")
	if detail.ParentSessionID != "" {
		t.Errorf("expected empty parent session ID, got %s", detail.ParentSessionID)
	}
}

func TestToSessionDetail_WithContextUsage(t *testing.T) {
	m := newManifest("td-4", "usage-session", "~/project")
	m.ContextUsage = &manifest.ContextUsage{
		TotalTokens:    200000,
		UsedTokens:     50000,
		PercentageUsed: 25.0,
	}

	detail := toSessionDetail(m, "active")
	if detail.ContextUsage == nil {
		t.Fatal("expected non-nil ContextUsage")
	}
	if detail.ContextUsage.TotalTokens != 200000 {
		t.Errorf("expected 200000 total tokens, got %d", detail.ContextUsage.TotalTokens)
	}
	if detail.ContextUsage.UsedTokens != 50000 {
		t.Errorf("expected 50000 used tokens, got %d", detail.ContextUsage.UsedTokens)
	}
	if detail.ContextUsage.PercentageUsed != 25.0 {
		t.Errorf("expected 25.0%% used, got %f", detail.ContextUsage.PercentageUsed)
	}
}

func TestToSessionDetail_NilContextUsage(t *testing.T) {
	m := newManifest("td-5", "no-usage", "~/project")
	m.ContextUsage = nil

	detail := toSessionDetail(m, "stopped")
	if detail.ContextUsage != nil {
		t.Error("expected nil ContextUsage")
	}
}

func TestToSessionDetail_DateFormatting(t *testing.T) {
	m := newManifest("td-6", "date-session", "~/project")
	m.CreatedAt = time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC)
	m.UpdatedAt = time.Date(2026, 4, 10, 15, 45, 0, 0, time.UTC)

	detail := toSessionDetail(m, "active")
	if detail.CreatedAt != "2026-04-10T14:30:00Z" {
		t.Errorf("expected 2026-04-10T14:30:00Z, got %s", detail.CreatedAt)
	}
	if detail.UpdatedAt != "2026-04-10T15:45:00Z" {
		t.Errorf("expected 2026-04-10T15:45:00Z, got %s", detail.UpdatedAt)
	}
}

func TestToSessionDetail_Lifecycle(t *testing.T) {
	m := newManifest("td-7", "lifecycle-session", "~/project")
	m.Lifecycle = manifest.LifecycleArchived

	detail := toSessionDetail(m, "archived")
	if detail.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("expected lifecycle 'archived', got %s", detail.Lifecycle)
	}
}
