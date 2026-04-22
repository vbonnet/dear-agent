package messaging

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/modelcard"
)

func newTestRouter(t *testing.T) (*Router, *modelcard.Registry) {
	t.Helper()
	reg := modelcard.NewRegistry()
	return NewRouter(reg), reg
}

func registerTestAgent(t *testing.T, reg *modelcard.Registry, id, role string) {
	t.Helper()
	card := modelcard.NewModelCard(id, id+"-name", role)
	if err := reg.Register(card); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
}

func TestRouter_RegisterAndSendDirect(t *testing.T) {
	router, _ := newTestRouter(t)

	var received *Message
	router.RegisterHandler("agent-2", func(msg *Message) error {
		received = msg
		return nil
	})

	msg := NewRequest("agent-1", "agent-2", "hello", "world", "act")
	delivered, err := router.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(delivered) != 1 || delivered[0] != "agent-2" {
		t.Errorf("delivered = %v, want [agent-2]", delivered)
	}
	if received == nil || received.Body != "world" {
		t.Error("handler did not receive the message")
	}
}

func TestRouter_DirectQueuesWhenNoHandler(t *testing.T) {
	router, _ := newTestRouter(t)

	msg := NewRequest("agent-1", "agent-2", "hello", "world", "act")
	delivered, err := router.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(delivered) != 1 || delivered[0] != "agent-2" {
		t.Errorf("delivered = %v, want [agent-2]", delivered)
	}

	// Check inbox
	queued := router.PeekInbox("agent-2")
	if len(queued) != 1 {
		t.Fatalf("inbox has %d messages, want 1", len(queued))
	}
	if queued[0].Body != "world" {
		t.Errorf("queued body = %q, want %q", queued[0].Body, "world")
	}
}

func TestRouter_DrainInbox(t *testing.T) {
	router, _ := newTestRouter(t)

	msg := NewRequest("agent-1", "agent-2", "hello", "world", "act")
	if _, err := router.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	drained := router.DrainInbox("agent-2")
	if len(drained) != 1 {
		t.Fatalf("drained %d messages, want 1", len(drained))
	}

	// Inbox should now be empty
	remaining := router.PeekInbox("agent-2")
	if len(remaining) != 0 {
		t.Errorf("inbox still has %d messages after drain", len(remaining))
	}
}

func TestRouter_DrainInbox_Empty(t *testing.T) {
	router, _ := newTestRouter(t)

	drained := router.DrainInbox("nobody")
	if len(drained) != 0 {
		t.Errorf("expected empty drain, got %d", len(drained))
	}
}

func TestRouter_DirectHandlerError(t *testing.T) {
	router, _ := newTestRouter(t)

	router.RegisterHandler("agent-2", func(msg *Message) error {
		return errors.New("handler failed")
	})

	msg := NewRequest("agent-1", "agent-2", "hello", "world", "act")
	_, err := router.Send(msg)
	if err == nil {
		t.Fatal("expected error from handler")
	}
}

func TestRouter_Broadcast(t *testing.T) {
	router, _ := newTestRouter(t)

	var mu sync.Mutex
	received := map[string]bool{}

	for _, id := range []string{"a", "b", "c"} {
		agentID := id
		router.RegisterHandler(agentID, func(msg *Message) error {
			mu.Lock()
			received[agentID] = true
			mu.Unlock()
			return nil
		})
	}

	msg := NewNotification("a", "update", "broadcast body")
	delivered, err := router.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Sender "a" should not receive its own broadcast
	if received["a"] {
		t.Error("sender received its own broadcast")
	}
	if !received["b"] || !received["c"] {
		t.Errorf("expected b and c to receive, got %v", received)
	}
	// delivered should contain b and c
	if len(delivered) != 2 {
		t.Errorf("delivered count = %d, want 2", len(delivered))
	}
}

func TestRouter_BroadcastHandlerErrorIsBestEffort(t *testing.T) {
	router, _ := newTestRouter(t)

	router.RegisterHandler("b", func(msg *Message) error {
		return errors.New("fail")
	})
	var cReceived bool
	router.RegisterHandler("c", func(msg *Message) error {
		cReceived = true
		return nil
	})

	msg := NewNotification("a", "update", "body")
	delivered, err := router.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !cReceived {
		t.Error("agent c should still receive despite b's error")
	}
	// b should NOT be in delivered because it errored
	for _, d := range delivered {
		if d == "b" {
			t.Error("agent b should not be in delivered list since handler errored")
		}
	}
}

