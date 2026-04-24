package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/trigger"
)

func TestTriggerListCommand(t *testing.T) {
	// Verify the trigger command is registered on rootCmd.
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "trigger" {
			found = true
			// Verify subcommands.
			subNames := make(map[string]bool)
			for _, sub := range c.Commands() {
				subNames[sub.Use] = true
			}
			if !subNames["list"] {
				t.Error("trigger command missing 'list' subcommand")
			}
			if !subNames["evaluate <event-type>"] {
				t.Error("trigger command missing 'evaluate' subcommand")
			}
			if !subNames["history"] {
				t.Error("trigger command missing 'history' subcommand")
			}
			break
		}
	}
	if !found {
		t.Fatal("trigger command not registered on rootCmd")
	}
}

func TestTriggerListCommand_WithEngrams(t *testing.T) {
	// Create a temp directory with triggered engrams.
	tmpDir := t.TempDir()

	engramContent := `---
type: pattern
title: Test Triggered Engram
tags:
  - testing
triggers:
  - on: phase.started
    priority: 80
    scope: project
    cooldown: 1h
  - on: task.assigned
    priority: 50
---
# Test content
This is a test engram with triggers.
`
	engramPath := filepath.Join(tmpDir, "test-trigger.ai.md")
	if err := os.WriteFile(engramPath, []byte(engramContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a non-triggered engram to verify it's excluded.
	plainContent := `---
type: strategy
title: Plain Engram
tags:
  - testing
---
# No triggers
`
	plainPath := filepath.Join(tmpDir, "plain.ai.md")
	if err := os.WriteFile(plainPath, []byte(plainContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run scanTriggeredEngrams.
	entries, err := scanTriggeredEngrams(tmpDir)
	if err != nil {
		t.Fatalf("scanTriggeredEngrams: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 triggered engram, got %d", len(entries))
	}

	if entries[0].path != engramPath {
		t.Errorf("expected path %s, got %s", engramPath, entries[0].path)
	}

	if len(entries[0].triggers) != 2 {
		t.Errorf("expected 2 triggers, got %d", len(entries[0].triggers))
	}
}

func TestTriggerListCommand_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	entries, err := scanTriggeredEngrams(tmpDir)
	if err != nil {
		t.Fatalf("scanTriggeredEngrams: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 triggered engrams, got %d", len(entries))
	}
}

func TestTriggerEvaluateCommand(t *testing.T) {
	// Create a temp directory with triggered engrams.
	tmpDir := t.TempDir()

	engram1 := `---
type: pattern
title: Phase Start Handler
tags:
  - phases
triggers:
  - on: phase.started
    priority: 80
    scope: project
---
# Phase start content
`
	engram2 := `---
type: strategy
title: Task Assignment Handler
tags:
  - tasks
triggers:
  - on: task.assigned
    priority: 60
  - on: phase.started
    priority: 40
---
# Task content
`
	if err := os.WriteFile(filepath.Join(tmpDir, "phase-handler.ai.md"), []byte(engram1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "task-handler.ai.md"), []byte(engram2), 0644); err != nil {
		t.Fatal(err)
	}

	// Scan and build registry.
	entries, err := scanTriggeredEngrams(tmpDir)
	if err != nil {
		t.Fatalf("scanTriggeredEngrams: %v", err)
	}

	registry := buildRegistryFromEntries(entries)

	// Evaluate phase.started event.
	t.Run("phase.started matches", func(t *testing.T) {
		from_registry := registry.Lookup("phase.started")
		if len(from_registry) != 2 {
			t.Fatalf("expected 2 entries for phase.started, got %d", len(from_registry))
		}
	})

	// Evaluate task.assigned event.
	t.Run("task.assigned matches", func(t *testing.T) {
		from_registry := registry.Lookup("task.assigned")
		if len(from_registry) != 1 {
			t.Fatalf("expected 1 entry for task.assigned, got %d", len(from_registry))
		}
	})

	// Evaluate unknown event.
	t.Run("unknown event no matches", func(t *testing.T) {
		from_registry := registry.Lookup("nonexistent.event")
		if len(from_registry) != 0 {
			t.Fatalf("expected 0 entries for nonexistent.event, got %d", len(from_registry))
		}
	})
}

func TestTriggerEvaluateCommand_WithMatcher(t *testing.T) {
	tmpDir := t.TempDir()

	engram1 := `---
type: pattern
title: High Priority Handler
tags:
  - testing
triggers:
  - on: phase.started
    priority: 90
---
# High priority
`
	engram2 := `---
type: strategy
title: Low Priority Handler
tags:
  - testing
triggers:
  - on: phase.started
    priority: 20
---
# Low priority
`
	if err := os.WriteFile(filepath.Join(tmpDir, "high.ai.md"), []byte(engram1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "low.ai.md"), []byte(engram2), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := scanTriggeredEngrams(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	registry := buildRegistryFromEntries(entries)
	matcher := trigger.NewTriggerMatcher(registry)

	event := trigger.TriggerEvent{
		Type: "phase.started",
		Data: map[string]interface{}{},
	}

	results := matcher.Match(event)
	if len(results) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(results))
	}

	// Results should be sorted by priority descending.
	if results[0].Priority < results[1].Priority {
		t.Errorf("results not sorted by priority: %d < %d", results[0].Priority, results[1].Priority)
	}
	if results[0].Priority != 90 {
		t.Errorf("expected first result priority 90, got %d", results[0].Priority)
	}
	if results[1].Priority != 20 {
		t.Errorf("expected second result priority 20, got %d", results[1].Priority)
	}
}

func TestTriggerHistoryCommand(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "trigger-state.json")

	// Create a trigger-state.json with some history.
	now := time.Now().Truncate(time.Second)
	stateData := map[string]interface{}{
		"last_injected": map[string]interface{}{
			"engrams/patterns/error-handling.ai.md": now.Add(-1 * time.Hour).Format(time.RFC3339),
			"engrams/strategies/testing.ai.md":      now.Add(-30 * time.Minute).Format(time.RFC3339),
			"engrams/workflows/deploy.ai.md":        now.Format(time.RFC3339),
		},
	}

	data, err := json.Marshal(stateData)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Load the state.
	state, err := trigger.LoadTriggerState(stateFile)
	if err != nil {
		t.Fatalf("LoadTriggerState: %v", err)
	}

	if len(state.LastInjected) != 3 {
		t.Errorf("expected 3 entries in LastInjected, got %d", len(state.LastInjected))
	}

	// Verify the paths exist in the map.
	paths := []string{
		"engrams/patterns/error-handling.ai.md",
		"engrams/strategies/testing.ai.md",
		"engrams/workflows/deploy.ai.md",
	}
	for _, p := range paths {
		if _, ok := state.LastInjected[p]; !ok {
			t.Errorf("expected path %s in LastInjected", p)
		}
	}
}

func TestTriggerHistoryCommand_NoState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "nonexistent-trigger-state.json")

	// Loading a nonexistent file should return empty state, not error.
	state, err := trigger.LoadTriggerState(stateFile)
	if err != nil {
		t.Fatalf("LoadTriggerState for nonexistent file should not error: %v", err)
	}

	if len(state.LastInjected) != 0 {
		t.Errorf("expected 0 entries for nonexistent state file, got %d", len(state.LastInjected))
	}
}

