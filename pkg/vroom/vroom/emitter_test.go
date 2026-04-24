package vroom

import (
	"sync"
	"testing"
	"time"
)

// mockPublisher records published events for assertion.
type mockPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	Topic string
	Data  map[string]interface{}
}

func (m *mockPublisher) Publish(topic string, data map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, publishedEvent{Topic: topic, Data: data})
	return nil
}

func (m *mockPublisher) waitForEvent(t *testing.T) publishedEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		if len(m.events) > 0 {
			ev := m.events[0]
			m.events = m.events[1:]
			m.mu.Unlock()
			return ev
		}
		m.mu.Unlock()
		select {
		case <-deadline:
			t.Fatal("timed out waiting for event")
			return publishedEvent{}
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestEmitDispatched(t *testing.T) {
	pub := &mockPublisher{}
	em := NewEmitter(pub, "orchestrator")

	em.EmitDispatched(DispatchedPayload{
		SessionID: "sess-1",
		TaskID:    "task-42",
		Worker:    "worker-a",
		Rationale: "highest priority",
	})

	ev := pub.waitForEvent(t)
	if ev.Topic != TopicDecisionDispatched {
		t.Errorf("topic = %q, want %q", ev.Topic, TopicDecisionDispatched)
	}
	if ev.Data["session_id"] != "sess-1" {
		t.Errorf("session_id = %v, want %q", ev.Data["session_id"], "sess-1")
	}
	if ev.Data["task_id"] != "task-42" {
		t.Errorf("task_id = %v, want %q", ev.Data["task_id"], "task-42")
	}
	if ev.Data["worker"] != "worker-a" {
		t.Errorf("worker = %v, want %q", ev.Data["worker"], "worker-a")
	}
	if ev.Data["role"] != "orchestrator" {
		t.Errorf("role = %v, want %q", ev.Data["role"], "orchestrator")
	}
	if _, ok := ev.Data["event_id"]; !ok {
		t.Error("missing event_id")
	}
	if _, ok := ev.Data["timestamp"]; !ok {
		t.Error("missing timestamp")
	}
}

func TestEmitEscalated(t *testing.T) {
	pub := &mockPublisher{}
	em := NewEmitter(pub, "overseer")

	em.EmitEscalated(EscalatedPayload{
		SessionID: "sess-2",
		Anomaly:   "stuck loop detected",
		Severity:  "high",
		Rationale: "no progress for 10 minutes",
	})

	ev := pub.waitForEvent(t)
	if ev.Topic != TopicDecisionEscalated {
		t.Errorf("topic = %q, want %q", ev.Topic, TopicDecisionEscalated)
	}
	if ev.Data["anomaly"] != "stuck loop detected" {
		t.Errorf("anomaly = %v, want %q", ev.Data["anomaly"], "stuck loop detected")
	}
	if ev.Data["severity"] != "high" {
		t.Errorf("severity = %v, want %q", ev.Data["severity"], "high")
	}
	if ev.Data["role"] != "overseer" {
		t.Errorf("role = %v, want %q", ev.Data["role"], "overseer")
	}
}

func TestEmitEvaluated(t *testing.T) {
	pub := &mockPublisher{}
	em := NewEmitter(pub, "verifier")

	em.EmitEvaluated(EvaluatedPayload{
		SessionID: "sess-3",
		OutputRef: "commit:abc123",
		Passed:    true,
		Rationale: "all quality gates passed",
	})

	ev := pub.waitForEvent(t)
	if ev.Topic != TopicDecisionEvaluated {
		t.Errorf("topic = %q, want %q", ev.Topic, TopicDecisionEvaluated)
	}
	if ev.Data["output_ref"] != "commit:abc123" {
		t.Errorf("output_ref = %v, want %q", ev.Data["output_ref"], "commit:abc123")
	}
	if ev.Data["passed"] != true {
		t.Errorf("passed = %v, want true", ev.Data["passed"])
	}
	if ev.Data["role"] != "verifier" {
		t.Errorf("role = %v, want %q", ev.Data["role"], "verifier")
	}
}

func TestEmitGated(t *testing.T) {
	pub := &mockPublisher{}
	em := NewEmitter(pub, "meta-orchestrator")

	em.EmitGated(GatedPayload{
		FromState: "planning",
		ToState:   "executing",
		Approved:  true,
		Rationale: "HITL approval received",
	})

	ev := pub.waitForEvent(t)
	if ev.Topic != TopicDecisionGated {
		t.Errorf("topic = %q, want %q", ev.Topic, TopicDecisionGated)
	}
	if ev.Data["from_state"] != "planning" {
		t.Errorf("from_state = %v, want %q", ev.Data["from_state"], "planning")
	}
	if ev.Data["to_state"] != "executing" {
		t.Errorf("to_state = %v, want %q", ev.Data["to_state"], "executing")
	}
	if ev.Data["approved"] != true {
		t.Errorf("approved = %v, want true", ev.Data["approved"])
	}
	if ev.Data["role"] != "meta-orchestrator" {
		t.Errorf("role = %v, want %q", ev.Data["role"], "meta-orchestrator")
	}
}
