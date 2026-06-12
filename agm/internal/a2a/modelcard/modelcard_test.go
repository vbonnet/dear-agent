package modelcard

import (
	"testing"
	"time"
)

func TestNewModelCard_Defaults(t *testing.T) {
	t.Parallel()
	before := time.Now()
	mc := NewModelCard("agent-1", "MyAgent", "worker")
	after := time.Now()

	if mc.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", mc.AgentID)
	}
	if mc.Status != StatusActive {
		t.Errorf("Status = %q, want active", mc.Status)
	}
	if mc.UpdatedAt.Before(before) || mc.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v outside [%v, %v]", mc.UpdatedAt, before, after)
	}
	if len(mc.InputModes) != 1 || mc.InputModes[0] != "text/plain" {
		t.Errorf("InputModes = %v, want [text/plain]", mc.InputModes)
	}
}

func TestHasCapability(t *testing.T) {
	t.Parallel()
	mc := NewModelCard("a", "B", "worker")
	mc.Capabilities = []string{"code-review", "testing"}

	if !mc.HasCapability("code-review") {
		t.Error("HasCapability(code-review) = false, want true")
	}
	if mc.HasCapability("deploy") {
		t.Error("HasCapability(deploy) = true, want false")
	}
	if mc.HasCapability("") {
		t.Error("HasCapability(\"\") = true, want false")
	}
}

func TestHasRole(t *testing.T) {
	t.Parallel()
	mc := NewModelCard("a", "B", "orchestrator")
	if !mc.HasRole("orchestrator") {
		t.Error("HasRole(orchestrator) = false, want true")
	}
	if mc.HasRole("worker") {
		t.Error("HasRole(worker) = true, want false")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	mc := NewModelCard("ag-1", "Agent One", "worker")

	if err := r.Register(mc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Get("ag-1")
	if !ok {
		t.Fatal("Get(ag-1) not found after Register")
	}
	if got.Name != "Agent One" {
		t.Errorf("Name = %q, want Agent One", got.Name)
	}
}

func TestRegistry_Register_ValidationErrors(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	cases := []struct {
		name string
		card *ModelCard
	}{
		{"empty AgentID", &ModelCard{Name: "N", Role: "worker"}},
		{"empty Name", &ModelCard{AgentID: "id", Role: "worker"}},
		{"empty Role", &ModelCard{AgentID: "id", Name: "N"}},
	}
	for _, tc := range cases {
		if err := r.Register(tc.card); err == nil {
			t.Errorf("%s: Register should return error, got nil", tc.name)
		}
	}
}

func TestRegistry_Unregister(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	mc := NewModelCard("ag-2", "Agent Two", "reviewer")
	_ = r.Register(mc)

	r.Unregister("ag-2")
	if _, ok := r.Get("ag-2"); ok {
		t.Error("Get(ag-2) still found after Unregister")
	}
}

func TestRegistry_FindByRole(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(NewModelCard("w1", "Worker 1", "worker"))
	_ = r.Register(NewModelCard("w2", "Worker 2", "worker"))
	_ = r.Register(NewModelCard("o1", "Orchestrator 1", "orchestrator"))

	workers := r.FindByRole("worker")
	if len(workers) != 2 {
		t.Errorf("FindByRole(worker) = %d, want 2", len(workers))
	}
	orchestrators := r.FindByRole("orchestrator")
	if len(orchestrators) != 1 {
		t.Errorf("FindByRole(orchestrator) = %d, want 1", len(orchestrators))
	}
	if len(r.FindByRole("nonexistent")) != 0 {
		t.Error("FindByRole(nonexistent) should be empty")
	}
}

func TestRegistry_UpdateStatus(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(NewModelCard("ag-3", "Agent Three", "worker"))

	if err := r.UpdateStatus("ag-3", StatusBusy); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := r.Get("ag-3")
	if got.Status != StatusBusy {
		t.Errorf("Status = %q, want busy", got.Status)
	}

	if err := r.UpdateStatus("nonexistent", StatusIdle); err == nil {
		t.Error("UpdateStatus for nonexistent agent should return error")
	}
}

func TestRegistry_ActiveAgents(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	a1 := NewModelCard("a1", "A1", "worker")
	a1.Status = StatusActive
	a2 := NewModelCard("a2", "A2", "worker")
	a2.Status = StatusBusy
	a3 := NewModelCard("a3", "A3", "worker")
	a3.Status = StatusOffline
	_ = r.Register(a1)
	_ = r.Register(a2)
	_ = r.Register(a3)

	active := r.ActiveAgents()
	if len(active) != 2 {
		t.Errorf("ActiveAgents = %d, want 2 (active+busy)", len(active))
	}
}
