package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
)

func TestGenerateRoadmap(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test task queue
	queueFile := filepath.Join(tmpDir, "queue.yaml")
	queueData := `schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready:
  - id: bead-1
    title: First bead
    tier: 1
    prompts:
      start: "Do task 1"
in_progress:
  - id: bead-2
    title: Second bead
    tier: 2
    prompts:
      start: "Do task 2"
    metadata:
      session_name: "session-1"
      iterations: 1
blocked: []
completed:
  - id: bead-3
    title: Third bead
    tier: 1
    prompts:
      start: "Do task 3"
`
	if err := os.WriteFile(queueFile, []byte(queueData), 0644); err != nil {
		t.Fatalf("failed to create test queue: %v", err)
	}

	// Generate roadmap
	roadmapFile := filepath.Join(tmpDir, "ROADMAP.md")
	if err := GenerateRoadmap(queueFile, roadmapFile); err != nil {
		t.Fatalf("GenerateRoadmap failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(roadmapFile); err != nil {
		t.Errorf("roadmap file not created: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(roadmapFile)
	if err != nil {
		t.Fatalf("failed to read roadmap file: %v", err)
	}

	roadmapStr := string(content)

	// Verify sections exist
	if !strings.Contains(roadmapStr, "# Task Roadmap") {
		t.Error("roadmap missing title")
	}

	if !strings.Contains(roadmapStr, "## Ready") {
		t.Error("roadmap missing Ready section")
	}

	if !strings.Contains(roadmapStr, "## In Progress") {
		t.Error("roadmap missing In Progress section")
	}

	if !strings.Contains(roadmapStr, "## Blocked") {
		t.Error("roadmap missing Blocked section")
	}

	if !strings.Contains(roadmapStr, "## Completed") {
		t.Error("roadmap missing Completed section")
	}

	// Verify counts
	if !strings.Contains(roadmapStr, "**Count**: 1 beads ready") {
		t.Error("roadmap has incorrect ready count")
	}

	if !strings.Contains(roadmapStr, "**Count**: 1 beads completed") {
		t.Error("roadmap has incorrect completed count")
	}

	// Verify in-progress list
	if !strings.Contains(roadmapStr, "**bead-2**: Second bead") {
		t.Error("roadmap missing in-progress bead")
	}

	if !strings.Contains(roadmapStr, "session: session-1") {
		t.Error("roadmap missing session info")
	}
}

func TestGenerateRoadmap_EmptyQueue(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty task queue
	queueFile := filepath.Join(tmpDir, "queue.yaml")
	queueData := `schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready: []
in_progress: []
blocked: []
completed: []
`
	if err := os.WriteFile(queueFile, []byte(queueData), 0644); err != nil {
		t.Fatalf("failed to create test queue: %v", err)
	}

	// Generate roadmap
	roadmapFile := filepath.Join(tmpDir, "ROADMAP.md")
	if err := GenerateRoadmap(queueFile, roadmapFile); err != nil {
		t.Fatalf("GenerateRoadmap failed: %v", err)
	}

	// Read content
	content, err := os.ReadFile(roadmapFile)
	if err != nil {
		t.Fatalf("failed to read roadmap file: %v", err)
	}

	roadmapStr := string(content)

	// Verify empty state messages
	if !strings.Contains(roadmapStr, "*No beads currently in progress*") {
		t.Error("roadmap missing empty in-progress message")
	}

	if !strings.Contains(roadmapStr, "*No blocked beads*") {
		t.Error("roadmap missing empty blocked message")
	}
}

func TestGenerateRoadmap_BlockedWithReasons(t *testing.T) {
	tmpDir := t.TempDir()

	// Create task queue with blocked beads
	queueFile := filepath.Join(tmpDir, "queue.yaml")
	queueData := `schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready: []
in_progress: []
blocked:
  - id: bead-blocked
    title: Blocked bead
    tier: 1
    prompts:
      start: "Do blocked task"
    metadata:
      escalation_reason: "Waiting for dependency"
completed: []
`
	if err := os.WriteFile(queueFile, []byte(queueData), 0644); err != nil {
		t.Fatalf("failed to create test queue: %v", err)
	}

	// Generate roadmap
	roadmapFile := filepath.Join(tmpDir, "ROADMAP.md")
	if err := GenerateRoadmap(queueFile, roadmapFile); err != nil {
		t.Fatalf("GenerateRoadmap failed: %v", err)
	}

	// Read content
	content, err := os.ReadFile(roadmapFile)
	if err != nil {
		t.Fatalf("failed to read roadmap file: %v", err)
	}

	roadmapStr := string(content)

	// Verify blocked bead with reason
	if !strings.Contains(roadmapStr, "**bead-blocked**: Blocked bead") {
		t.Error("roadmap missing blocked bead")
	}

	if !strings.Contains(roadmapStr, "Reason: Waiting for dependency") {
		t.Error("roadmap missing block reason")
	}
}

func TestTruncateWithEllipsis(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		wantEnd bool // Should end with "..."
	}{
		{"no truncation", "short text", 100, false},
		{"exact length", "exact", 5, false},
		{"truncate simple", "this is a very long text that needs truncation", 20, true},
		{"truncate at newline", "line1\nline2\nline3\nline4\nline5", 20, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateWithEllipsis(tt.content, tt.maxLen)

			// Allow slight overage due to newline truncation logic
			// But result should be close to maxLen (within 10 chars for newline boundary)
			if len(result) > tt.maxLen+10 {
				t.Errorf("truncated length %d significantly exceeds max %d", len(result), tt.maxLen)
			}

			if tt.wantEnd && !strings.HasSuffix(result, "...") {
				t.Error("truncated text should end with '...'")
			}

			if !tt.wantEnd && strings.HasSuffix(result, "...") {
				t.Error("non-truncated text should not end with '...'")
			}
		})
	}
}

func TestGenerateRoadmap_TokenLimit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create queue with many beads to exceed token limit
	queueFile := filepath.Join(tmpDir, "queue.yaml")

	// Build large queue
	var queueData strings.Builder
	queueData.WriteString(`schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready: []
in_progress:
`)

	// Add many in-progress beads to exceed token limit
	for i := 0; i < 100; i++ {
		queueData.WriteString("  - id: bead-" + string(rune('0'+i%10)) + "\n")
		queueData.WriteString("    title: This is a very long title for bead " + string(rune('0'+i%10)) + " with lots of text to increase token count\n")
		queueData.WriteString("    tier: 1\n")
		queueData.WriteString("    prompts:\n")
		queueData.WriteString("      start: \"Long prompt text here\"\n")
		queueData.WriteString("    metadata:\n")
		queueData.WriteString("      session_name: \"session-" + string(rune('0'+i%10)) + "\"\n")
		queueData.WriteString("      iterations: 1\n")
	}

	queueData.WriteString("blocked: []\ncompleted: []\n")

	if err := os.WriteFile(queueFile, []byte(queueData.String()), 0644); err != nil {
		t.Fatalf("failed to create test queue: %v", err)
	}

	// Generate roadmap
	roadmapFile := filepath.Join(tmpDir, "ROADMAP.md")
	if err := GenerateRoadmap(queueFile, roadmapFile); err != nil {
		t.Fatalf("GenerateRoadmap failed: %v", err)
	}

	// Read content
	content, err := os.ReadFile(roadmapFile)
	if err != nil {
		t.Fatalf("failed to read roadmap file: %v", err)
	}

	// Verify token limit enforced
	if len(content) > MaxRoadmapChars {
		t.Errorf("roadmap exceeds token limit: got %d chars, max %d", len(content), MaxRoadmapChars)
	}

	// Verify truncation marker present
	if !strings.HasSuffix(string(content), "...") {
		t.Error("truncated roadmap should end with '...'")
	}
}