func TestRouter_RoleRouting(t *testing.T) {
	router, reg := newTestRouter(t)

	registerTestAgent(t, reg, "w1", "worker")
	registerTestAgent(t, reg, "w2", "worker")
	registerTestAgent(t, reg, "r1", "reviewer")

	var mu sync.Mutex
	received := map[string]bool{}

	for _, id := range []string{"w1", "w2", "r1"} {
		agentID := id
		router.RegisterHandler(agentID, func(msg *Message) error {
			mu.Lock()
			received[agentID] = true
			mu.Unlock()
			return nil
		})
	}

	msg := NewRoleMessage("orch", "worker", TypeRequest, "build", "build please")
	delivered, err := router.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !received["w1"] || !received["w2"] {
		t.Errorf("expected both workers to receive, got %v", received)
	}
	if received["r1"] {
		t.Error("reviewer should not receive worker-targeted message")
	}
	if len(delivered) != 2 {
		t.Errorf("delivered count = %d, want 2", len(delivered))
	}
}

func TestRouter_RoleRoutingSkipsSelf(t *testing.T) {
	router, reg := newTestRouter(t)

	registerTestAgent(t, reg, "w1", "worker")
	registerTestAgent(t, reg, "w2", "worker")

	var w1Received bool
	router.RegisterHandler("w1", func(msg *Message) error {
		w1Received = true
		return nil
	})
	router.RegisterHandler("w2", func(msg *Message) error {
		return nil
	})

	// w1 sends to role "worker" — should not get its own message
	msg := NewRoleMessage("w1", "worker", TypeNotification, "status", "my status")
	if _, err := router.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if w1Received {
		t.Error("sender should not receive its own role-routed message")
	}
}

func TestRouter_RoleRoutingNoAgents(t *testing.T) {
	router, _ := newTestRouter(t)

	msg := NewRoleMessage("orch", "nonexistent", TypeRequest, "hello", "body")
	_, err := router.Send(msg)
	if err == nil {
		t.Fatal("expected error when no agents have the target role")
	}
}

func TestRouter_RoleRoutingQueuesWhenNoHandler(t *testing.T) {
	router, reg := newTestRouter(t)

	registerTestAgent(t, reg, "w1", "worker")
	// No handler registered for w1

	msg := NewRoleMessage("orch", "worker", TypeRequest, "build", "build it")
	delivered, err := router.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(delivered) != 1 || delivered[0] != "w1" {
		t.Errorf("delivered = %v, want [w1]", delivered)
	}

	queued := router.PeekInbox("w1")
	if len(queued) != 1 {
		t.Fatalf("inbox has %d messages, want 1", len(queued))
	}
}

func TestRouter_UnregisterHandler(t *testing.T) {
	router, _ := newTestRouter(t)

	router.RegisterHandler("agent-2", func(msg *Message) error {
		t.Fatal("handler should not be called after unregister")
		return nil
	})
	router.UnregisterHandler("agent-2")

	msg := NewRequest("agent-1", "agent-2", "hello", "world", "act")
	if _, err := router.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Should be queued since handler was removed
	queued := router.PeekInbox("agent-2")
	if len(queued) != 1 {
		t.Errorf("expected message queued after unregister, got %d", len(queued))
	}
}

func TestRouter_SendValidationError(t *testing.T) {
	router, _ := newTestRouter(t)

	msg := &Message{} // empty message fails validation
	_, err := router.Send(msg)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouter_SendExpiredMessage(t *testing.T) {
	router, _ := newTestRouter(t)

	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.ExpiresAt = time.Now().Add(-time.Hour)

	_, err := router.Send(msg)
	if err == nil {
		t.Fatal("expected expired message error")
	}
}

func TestRouter_SendUnknownRoutingMode(t *testing.T) {
	router, _ := newTestRouter(t)

	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.RoutingMode = "carrier-pigeon"
	// Must pass validation first — RoutingMode validation happens in Validate
	// but unknown modes also hit the switch default in Send
	_, err := router.Send(msg)
	if err == nil {
		t.Fatal("expected error for unknown routing mode")
	}
}