func TestTriggerCommandRegistration(t *testing.T) {
	// Verify the trigger command structure.
	if triggerCmd.Use != "trigger" {
		t.Errorf("expected triggerCmd.Use = 'trigger', got %q", triggerCmd.Use)
	}

	if triggerListCmd.Use != "list" {
		t.Errorf("expected triggerListCmd.Use = 'list', got %q", triggerListCmd.Use)
	}

	if triggerEvaluateCmd.Use != "evaluate <event-type>" {
		t.Errorf("expected triggerEvaluateCmd.Use = 'evaluate <event-type>', got %q", triggerEvaluateCmd.Use)
	}

	if triggerHistoryCmd.Use != "history" {
		t.Errorf("expected triggerHistoryCmd.Use = 'history', got %q", triggerHistoryCmd.Use)
	}

	// Verify evaluate command requires exactly 1 arg.
	if err := triggerEvaluateCmd.Args(triggerEvaluateCmd, []string{}); err == nil {
		t.Error("evaluate command should require exactly 1 argument")
	}

	if err := triggerEvaluateCmd.Args(triggerEvaluateCmd, []string{"phase.started"}); err != nil {
		t.Errorf("evaluate command should accept 1 argument: %v", err)
	}

	// Verify --data flag is registered on evaluate.
	dataFlag := triggerEvaluateCmd.Flags().Lookup("data")
	if dataFlag == nil {
		t.Error("evaluate command should have --data flag")
	}
}
