package modelcard

import (
	"testing"
)

func TestNewModelCard_FieldsSet(t *testing.T) {
	mc := NewModelCard("agent-1", "Planner", "orchestrator")
	if mc.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", mc.AgentID, "agent-1")
	}
	if mc.Name != "Planner" {
		t.Errorf("Name = %q, want %q", mc.Name, "Planner")
	}
	if mc.Role != "orchestrator" {
		t.Errorf("Role = %q, want %q", mc.Role, "orchestrator")
	}
}

func TestSetStatus_Updates(t *testing.T) {
	mc := NewModelCard("a", "n", "r")
	mc.SetStatus(StatusActive)
	if mc.Status != StatusActive {
		t.Errorf("Status = %q, want %q", mc.Status, StatusActive)
	}
}

func TestHasCapability_Found(t *testing.T) {
	mc := NewModelCard("a", "n", "r")
	mc.Capabilities = []string{"read", "write"}
	if !mc.HasCapability("read") {
		t.Error("HasCapability should find 'read'")
	}
}

func TestHasCapability_NotFound(t *testing.T) {
	mc := NewModelCard("a", "n", "r")
	mc.Capabilities = []string{"read"}
	if mc.HasCapability("delete") {
		t.Error("HasCapability should not find 'delete'")
	}
}

func TestHasRole(t *testing.T) {
	mc := NewModelCard("a", "n", "worker")
	if !mc.HasRole("worker") {
		t.Error("HasRole should match own role")
	}
	if mc.HasRole("orchestrator") {
		t.Error("HasRole should not match different role")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mc := NewModelCard("a1", "Agent1", "worker")
	if err := r.Register(mc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("a1")
	if !ok {
		t.Fatal("Get returned false after Register")
	}
	if got.Name != "Agent1" {
		t.Errorf("Get name = %q, want %q", got.Name, "Agent1")
	}
}

func TestRegistry_DuplicateRegister_Overwrites(t *testing.T) {
	r := NewRegistry()
	mc := NewModelCard("dup", "D", "r")
	if err := r.Register(mc); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	mc2 := NewModelCard("dup", "D-updated", "r")
	if err := r.Register(mc2); err != nil {
		t.Fatalf("duplicate Register should overwrite, got error: %v", err)
	}
	got, _ := r.Get("dup")
	if got.Name != "D-updated" {
		t.Errorf("after overwrite Name = %q, want %q", got.Name, "D-updated")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	mc := NewModelCard("a2", "A2", "r")
	_ = r.Register(mc)
	r.Unregister("a2")
	if _, ok := r.Get("a2"); ok {
		t.Error("Get should return false after Unregister")
	}
}

func TestRegistry_FindByRole(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewModelCard("w1", "W1", "worker"))
	_ = r.Register(NewModelCard("w2", "W2", "worker"))
	_ = r.Register(NewModelCard("o1", "O1", "orchestrator"))
	workers := r.FindByRole("worker")
	if len(workers) != 2 {
		t.Errorf("FindByRole(worker) = %d, want 2", len(workers))
	}
}

func TestRegistry_ActiveAgents(t *testing.T) {
	r := NewRegistry()
	mc1 := NewModelCard("a", "A", "r") // starts StatusActive by default
	mc2 := NewModelCard("b", "B", "r")
	_ = r.Register(mc1)
	_ = r.Register(mc2)
	// Set mc2 to idle so only mc1 is active
	mc2.SetStatus(StatusIdle)
	_ = r.Register(mc2) // re-register to persist the status change

	active := r.ActiveAgents()
	if len(active) != 1 {
		t.Errorf("ActiveAgents = %d, want 1", len(active))
	}
}
