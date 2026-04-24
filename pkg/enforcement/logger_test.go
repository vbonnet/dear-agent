package enforcement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileViolation(t *testing.T) {
	dir := t.TempDir()

	pattern := &Pattern{
		ID:          "cd-command",
		Reason:      "Using cd command",
		Alternative: "Use -C flag",
		Severity:    "high",
	}

	violation := ViolationData{
		PatternID:   "cd-command",
		PatternType: "bash",
		Command:     "cd /repo",
		SessionID:   "test-session",
		AgentType:   "general-purpose",
		Timestamp:   time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC),
	}

	path, err := FileViolation(violation, dir, pattern)
	if err != nil {
		t.Fatalf("FileViolation failed: %v", err)
	}

	if !strings.Contains(path, "2026-03-28-cd-command-") {
		t.Errorf("unexpected path: %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	s := string(content)
	if !strings.Contains(s, "pattern_id: cd-command") {
		t.Error("content should contain pattern_id")
	}
	if !strings.Contains(s, "severity: high") {
		t.Error("content should contain severity")
	}
	if !strings.Contains(s, "cd /repo") {
		t.Error("content should contain command")
	}
}

func TestFileViolationWithOptionalFields(t *testing.T) {
	dir := t.TempDir()

	pattern := &Pattern{
		ID:       "test",
		Reason:   "test",
		Severity: "medium",
	}

	violation := ViolationData{
		PatternID:          "test",
		PatternType:        "bash",
		Command:            "test cmd",
		SessionID:          "session",
		AgentType:          "explore",
		Tags:               []string{"tag1", "tag2"},
		ConversationLength: 42,
		EngramVersion:      "1.0.0",
		EngramHash:         "abc123",
	}

	path, err := FileViolation(violation, dir, pattern)
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	s := string(content)
	if !strings.Contains(s, "conversation_length: 42") {
		t.Error("should contain conversation_length")
	}
	if !strings.Contains(s, "- tag1") {
		t.Error("should contain tags")
	}
	if !strings.Contains(s, "engram_version: 1.0.0") {
		t.Error("should contain engram_version")
	}
}

func TestFileViolationMissingRequired(t *testing.T) {
	dir := t.TempDir()
	pattern := &Pattern{ID: "test"}

	_, err := FileViolation(ViolationData{}, dir, pattern)
	if err == nil {
		t.Error("expected error for missing required fields")
	}

	_, err = FileViolation(ViolationData{PatternID: "a", PatternType: "b", Command: "c"}, dir, nil)
	if err == nil {
		t.Error("expected error for nil pattern")
	}
}

func TestFileViolationCreatesSubdir(t *testing.T) {
	dir := t.TempDir()
	pattern := &Pattern{ID: "test", Severity: "low"}
	violation := ViolationData{
		PatternID:   "test",
		PatternType: "bash",
		Command:     "test",
		SessionID:   "s",
		AgentType:   "a",
	}

	_, err := FileViolation(violation, dir, pattern)
	if err != nil {
		t.Fatal(err)
	}

	// Check subdirectory was created
	if _, err := os.Stat(filepath.Join(dir, "bash")); os.IsNotExist(err) {
		t.Error("expected bash subdirectory to be created")
	}
}

func TestViolationType(t *testing.T) {
	if ViolationType("cd-command") != "cd_usage" {
		t.Error("cd-command should map to cd_usage")
	}
	if ViolationType("command-chaining") != "chained_commands" {
		t.Error("command-chaining should map to chained_commands")
	}
	if ViolationType("unknown-pattern") != "bash_over_tools" {
		t.Error("unknown patterns should default to bash_over_tools")
	}
}
