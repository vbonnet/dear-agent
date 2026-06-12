package modelcard_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/modelcard"
)

func TestNewModelCard_Defaults(t *testing.T) {
	t.Parallel()
	mc := modelcard.NewModelCard("agent-1", "Worker", "worker")
	if mc.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", mc.AgentID, "agent-1")
	}
	if mc.Name != "Worker" {
		t.Errorf("Name = %q, want %q", mc.Name, "Worker")
	}
	if mc.Role != "worker" {
		t.Errorf("Role = %q, want %q", mc.Role, "worker")
	}
	if mc.Status != modelcard.StatusActive {
		t.Errorf("Status = %q, want %q", mc.Status, modelcard.StatusActive)
	}
	if mc.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestHasCapability(t *testing.T) {
	t.Parallel()
	mc := modelcard.NewModelCard("x", "X", "worker")
	mc.Capabilities = []string{"code-review", "testing"}
	if !mc.HasCapability("code-review") {
		t.Error("HasCapability(code-review) should be true")
	}
	if mc.HasCapability("nonexistent") {
		t.Error("HasCapability(nonexistent) should be false")
	}
}

func TestHasRole(t *testing.T) {
	t.Parallel()
	mc := modelcard.NewModelCard("x", "X", "orchestrator")
	if !mc.HasRole("orchestrator") {
		t.Error("HasRole(orchestrator) should be true")
	}
	if mc.HasRole("worker") {
		t.Error("HasRole(worker) should be false")
	}
}

func TestSetStatus(t *testing.T) {
	t.Parallel()
	mc := modelcard.NewModelCard("x", "X", "worker")
	mc.SetStatus(modelcard.StatusBusy)
	if mc.Status != modelcard.StatusBusy {
		t.Errorf("Status = %q, want %q", mc.Status, modelcard.StatusBusy)
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := modelcard.NewRegistry()
	mc := modelcard.NewModelCard("a1", "Agent One", "worker")
	if err := r.Register(mc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("a1")
	if !ok {
		t.Fatal("Get returned false for registered agent")
	}
	if got.Name != "Agent One" {
		t.Errorf("Name = %q, want %q", got.Name, "Agent One")
	}
}

func TestRegistry_Register_RejectsEmptyID(t *testing.T) {
	t.Parallel()
	r := modelcard.NewRegistry()
	mc := &modelcard.ModelCard{Name: "X", Role: "worker"} // empty AgentID
	if err := r.Register(mc); err == nil {
		t.Error("expected error for empty AgentID")
	}
}

func TestRegistry_Register_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	r := modelcard.NewRegistry()
	mc := &modelcard.ModelCard{AgentID: "x", Role: "worker"} // empty Name
	if err := r.Register(mc); err == nil {
		t.Error("expected error for empty Name")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	t.Parallel()
	r := modelcard.NewRegistry()
	mc := modelcard.NewModelCard("a1", "X", "worker")
	_ = r.Register(mc)
	r.Unregister("a1")
	if _, ok := r.Get("a1"); ok {
		t.Error("Get should return false after Unregister")
	}
}

func TestRegistry_FindByRole(t *testing.T) {
	t.Parallel()
	r := modelcard.NewRegistry()
	_ = r.Register(modelcard.NewModelCard("w1", "Worker1", "worker"))
	_ = r.Register(modelcard.NewModelCard("w2", "Worker2", "worker"))
	_ = r.Register(modelcard.NewModelCard("o1", "Orch1", "orchestrator"))

	workers := r.FindByRole("worker")
	if len(workers) != 2 {
		t.Errorf("FindByRole(worker) = %d, want 2", len(workers))
	}
	orchs := r.FindByRole("orchestrator")
	if len(orchs) != 1 {
		t.Errorf("FindByRole(orchestrator) = %d, want 1", len(orchs))
	}
}

func TestRegistry_FindByCapability(t *testing.T) {
	t.Parallel()
	r := modelcard.NewRegistry()
	mc1 := modelcard.NewModelCard("a1", "A1", "worker")
	mc1.Capabilities = []string{"testing", "code-review"}
	mc2 := modelcard.NewModelCard("a2", "A2", "worker")
	mc2.Capabilities = []string{"testing"}
	_ = r.Register(mc1)
	_ = r.Register(mc2)

	got := r.FindByCapability("code-review")
	if len(got) != 1 {
		t.Errorf("FindByCapability(code-review) = %d, want 1", len(got))
	}
	all := r.FindByCapability("testing")
	if len(all) != 2 {
		t.Errorf("FindByCapability(testing) = %d, want 2", len(all))
	}
}

func TestStatusConstants(t *testing.T) {
	t.Parallel()
	want := map[modelcard.Status]string{
		modelcard.StatusActive:  "active",
		modelcard.StatusBusy:    "busy",
		modelcard.StatusIdle:    "idle",
		modelcard.StatusOffline: "offline",
	}
	for status, str := range want {
		if string(status) != str {
			t.Errorf("Status %q has value %q, want %q", status, string(status), str)
		}
	}
}