func TestGenerateRoadmapMarkdown(t *testing.T) {
	queue := &taskqueue.TaskQueue{
		SchemaVersion: "1.0.0",
		LastUpdated:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Ready: []taskqueue.Bead{
			{ID: "bead-1", Title: "Ready bead", Tier: 1},
		},
		InProgress: []taskqueue.Bead{
			{ID: "bead-2", Title: "In progress bead", Tier: 2, Metadata: taskqueue.BeadMetadata{SessionName: "session-1", Iterations: 2}},
		},
		Blocked: []taskqueue.Bead{
			{ID: "bead-3", Title: "Blocked bead", Tier: 1, Metadata: taskqueue.BeadMetadata{EscalationReason: "dependency missing"}},
		},
		Completed: []taskqueue.Bead{
			{ID: "bead-4", Title: "Completed bead", Tier: 1},
			{ID: "bead-5", Title: "Another completed", Tier: 2},
		},
	}

	markdown := generateRoadmapMarkdown(queue)

	// Verify all sections present
	if !strings.Contains(markdown, "## Ready") {
		t.Error("missing Ready section")
	}

	if !strings.Contains(markdown, "## In Progress") {
		t.Error("missing In Progress section")
	}

	if !strings.Contains(markdown, "## Blocked") {
		t.Error("missing Blocked section")
	}

	if !strings.Contains(markdown, "## Completed") {
		t.Error("missing Completed section")
	}

	// Verify progress calculation
	// Total: 5 beads, 2 completed = 40%
	if !strings.Contains(markdown, "2/5 (40%) completed") {
		t.Error("incorrect progress calculation")
	}
}
