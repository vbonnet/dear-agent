package backlog

import (
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/vroom/vroom"
)

type capturePublisher struct {
	topic string
	data  map[string]interface{}
	err   error
}

func (c *capturePublisher) Publish(topic string, data map[string]interface{}) error {
	c.topic = topic
	c.data = data
	return c.err
}

func sampleSuggestion() Suggestion {
	return Suggestion{
		Item:   Item{ID: "1.4", Title: "Quick lint fix"},
		Reason: "priority=—, unblocks 0, effort=S",
	}
}

func TestOrchestratorNotifierDispatch(t *testing.T) {
	pub := &capturePublisher{}
	err := NewOrchestratorNotifier(pub).WithWorker("claude-code").Dispatch(sampleSuggestion())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if pub.topic != vroom.TopicDecisionDispatched {
		t.Errorf("topic = %q, want %q", pub.topic, vroom.TopicDecisionDispatched)
	}
	if pub.data["task_id"] != "1.4" {
		t.Errorf("task_id = %v, want 1.4", pub.data["task_id"])
	}
	if pub.data["worker"] != "claude-code" {
		t.Errorf("worker = %v, want claude-code", pub.data["worker"])
	}
	if pub.data["role"] != DefaultOrchestratorRole {
		t.Errorf("role = %v, want %s", pub.data["role"], DefaultOrchestratorRole)
	}
	for _, k := range []string{"event_id", "timestamp", "rationale", "session_id"} {
		if _, ok := pub.data[k]; !ok {
			t.Errorf("envelope missing %q", k)
		}
	}
	if r, _ := pub.data["rationale"].(string); r == "" {
		t.Error("rationale should be non-empty")
	}
}

func TestOrchestratorNotifierNilPublisherIsNoop(t *testing.T) {
	if err := NewOrchestratorNotifier(nil).Dispatch(sampleSuggestion()); err != nil {
		t.Errorf("nil publisher should be a no-op, got %v", err)
	}
}

func TestOrchestratorNotifierPropagatesError(t *testing.T) {
	pub := &capturePublisher{err: errors.New("bus down")}
	if err := NewOrchestratorNotifier(pub).Dispatch(sampleSuggestion()); err == nil {
		t.Error("publish error should propagate")
	}
}
