package roadmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPhaseHeader(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "phase with colon",
			line:     "## Phase 0: Data Collection",
			expected: "phase-0",
		},
		{
			name:     "phase with dash",
			line:     "## Phase 1 - Task Management",
			expected: "phase-1",
		},
		{
			name:     "phase with extra spaces",
			line:     "##   Phase 2:  Hooks",
			expected: "phase-2",
		},
		{
			name:     "not a phase header",
			line:     "## Introduction",
			expected: "",
		},
		{
			name:     "task header",
			line:     "### Task 0.1: Parser",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectPhaseHeader(tt.line)
			if got != tt.expected {
				t.Errorf("detectPhaseHeader(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}

func TestExtractBeadID(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "simple bead ID",
			text:     "`oss-roadmap-parser`",
			expected: "oss-roadmap-parser",
		},
		{
			name:     "bead ID with numbers",
			text:     "`oss-task-123`",
			expected: "oss-task-123",
		},
		{
			name:     "bead ID in sentence",
			text:     "The bead `oss-abc-def` handles parsing",
			expected: "oss-abc-def",
		},
		{
			name:     "no bead ID",
			text:     "Some text without backticks",
			expected: "",
		},
		{
			name:     "wrong prefix",
			text:     "`abc-test`",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBeadID(tt.text)
			if got != tt.expected {
				t.Errorf("extractBeadID(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		name      string
		rawStatus string
		expected  string
	}{
		// Completed variants
		{
			name:      "checkmark emoji",
			rawStatus: "✅ COMPLETE",
			expected:  "completed",
		},
		{
			name:      "complete lowercase",
			rawStatus: "complete",
			expected:  "completed",
		},
		{
			name:      "closed",
			rawStatus: "CLOSED",
			expected:  "completed",
		},
		{
			name:      "done",
			rawStatus: "done",
			expected:  "completed",
		},
		// In progress variants
		{
			name:      "circling emoji",
			rawStatus: "🔄 IN_PROGRESS",
			expected:  "in_progress",
		},
		{
			name:      "in progress with space",
			rawStatus: "in progress",
			expected:  "in_progress",
		},
		{
			name:      "working",
			rawStatus: "WORKING",
			expected:  "in_progress",
		},
		// Pending variants
		{
			name:      "clipboard emoji",
			rawStatus: "📋 PLANNED",
			expected:  "pending",
		},
		{
			name:      "planned",
			rawStatus: "planned",
			expected:  "pending",
		},
		{
			name:      "open",
			rawStatus: "OPEN",
			expected:  "pending",
		},
		{
			name:      "todo",
			rawStatus: "TODO",
			expected:  "pending",
		},
		// Blocked
		{
			name:      "pause emoji",
			rawStatus: "⏸ BLOCKED",
			expected:  "blocked",
		},
		{
			name:      "paused",
			rawStatus: "paused",
			expected:  "blocked",
		},
		// Cancelled
		{
			name:      "x emoji",
			rawStatus: "❌ CANCELLED",
			expected:  "cancelled",
		},
		{
			name:      "abandoned",
			rawStatus: "ABANDONED",
			expected:  "cancelled",
		},
		// Edge cases
		{
			name:      "empty status",
			rawStatus: "",
			expected:  "pending",
		},
		{
			name:      "mixed case",
			rawStatus: "CoMpLeTe",
			expected:  "completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeStatus(tt.rawStatus)
			if got != tt.expected {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tt.rawStatus, got, tt.expected)
			}
		})
	}
}

func TestExtractBeadFromLine(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		lineNum      int
		currentPhase string
		wantNil      bool
		wantID       string
		wantPhase    string
		wantStatus   string
	}{
		{
			name:         "valid table row",
			line:         "| `oss-roadmap-parser` | ROADMAP.md parser | 2 days | ✅ COMPLETE |",
			lineNum:      42,
			currentPhase: "phase-0",
			wantNil:      false,
			wantID:       "oss-roadmap-parser",
			wantPhase:    "phase-0",
			wantStatus:   "completed",
		},
		{
			name:         "table row with extra spaces",
			line:         "|  `oss-task-123`  |  Description here  |  1 day  |  📋 PLANNED  |",
			lineNum:      10,
			currentPhase: "phase-1",
			wantNil:      false,
			wantID:       "oss-task-123",
			wantPhase:    "phase-1",
			wantStatus:   "pending",
		},
		{
			name:         "no phase assigned",
			line:         "| `oss-orphan` | No phase | 1 hour | OPEN |",
			lineNum:      5,
			currentPhase: "",
			wantNil:      false,
			wantID:       "oss-orphan",
			wantPhase:    "phase-unknown",
			wantStatus:   "pending",
		},
		{
			name:         "table separator",
			line:         "|---------|-------------|--------|--------|",
			lineNum:      20,
			currentPhase: "phase-0",
			wantNil:      true,
		},
		{
			name:         "header row",
			line:         "| Bead ID | Description | Effort | Status |",
			lineNum:      19,
			currentPhase: "phase-0",
			wantNil:      true,
		},
		{
			name:         "not a table line",
			line:         "This is just regular text",
			lineNum:      1,
			currentPhase: "phase-0",
			wantNil:      true,
		},
		{
			name:         "empty line",
			line:         "",
			lineNum:      50,
			currentPhase: "phase-0",
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBeadFromLine(tt.line, tt.lineNum, tt.currentPhase)

			if tt.wantNil {
				if got != nil {
					t.Errorf("extractBeadFromLine() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("extractBeadFromLine() = nil, want non-nil")
			}

			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
			if got.Phase != tt.wantPhase {
				t.Errorf("Phase = %q, want %q", got.Phase, tt.wantPhase)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.LineNumber != tt.lineNum {
				t.Errorf("LineNumber = %d, want %d", got.LineNumber, tt.lineNum)
			}
		})
	}
}

func TestParseROADMAP(t *testing.T) {
	// Create a temporary ROADMAP.md for testing
	tempDir := t.TempDir()
	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")

	content := `# Test ROADMAP

## Phase 0: Data Collection

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | First task | 1 day | ✅ COMPLETE |
| ` + "`oss-task-2`" + ` | Second task | 2 hours | 🔄 IN_PROGRESS |

## Phase 1: Processing

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-3`" + ` | Third task | 1 week | 📋 PLANNED |

Some text outside tables

| ` + "`oss-task-4`" + ` | Fourth task | 3 days | ❌ CANCELLED |
`

	if err := os.WriteFile(roadmapPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test ROADMAP: %v", err)
	}

	beads, err := ParseROADMAP(roadmapPath)
	if err != nil {
		t.Fatalf("ParseROADMAP() error = %v", err)
	}

	if len(beads) != 4 {
		t.Fatalf("ParseROADMAP() returned %d beads, want 4", len(beads))
	}

	// Verify Phase 0 tasks
	if beads[0].ID != "oss-task-1" {
		t.Errorf("beads[0].ID = %q, want %q", beads[0].ID, "oss-task-1")
	}
	if beads[0].Phase != "phase-0" {
		t.Errorf("beads[0].Phase = %q, want %q", beads[0].Phase, "phase-0")
	}
	if beads[0].Status != "completed" {
		t.Errorf("beads[0].Status = %q, want %q", beads[0].Status, "completed")
	}

	if beads[1].ID != "oss-task-2" {
		t.Errorf("beads[1].ID = %q, want %q", beads[1].ID, "oss-task-2")
	}
	if beads[1].Status != "in_progress" {
		t.Errorf("beads[1].Status = %q, want %q", beads[1].Status, "in_progress")
	}

	// Verify Phase 1 task
	if beads[2].ID != "oss-task-3" {
		t.Errorf("beads[2].ID = %q, want %q", beads[2].ID, "oss-task-3")
	}
	if beads[2].Phase != "phase-1" {
		t.Errorf("beads[2].Phase = %q, want %q", beads[2].Phase, "phase-1")
	}
	if beads[2].Status != "pending" {
		t.Errorf("beads[2].Status = %q, want %q", beads[2].Status, "pending")
	}

	// Verify cancelled task
	if beads[3].Status != "cancelled" {
		t.Errorf("beads[3].Status = %q, want %q", beads[3].Status, "cancelled")
	}
}

func TestParseROADMAP_ErrorHandling(t *testing.T) {
	// Test with non-existent file
	_, err := ParseROADMAP("/nonexistent/path/ROADMAP.md")
	if err == nil {
		t.Error("ParseROADMAP() with non-existent file should return error")
	}
}
