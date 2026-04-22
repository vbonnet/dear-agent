package trigger

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

func TestRegister(t *testing.T) {
	r := NewTriggerRegistry()

	triggers := []engram.TriggerSpec{
		{On: "phase.started", Match: map[string]interface{}{"phase": "design"}, Priority: 80},
		{On: "task.assigned", Match: map[string]interface{}{"role": "backend"}, Priority: 50},
	}

	r.Register("engrams/design-patterns.ai.md", triggers)

	entries := r.Lookup("phase.started")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for phase.started, got %d", len(entries))
	}
	if entries[0].EngramPath != "engrams/design-patterns.ai.md" {
		t.Errorf("expected engram path 'engrams/design-patterns.ai.md', got %q", entries[0].EngramPath)
	}
	if entries[0].Trigger.Priority != 80 {
		t.Errorf("expected priority 80, got %d", entries[0].Trigger.Priority)
	}

	entries = r.Lookup("task.assigned")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for task.assigned, got %d", len(entries))
	}
	if entries[0].EngramPath != "engrams/design-patterns.ai.md" {
		t.Errorf("expected engram path 'engrams/design-patterns.ai.md', got %q", entries[0].EngramPath)
	}
}

func TestLookupEmpty(t *testing.T) {
	r := NewTriggerRegistry()

	entries := r.Lookup("nonexistent.event")
	if entries != nil {
		t.Errorf("expected nil for nonexistent event type, got %v", entries)
	}
}

func TestClear(t *testing.T) {
	r := NewTriggerRegistry()

	triggers := []engram.TriggerSpec{
		{On: "phase.started", Priority: 50},
		{On: "task.assigned", Priority: 50},
	}
	r.Register("engrams/test.ai.md", triggers)

	// Verify entries exist.
	if entries := r.Lookup("phase.started"); len(entries) == 0 {
		t.Fatal("expected entries before clear")
	}

	r.Clear()

	if entries := r.Lookup("phase.started"); entries != nil {
		t.Errorf("expected nil after clear, got %v", entries)
	}
	if entries := r.Lookup("task.assigned"); entries != nil {
		t.Errorf("expected nil after clear, got %v", entries)
	}
}
