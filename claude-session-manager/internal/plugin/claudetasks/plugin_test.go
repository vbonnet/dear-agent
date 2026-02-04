package claudetasks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/plugin"
)

func TestMetadata(t *testing.T) {
	p := NewPlugin()
	metadata := p.Metadata()

	if metadata.Name != "claude-tasks" {
		t.Errorf("Name = %q, want %q", metadata.Name, "claude-tasks")
	}
	if metadata.Version == "" {
		t.Error("Version is empty")
	}
	if metadata.Author == "" {
		t.Error("Author is empty")
	}
	if metadata.Description == "" {
		t.Error("Description is empty")
	}
}

func TestSupportsSession(t *testing.T) {
	p := NewPlugin()

	tests := []struct {
		name        string
		setupFunc   func(string)
		wantSupport bool
	}{
		{
			name: "session with ROADMAP.md",
			setupFunc: func(dir string) {
				os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("# Roadmap"), 0644)
			},
			wantSupport: true,
		},
		{
			name: "session without ROADMAP.md",
			setupFunc: func(dir string) {
				// Don't create ROADMAP.md
			},
			wantSupport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setupFunc(tempDir)

			got := p.SupportsSession(tempDir)
			if got != tt.wantSupport {
				t.Errorf("SupportsSession() = %v, want %v", got, tt.wantSupport)
			}
		})
	}
}

func TestGetTasks(t *testing.T) {
	p := NewPlugin()

	tests := []struct {
		name      string
		content   string
		wantCount int
		wantErr   bool
	}{
		{
			name: "valid ROADMAP with tasks",
			content: `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | First task | 1 day | ✅ COMPLETE |
| ` + "`oss-task-2`" + ` | Second task | 2 hours | 📋 PLANNED |

## Phase 1: Implementation

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-3`" + ` | Third task | 1 week | 🔄 IN_PROGRESS |
`,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "empty ROADMAP",
			content:   "# Empty Roadmap\n\nNo tasks here.",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "ROADMAP with orphan tasks",
			content: `# Test ROADMAP

Some intro text

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-orphan`" + ` | Orphan task | 1 day | ✅ COMPLETE |
`,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			roadmapPath := filepath.Join(tempDir, "ROADMAP.md")
			if err := os.WriteFile(roadmapPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test ROADMAP: %v", err)
			}

			tasks, err := p.GetTasks(tempDir)
			if tt.wantErr {
				if err == nil {
					t.Error("GetTasks() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetTasks() unexpected error: %v", err)
				return
			}

			if len(tasks) != tt.wantCount {
				t.Errorf("GetTasks() returned %d tasks, want %d", len(tasks), tt.wantCount)
			}

			// Verify task structure
			for _, task := range tasks {
				if task.ID == "" {
					t.Error("Task has empty ID")
				}
				if task.Status == "" {
					t.Error("Task has empty Status")
				}
				if task.Phase == "" {
					t.Error("Task has empty Phase")
				}
			}
		})
	}
}

func TestGetTasks_NoROADMAP(t *testing.T) {
	p := NewPlugin()
	tempDir := t.TempDir()

	// No ROADMAP.md file
	tasks, err := p.GetTasks(tempDir)
	if err != nil {
		t.Errorf("GetTasks() with no ROADMAP should not error, got: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("GetTasks() with no ROADMAP returned %d tasks, want 0", len(tasks))
	}
}

func TestGetTasks_TaskFields(t *testing.T) {
	p := NewPlugin()
	tempDir := t.TempDir()

	content := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-test-123`" + ` | Test task description | 2 days | ✅ COMPLETE |
`

	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")
	if err := os.WriteFile(roadmapPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test ROADMAP: %v", err)
	}

	tasks, err := p.GetTasks(tempDir)
	if err != nil {
		t.Fatalf("GetTasks() error: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("GetTasks() returned %d tasks, want 1", len(tasks))
	}

	task := tasks[0]

	// Verify fields
	if task.ID != "oss-test-123" {
		t.Errorf("Task.ID = %q, want %q", task.ID, "oss-test-123")
	}
	if task.Status != "completed" {
		t.Errorf("Task.Status = %q, want %q", task.Status, "completed")
	}
	if task.Phase != "phase-0" {
		t.Errorf("Task.Phase = %q, want %q", task.Phase, "phase-0")
	}
	if task.Metadata["effort"] != "2 days" {
		t.Errorf("Task.Metadata[effort] = %q, want %q", task.Metadata["effort"], "2 days")
	}
	if task.Metadata["source"] != "roadmap" {
		t.Errorf("Task.Metadata[source] = %q, want %q", task.Metadata["source"], "roadmap")
	}
}

func TestGetPhaseProgress(t *testing.T) {
	p := NewPlugin()
	tempDir := t.TempDir()

	content := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task 1 | 1 day | ✅ COMPLETE |
| ` + "`oss-task-2`" + ` | Task 2 | 1 day | ✅ COMPLETE |
| ` + "`oss-task-3`" + ` | Task 3 | 1 day | 📋 PLANNED |
| ` + "`oss-task-4`" + ` | Task 4 | 1 day | 🔄 IN_PROGRESS |

## Phase 1: Implementation

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-5`" + ` | Task 5 | 1 week | 📋 PLANNED |
| ` + "`oss-task-6`" + ` | Task 6 | 1 week | 📋 PLANNED |
`

	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")
	if err := os.WriteFile(roadmapPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test ROADMAP: %v", err)
	}

	stats, err := p.GetPhaseProgress(tempDir)
	if err != nil {
		t.Fatalf("GetPhaseProgress() error: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("GetPhaseProgress() returned %d phases, want 2", len(stats))
	}

	// Find phase-0 stats
	var phase0 *plugin.PhaseStats
	for i := range stats {
		if stats[i].Phase == "phase-0" {
			phase0 = &stats[i]
			break
		}
	}

	if phase0 == nil {
		t.Fatal("phase-0 not found in stats")
	}

	// Verify phase-0 stats
	if phase0.Total != 4 {
		t.Errorf("phase-0 Total = %d, want 4", phase0.Total)
	}
	if phase0.Completed != 2 {
		t.Errorf("phase-0 Completed = %d, want 2", phase0.Completed)
	}
	if phase0.InProgress != 1 {
		t.Errorf("phase-0 InProgress = %d, want 1", phase0.InProgress)
	}
	if phase0.Pending != 1 {
		t.Errorf("phase-0 Pending = %d, want 1", phase0.Pending)
	}

	// Verify percentage calculation
	expectedPercentage := 50.0 // 2 completed out of 4 total
	if phase0.Percentage != expectedPercentage {
		t.Errorf("phase-0 Percentage = %f, want %f", phase0.Percentage, expectedPercentage)
	}
}

func TestGetPhaseProgress_NoTasks(t *testing.T) {
	p := NewPlugin()
	tempDir := t.TempDir()

	content := `# Empty ROADMAP

No tasks here.
`

	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")
	if err := os.WriteFile(roadmapPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test ROADMAP: %v", err)
	}

	stats, err := p.GetPhaseProgress(tempDir)
	if err != nil {
		t.Fatalf("GetPhaseProgress() error: %v", err)
	}

	if len(stats) != 0 {
		t.Errorf("GetPhaseProgress() with no tasks returned %d phases, want 0", len(stats))
	}
}
