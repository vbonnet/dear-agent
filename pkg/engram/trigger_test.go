package engram

import (
	"testing"
)

// TestParseBytes_WithTriggers verifies parsing an engram with trigger specs
func TestParseBytes_WithTriggers(t *testing.T) {
	content := []byte(`---
type: pattern
title: Triggered Pattern
description: A pattern with triggers
tags:
  - test
triggers:
  - on: phase.started
    match:
      phase: planning
    scope: project
    priority: 80
    cooldown: 1h
  - on: task.assigned
    match:
      role: architect
    scope: global
    priority: 50
---
# Triggered Content

This engram has triggers.
`)

	parser := NewParser()
	eng, err := parser.ParseBytes("/test/triggered.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	if len(eng.Frontmatter.Triggers) != 2 {
		t.Fatalf("len(Frontmatter.Triggers) = %d, want 2", len(eng.Frontmatter.Triggers))
	}

	// Verify first trigger
	t1 := eng.Frontmatter.Triggers[0]
	if t1.On != "phase.started" {
		t.Errorf("Triggers[0].On = %q, want %q", t1.On, "phase.started")
	}
	if t1.Scope != "project" {
		t.Errorf("Triggers[0].Scope = %q, want %q", t1.Scope, "project")
	}
	if t1.Priority != 80 {
		t.Errorf("Triggers[0].Priority = %d, want 80", t1.Priority)
	}
	if t1.Cooldown != "1h" {
		t.Errorf("Triggers[0].Cooldown = %q, want %q", t1.Cooldown, "1h")
	}
	if phase, ok := t1.Match["phase"]; !ok || phase != "planning" {
		t.Errorf("Triggers[0].Match[\"phase\"] = %v, want \"planning\"", t1.Match["phase"])
	}

	// Verify second trigger
	t2 := eng.Frontmatter.Triggers[1]
	if t2.On != "task.assigned" {
		t.Errorf("Triggers[1].On = %q, want %q", t2.On, "task.assigned")
	}
	if t2.Scope != "global" {
		t.Errorf("Triggers[1].Scope = %q, want %q", t2.Scope, "global")
	}
	if t2.Priority != 50 {
		t.Errorf("Triggers[1].Priority = %d, want 50", t2.Priority)
	}
	if role, ok := t2.Match["role"]; !ok || role != "architect" {
		t.Errorf("Triggers[1].Match[\"role\"] = %v, want \"architect\"", t2.Match["role"])
	}
}

// TestParseBytes_WithoutTriggers verifies backward compatibility (no triggers field)
func TestParseBytes_WithoutTriggers(t *testing.T) {
	content := []byte(`---
type: pattern
title: No Triggers
description: A pattern without triggers
tags:
  - test
---
# No Triggers

This engram has no triggers.
`)

	parser := NewParser()
	eng, err := parser.ParseBytes("/test/no-triggers.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	if len(eng.Frontmatter.Triggers) != 0 {
		t.Errorf("len(Frontmatter.Triggers) = %d, want 0", len(eng.Frontmatter.Triggers))
	}
}

// TestParseBytes_EmptyTriggers verifies empty triggers field parses as nil/empty slice
func TestParseBytes_EmptyTriggers(t *testing.T) {
	content := []byte(`---
type: pattern
title: Empty Triggers
description: A pattern with empty triggers
triggers: []
---
# Empty Triggers

This engram has an explicit empty triggers list.
`)

	parser := NewParser()
	eng, err := parser.ParseBytes("/test/empty-triggers.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	if len(eng.Frontmatter.Triggers) != 0 {
		t.Errorf("len(Frontmatter.Triggers) = %d, want 0", len(eng.Frontmatter.Triggers))
	}
}

// TestParseBytes_TriggerMinimalFields verifies a trigger with only required "on" field
func TestParseBytes_TriggerMinimalFields(t *testing.T) {
	content := []byte(`---
type: pattern
title: Minimal Trigger
description: A pattern with a minimal trigger
triggers:
  - on: event
---
# Minimal Trigger

This engram has a trigger with only the on field.
`)

	parser := NewParser()
	eng, err := parser.ParseBytes("/test/minimal-trigger.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	if len(eng.Frontmatter.Triggers) != 1 {
		t.Fatalf("len(Frontmatter.Triggers) = %d, want 1", len(eng.Frontmatter.Triggers))
	}

	tr := eng.Frontmatter.Triggers[0]
	if tr.On != "event" {
		t.Errorf("Triggers[0].On = %q, want %q", tr.On, "event")
	}
	if tr.Scope != "" {
		t.Errorf("Triggers[0].Scope = %q, want empty", tr.Scope)
	}
	if tr.Priority != 0 {
		t.Errorf("Triggers[0].Priority = %d, want 0", tr.Priority)
	}
	if tr.Cooldown != "" {
		t.Errorf("Triggers[0].Cooldown = %q, want empty", tr.Cooldown)
	}
	if len(tr.Match) != 0 {
		t.Errorf("len(Triggers[0].Match) = %d, want 0", len(tr.Match))
	}
}
