package modelcard

import (
	"encoding/json"
	"testing"
)

// --- ModelCard ---

func TestNewModelCard_Defaults(t *testing.T) {
	mc := NewModelCard("agent-1", "Test Agent", "worker")
	if mc.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", mc.AgentID)
	}
	if mc.Status != StatusActive {
		t.Errorf("Status = %q, want active", mc.Status)
	}
	if mc.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
	if len(mc.InputModes) == 0 {
		t.Error("InputModes should be non-empty")
	}
	if mc.Tags == nil {
		t.Error("Tags map should be initialised")
	}
}

func TestModelCard_HasCapability(t *testing.T) {
	mc := NewModelCard("a", "Test", "worker")
	mc.Capabilities = []string{"code-review", "testing"}
	if !mc.HasCapability("code-review") {
		t.Error("should have code-review capability")
	}
	if !mc.HasCapability("testing") {
		t.Error("should have testing capability")
	}
	if mc.HasCapability("refactoring") {
		t.Error("should not have refactoring capability")
	}
}

func TestModelCard_HasRole(t *testing.T) {
	mc := NewModelCard("a", "Test", "orchestrator")
	if !mc.HasRole("orchestrator") {
		t.Error("should have orchestrator role")
	}
	if mc.HasRole("worker") {
		t.Error("should not have worker role")
	}
}

func TestModelCard_MarshalJSON_RoundTrip(t *testing.T) {
	mc := NewModelCard("agent-x", "X Agent", "researcher")
	mc.Capabilities = []string{"search"}
	mc.Tags["env"] = "prod"

	data, err := mc.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded ModelCard
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.AgentID != "agent-x" {
		t.Errorf("decoded AgentID = %q, want agent-x", decoded.AgentID)
	}
	if decoded.Tags["env"] != "prod" {
		t.Errorf("decoded Tags[env] = %q, want prod", decoded.Tags["env"])
	}
}

func TestModelCard_SetStatus(t *testing.T) {
	mc := NewModelCard("a", "Test", "worker")
	before := mc.UpdatedAt
	mc.SetStatus(StatusBusy)
	if mc.Status != StatusBusy {
		t.Errorf("Status = %q, want busy", mc.Status)
	}
	if !mc.UpdatedAt.After(before) {
		t.Error("UpdatedAt should advance after SetStatus")
	}
}

// --- Registry ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mc := NewModelCard("agent-1", "Agent One", "worker")
	if err := r.Register(mc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("agent-1")
	if !ok {
		t.Fatal("Get returned not-found for registered agent")
	}
	if got.Name != "Agent One" {
		t.Errorf("Name = %q, want Agent One", got.Name)
	}
}

func TestRegistry_Register_RequiresAgentID(t *testing.T) {
	r := NewRegistry()
	mc := &ModelCard{Name: "No ID", Role: "worker"}
	if err := r.Register(mc); err == nil {
		t.Error("expected error when AgentID is empty")
	}
}

func TestRegistry_Register_RequiresName(t *testing.T) {
	r := NewRegistry()
	mc := &ModelCard{AgentID: "x", Role: "worker"}
	if err := r.Register(mc); err == nil {
		t.Error("expected error when Name is empty")
	}
}

func TestRegistry_Register_RequiresRole(t *testing.T) {
	r := NewRegistry()
	mc := &ModelCard{AgentID: "x", Name: "X"}
	if err := r.Register(mc); err == nil {
		t.Error("expected error when Role is empty")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	mc := NewModelCard("a", "A", "worker")
	_ = r.Register(mc)
	r.Unregister("a")
	_, ok := r.Get("a")
	if ok {
		t.Error("Get should return not-found after Unregister")
	}
}

func TestRegistry_FindByRole(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewModelCard("w1", "Worker 1", "worker"))
	_ = r.Register(NewModelCard("w2", "Worker 2", "worker"))
	_ = r.Register(NewModelCard("o1", "Orchestrator 1", "orchestrator"))

	workers := r.FindByRole("worker")
	if len(workers) != 2 {
		t.Errorf("want 2 workers, got %d", len(workers))
	}
	orchestrators := r.FindByRole("orchestrator")
	if len(orchestrators) != 1 {
		t.Errorf("want 1 orchestrator, got %d", len(orchestrators))
	}
}

func TestRegistry_FindByCapability(t *testing.T) {
	r := NewRegistry()
	mc1 := NewModelCard("a", "A", "worker")
	mc1.Capabilities = []string{"code-review", "testing"}
	mc2 := NewModelCard("b", "B", "researcher")
	mc2.Capabilities = []string{"search"}
	_ = r.Register(mc1)
	_ = r.Register(mc2)

	reviewers := r.FindByCapability("code-review")
	if len(reviewers) != 1 {
		t.Errorf("want 1 code-review agent, got %d", len(reviewers))
	}
	none := r.FindByCapability("nonexistent")
	if len(none) != 0 {
		t.Errorf("want 0 results for nonexistent capability, got %d", len(none))
	}
}

func TestRegistry_ActiveAgents(t *testing.T) {
	r := NewRegistry()
	mc1 := NewModelCard("a", "A", "worker")
	mc2 := NewModelCard("b", "B", "worker")
	mc3 := NewModelCard("c", "C", "worker")
	mc2.Status = StatusIdle
	mc3.Status = StatusBusy
	_ = r.Register(mc1) // active
	_ = r.Register(mc2) // idle
	_ = r.Register(mc3) // busy

	active := r.ActiveAgents()
	if len(active) != 2 {
		t.Errorf("want 2 active/busy agents, got %d", len(active))
	}
}

func TestRegistry_UpdateStatus(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewModelCard("a", "A", "worker"))
	if err := r.UpdateStatus("a", StatusOffline); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := r.Get("a")
	if got.Status != StatusOffline {
		t.Errorf("Status = %q, want offline", got.Status)
	}
}

func TestRegistry_UpdateStatus_NotFound(t *testing.T) {
	r := NewRegistry()
	if err := r.UpdateStatus("nonexistent", StatusBusy); err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewModelCard("a", "A", "worker"))
	_ = r.Register(NewModelCard("b", "B", "researcher"))
	all := r.All()
	if len(all) != 2 {
		t.Errorf("want 2 cards, got %d", len(all))
	}
}
