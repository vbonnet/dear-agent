package trigger

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// createTestEngram creates a temporary .ai.md file for testing.
func createTestEngram(t *testing.T, dir string, name string, title string, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	fileContent := "---\n" +
		"type: pattern\n" +
		"title: " + title + "\n" +
		"tags: [test]\n" +
		"---\n" +
		content
	if err := os.WriteFile(path, []byte(fileContent), 0644); err != nil {
		t.Fatalf("Failed to create test engram: %v", err)
	}
	return path
}

func TestHandleEvent(t *testing.T) {
	tmpDir := t.TempDir()
	engramDir := filepath.Join(tmpDir, "engrams")
	projectDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(engramDir, 0755)
	os.MkdirAll(projectDir, 0755)

	// Create a test engram file
	engramPath := createTestEngram(t, engramDir, "test-pattern.ai.md",
		"Test Pattern", "This is test content for the pattern.\n")

	// Set up registry with a trigger
	registry := NewTriggerRegistry()
	registry.Register(engramPath, []engram.TriggerSpec{
		{
			On:       "task.started",
			Priority: 50,
		},
	})

	// Set up matcher and subscriber
	matcher := NewTriggerMatcher(registry)
	statePath := filepath.Join(tmpDir, "trigger-state.json")
	subscriber := NewTriggerSubscriber(matcher, statePath, projectDir)

	// Create an event
	event := eventbus.NewEvent("task.started", "test-publisher", map[string]interface{}{
		"task": "build",
	})

	// Handle the event
	resp, err := subscriber.handleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("handleEvent failed: %v", err)
	}
	if resp != nil {
		t.Error("Expected nil response (fire-and-forget)")
	}

	// Verify injection happened
	outputPath := filepath.Join(projectDir, ".engram", "triggered-context.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "## Test Pattern") {
		t.Error("Missing engram title in output")
	}
	if !strings.Contains(content, "This is test content for the pattern.") {
		t.Error("Missing engram content in output")
	}
	if !strings.Contains(content, "<!-- Event: task.started -->") {
		t.Error("Missing event type in output")
	}

	// Verify state was saved
	state, err := LoadTriggerState(statePath)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if _, ok := state.LastInjected[engramPath]; !ok {
		t.Error("State should record injection for engram path")
	}
}

func TestHandleEventNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(projectDir, 0755)

	// Set up registry with NO matching triggers
	registry := NewTriggerRegistry()
	registry.Register("/nonexistent/engram.ai.md", []engram.TriggerSpec{
		{
			On:       "phase.completed",
			Priority: 50,
		},
	})

	matcher := NewTriggerMatcher(registry)
	statePath := filepath.Join(tmpDir, "trigger-state.json")
	subscriber := NewTriggerSubscriber(matcher, statePath, projectDir)

	// Send an event that does NOT match any trigger
	event := eventbus.NewEvent("task.started", "test-publisher", map[string]interface{}{
		"task": "build",
	})

	resp, err := subscriber.handleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("handleEvent failed: %v", err)
	}
	if resp != nil {
		t.Error("Expected nil response")
	}

	// Verify no injection file was created
	outputPath := filepath.Join(projectDir, ".engram", "triggered-context.md")
	if _, err := os.Stat(outputPath); err == nil {
		t.Error("triggered-context.md should not exist when no triggers match")
	}
}
