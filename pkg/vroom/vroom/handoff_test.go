package vroom

import "testing"

func TestEmitHandedOff(t *testing.T) {
	pub := &mockPublisher{}
	em := NewEmitter(pub, "implementer")

	em.EmitHandedOff(HandedOffPayload{
		SessionID:       "sess-9",
		FromRole:        "implementer",
		ToRole:          "architect",
		ConfidenceLevel: "low",
		ConfidenceScore: 0.25,
		Rationale:       "integration suite never ran",
		Gaps:            []string{"db migration untested", "auth path unverified"},
	})

	ev := pub.waitForEvent(t)
	if ev.Topic != TopicDecisionHandedOff {
		t.Errorf("topic = %q, want %q", ev.Topic, TopicDecisionHandedOff)
	}
	if ev.Data["session_id"] != "sess-9" {
		t.Errorf("session_id = %v, want %q", ev.Data["session_id"], "sess-9")
	}
	if ev.Data["from_role"] != "implementer" {
		t.Errorf("from_role = %v, want %q", ev.Data["from_role"], "implementer")
	}
	if ev.Data["to_role"] != "architect" {
		t.Errorf("to_role = %v, want %q", ev.Data["to_role"], "architect")
	}
	if ev.Data["confidence_level"] != "low" {
		t.Errorf("confidence_level = %v, want %q", ev.Data["confidence_level"], "low")
	}
	// JSON round-trip turns numbers into float64.
	if score, ok := ev.Data["confidence_score"].(float64); !ok || score != 0.25 {
		t.Errorf("confidence_score = %v, want 0.25", ev.Data["confidence_score"])
	}
	gaps, ok := ev.Data["gaps"].([]any)
	if !ok || len(gaps) != 2 {
		t.Fatalf("gaps = %v, want 2 entries", ev.Data["gaps"])
	}
	// The emitter stamps every decision event with role/timestamp/event_id
	// so the append-only trail is self-describing.
	if ev.Data["role"] != "implementer" {
		t.Errorf("role = %v, want %q", ev.Data["role"], "implementer")
	}
	if ev.Data["event_id"] == nil || ev.Data["timestamp"] == nil {
		t.Errorf("event_id/timestamp must be stamped: %+v", ev.Data)
	}
}
