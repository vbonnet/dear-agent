package broker

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/messaging"
	"github.com/vbonnet/dear-agent/agm/internal/a2a/modelcard"
)

func newTestBroker(t *testing.T) *Broker {
	t.Helper()
	return New()
}

func makeCard(id, role string, caps ...string) *modelcard.ModelCard {
	card := modelcard.NewModelCard(id, id+"-name", role)
	card.Capabilities = caps
	return card
}

func TestBroker_RegisterAndGetAgent(t *testing.T) {
	b := newTestBroker(t)

	card := makeCard("agent-1", "worker")
	if err := b.RegisterAgent(card); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	got, ok := b.GetAgent("agent-1")
	if !ok {
		t.Fatal("expected agent to be found")
	}
	if got.AgentID != "agent-1" {
		t.Errorf("agent_id = %q, want %q", got.AgentID, "agent-1")
	}
}

func TestBroker_RegisterAgentValidation(t *testing.T) {
	b := newTestBroker(t)

	card := &modelcard.ModelCard{} // missing required fields
	err := b.RegisterAgent(card)
	if err == nil {
		t.Fatal("expected error for invalid card")
	}
}

func TestBroker_UnregisterAgent(t *testing.T) {
	b := newTestBroker(t)

	card := makeCard("agent-1", "worker")
	if err := b.RegisterAgent(card); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	b.UnregisterAgent("agent-1")

	_, ok := b.GetAgent("agent-1")
	if ok {
		t.Error("expected agent to be unregistered")
	}
}

func TestBroker_SendAndReceive(t *testing.T) {
	b := newTestBroker(t)

	if err := b.RegisterAgent(makeCard("sender", "orchestrator")); err != nil {
		t.Fatalf("register sender: %v", err)
	}
	if err := b.RegisterAgent(makeCard("receiver", "worker")); err != nil {
		t.Fatalf("register receiver: %v", err)
	}

	var received *messaging.Message
	b.OnMessage("receiver", func(msg *messaging.Message) error {
		received = msg
		return nil
	})

	msg := messaging.NewRequest("sender", "receiver", "task", "do it", "build")
	delivered, err := b.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(delivered) != 1 || delivered[0] != "receiver" {
		t.Errorf("delivered = %v, want [receiver]", delivered)
	}
	if received == nil || received.Body != "do it" {
		t.Error("handler did not receive the message")
	}
}

func TestBroker_DrainInbox(t *testing.T) {
	b := newTestBroker(t)

	// Send without handler so message is queued
	msg := messaging.NewRequest("sender", "receiver", "task", "queued", "act")
	if _, err := b.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	drained := b.DrainInbox("receiver")
	if len(drained) != 1 {
		t.Fatalf("drained %d messages, want 1", len(drained))
	}
	if drained[0].Body != "queued" {
		t.Errorf("body = %q, want %q", drained[0].Body, "queued")
	}

	// Should be empty after drain
	again := b.DrainInbox("receiver")
	if len(again) != 0 {
		t.Errorf("expected empty after drain, got %d", len(again))
	}
}

func TestBroker_FindAgentsByRole(t *testing.T) {
	b := newTestBroker(t)

	if err := b.RegisterAgent(makeCard("w1", "worker")); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterAgent(makeCard("w2", "worker")); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterAgent(makeCard("r1", "reviewer")); err != nil {
		t.Fatal(err)
	}

	workers := b.FindAgentsByRole("worker")
	if len(workers) != 2 {
		t.Errorf("found %d workers, want 2", len(workers))
	}

	reviewers := b.FindAgentsByRole("reviewer")
	if len(reviewers) != 1 {
		t.Errorf("found %d reviewers, want 1", len(reviewers))
	}

	none := b.FindAgentsByRole("nonexistent")
	if len(none) != 0 {
		t.Errorf("found %d for nonexistent role, want 0", len(none))
	}
}

func TestBroker_FindAgentsByCapability(t *testing.T) {
	b := newTestBroker(t)

	if err := b.RegisterAgent(makeCard("a1", "worker", "code-review", "testing")); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterAgent(makeCard("a2", "worker", "refactoring")); err != nil {
		t.Fatal(err)
	}

	testers := b.FindAgentsByCapability("testing")
	if len(testers) != 1 {
		t.Errorf("found %d testers, want 1", len(testers))
	}
	if testers[0].AgentID != "a1" {
		t.Errorf("tester = %q, want %q", testers[0].AgentID, "a1")
	}

	none := b.FindAgentsByCapability("unknown")
	if len(none) != 0 {
		t.Errorf("found %d for unknown capability, want 0", len(none))
	}
}

func TestBroker_ActiveAgents(t *testing.T) {
	b := newTestBroker(t)

	if err := b.RegisterAgent(makeCard("a1", "worker")); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterAgent(makeCard("a2", "worker")); err != nil {
		t.Fatal(err)
	}

	// Both start as active
	active := b.ActiveAgents()
	if len(active) != 2 {
		t.Errorf("active = %d, want 2", len(active))
	}

	// Set one to offline
	if err := b.UpdateStatus("a2", modelcard.StatusOffline); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	active = b.ActiveAgents()
	if len(active) != 1 {
		t.Errorf("active = %d after offline, want 1", len(active))
	}
}

func TestBroker_UpdateStatus(t *testing.T) {
	b := newTestBroker(t)

	if err := b.RegisterAgent(makeCard("a1", "worker")); err != nil {
		t.Fatal(err)
	}

	if err := b.UpdateStatus("a1", modelcard.StatusBusy); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	card, _ := b.GetAgent("a1")
	if card.Status != modelcard.StatusBusy {
		t.Errorf("status = %q, want %q", card.Status, modelcard.StatusBusy)
	}
}

func TestBroker_UpdateStatusUnknownAgent(t *testing.T) {
	b := newTestBroker(t)

	err := b.UpdateStatus("unknown", modelcard.StatusBusy)
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestBroker_UnregisterRemovesHandler(t *testing.T) {
	b := newTestBroker(t)

	if err := b.RegisterAgent(makeCard("a1", "worker")); err != nil {
		t.Fatal(err)
	}
	b.OnMessage("a1", func(msg *messaging.Message) error {
		t.Fatal("handler should not be called after unregister")
		return nil
	})

	b.UnregisterAgent("a1")

	// Message should be queued, not delivered to handler
	msg := messaging.NewRequest("sender", "a1", "hello", "body", "act")
	if _, err := b.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	drained := b.DrainInbox("a1")
	if len(drained) != 1 {
		t.Errorf("expected message queued, got %d", len(drained))
	}
}

// Verify Broker satisfies MessageBroker interface.
var _ MessageBroker = (*Broker)(nil)
