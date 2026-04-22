package trigger

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

func TestMatchExact(t *testing.T) {
	r := NewTriggerRegistry()
	r.Register("engrams/go-errors.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: map[string]interface{}{"phase": "implementation"}, Priority: 80},
	})

	m := NewTriggerMatcher(r)
	results := m.Match(TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{"phase": "implementation"},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].EngramPath != "engrams/go-errors.ai.md" {
		t.Errorf("expected engram path 'engrams/go-errors.ai.md', got %q", results[0].EngramPath)
	}
}

func TestMatchArray(t *testing.T) {
	r := NewTriggerRegistry()
	r.Register("engrams/testing.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: map[string]interface{}{
			"phase": []interface{}{"testing", "qa", "validation"},
		}, Priority: 70},
	})

	m := NewTriggerMatcher(r)

	// Should match "qa" from the array.
	results := m.Match(TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{"phase": "qa"},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'qa', got %d", len(results))
	}

	// Should match "testing" from the array.
	results = m.Match(TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{"phase": "testing"},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'testing', got %d", len(results))
	}
}

func TestMatchNoMatch(t *testing.T) {
	r := NewTriggerRegistry()
	r.Register("engrams/go-errors.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: map[string]interface{}{"phase": "design"}, Priority: 80},
	})

	m := NewTriggerMatcher(r)
	results := m.Match(TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{"phase": "implementation"},
	})

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMatchEmptyMatch(t *testing.T) {
	r := NewTriggerRegistry()
	r.Register("engrams/always-on.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: nil, Priority: 50},
	})

	m := NewTriggerMatcher(r)
	results := m.Match(TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{"phase": "anything"},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for empty match, got %d", len(results))
	}
	if results[0].EngramPath != "engrams/always-on.ai.md" {
		t.Errorf("expected engram path 'engrams/always-on.ai.md', got %q", results[0].EngramPath)
	}
}

func TestMatchPrioritySorting(t *testing.T) {
	r := NewTriggerRegistry()
	r.Register("engrams/low-pri.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: nil, Priority: 10},
	})
	r.Register("engrams/high-pri.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: nil, Priority: 90},
	})
	r.Register("engrams/mid-pri.ai.md", []engram.TriggerSpec{
		{On: "phase.started", Match: nil, Priority: 50},
	})

	m := NewTriggerMatcher(r)
	results := m.Match(TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{},
	})

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Priority != 90 {
		t.Errorf("expected first result priority 90, got %d", results[0].Priority)
	}
	if results[1].Priority != 50 {
		t.Errorf("expected second result priority 50, got %d", results[1].Priority)
	}
	if results[2].Priority != 10 {
		t.Errorf("expected third result priority 10, got %d", results[2].Priority)
	}
}

func TestMatchMultipleTriggers(t *testing.T) {
	r := NewTriggerRegistry()
	r.Register("engrams/go-patterns.ai.md", []engram.TriggerSpec{
		{On: "task.assigned", Match: map[string]interface{}{"lang": "go"}, Priority: 70},
	})
	r.Register("engrams/error-handling.ai.md", []engram.TriggerSpec{
		{On: "task.assigned", Match: map[string]interface{}{"lang": "go"}, Priority: 60},
	})

	m := NewTriggerMatcher(r)
	results := m.Match(TriggerEvent{
		Type: "task.assigned",
		Data: map[string]interface{}{"lang": "go"},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].EngramPath != "engrams/go-patterns.ai.md" {
		t.Errorf("expected first result 'engrams/go-patterns.ai.md', got %q", results[0].EngramPath)
	}
	if results[1].EngramPath != "engrams/error-handling.ai.md" {
		t.Errorf("expected second result 'engrams/error-handling.ai.md', got %q", results[1].EngramPath)
	}
}
