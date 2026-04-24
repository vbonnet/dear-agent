package heartbeat

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultStatePath(t *testing.T) {
	path := DefaultStatePath()
	if path == "" {
		t.Error("DefaultStatePath() should return a non-empty path")
	}
	if filepath.Base(path) != "heartbeat-state.json" {
		t.Errorf("DefaultStatePath() = %q, want filename heartbeat-state.json", path)
	}
}

func TestLoadState_MissingFile(t *testing.T) {
	state := LoadState("/nonexistent/path/heartbeat-state.json")
	if state.Batch != nil {
		t.Error("LoadState with missing file should return nil Batch")
	}
	if state.Operation != nil {
		t.Error("LoadState with missing file should return nil Operation")
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	state := LoadState(path)
	if state.Batch != nil {
		t.Error("LoadState with invalid JSON should return nil Batch")
	}
	if state.Operation != nil {
		t.Error("LoadState with invalid JSON should return nil Operation")
	}
}

func TestSaveState_LoadState_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "heartbeat-state.json")

	now := time.Now().Truncate(time.Second)
	original := State{
		Batch: &BatchState{
			Count:          5,
			ToolType:       "Edit",
			StartedAt:      now,
			LastFile:       "/tmp/test.go",
			FilesProcessed: []string{"/tmp/a.go", "/tmp/b.go"},
		},
		Operation: &OperationState{
			ToolName:       "Bash",
			StartedAt:      now,
			CommandPreview: "go test ./...",
		},
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	loaded := LoadState(path)
	if loaded.Batch == nil {
		t.Fatal("Loaded state Batch should not be nil")
	}
	if loaded.Batch.Count != 5 {
		t.Errorf("Batch.Count = %d, want 5", loaded.Batch.Count)
	}
	if loaded.Batch.ToolType != "Edit" {
		t.Errorf("Batch.ToolType = %q, want Edit", loaded.Batch.ToolType)
	}
	if loaded.Batch.LastFile != "/tmp/test.go" {
		t.Errorf("Batch.LastFile = %q, want /tmp/test.go", loaded.Batch.LastFile)
	}
	if len(loaded.Batch.FilesProcessed) != 2 {
		t.Errorf("Batch.FilesProcessed len = %d, want 2", len(loaded.Batch.FilesProcessed))
	}
	if loaded.Operation == nil {
		t.Fatal("Loaded state Operation should not be nil")
	}
	if loaded.Operation.ToolName != "Bash" {
		t.Errorf("Operation.ToolName = %q, want Bash", loaded.Operation.ToolName)
	}
	if loaded.Operation.CommandPreview != "go test ./..." {
		t.Errorf("Operation.CommandPreview = %q, want 'go test ./...'", loaded.Operation.CommandPreview)
	}
}

func TestSaveState_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "dir", "heartbeat-state.json")

	err := SaveState(path, State{})
	if err != nil {
		t.Fatalf("SaveState() should create parent dirs, got error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("State file should exist at %q", path)
	}
}

func TestRecordToolUse_IncrementsForFileOps(t *testing.T) {
	fileTools := []string{"Edit", "Write", "Read", "NotebookEdit"}

	for _, tool := range fileTools {
		t.Run(tool, func(t *testing.T) {
			state := State{}
			params := map[string]interface{}{"file_path": "/tmp/test.go"}
			state.RecordToolUse(tool, params)

			if state.Batch == nil {
				t.Fatal("Batch should not be nil after file op")
			}
			if state.Batch.Count != 1 {
				t.Errorf("Batch.Count = %d, want 1", state.Batch.Count)
			}
			if state.Batch.ToolType != tool {
				t.Errorf("Batch.ToolType = %q, want %q", state.Batch.ToolType, tool)
			}
		})
	}
}

func TestRecordToolUse_ResetsForNonFileOps(t *testing.T) {
	state := State{}
	params := map[string]interface{}{"file_path": "/tmp/test.go"}

	// Build up a batch
	state.RecordToolUse("Edit", params)
	state.RecordToolUse("Edit", params)
	state.RecordToolUse("Edit", params)

	if state.Batch == nil || state.Batch.Count != 3 {
		t.Fatal("Should have batch count of 3")
	}

	// Non-file op resets
	state.RecordToolUse("Bash", map[string]interface{}{"command": "echo hi"})
	if state.Batch != nil {
		t.Error("Batch should be nil after non-file op")
	}
}

func TestRecordToolUse_NonFileTools(t *testing.T) {
	nonFileTools := []string{"Bash", "Agent", "Skill", "WebSearch", "WebFetch"}

	for _, tool := range nonFileTools {
		t.Run(tool, func(t *testing.T) {
			state := State{
				Batch: &BatchState{Count: 5, ToolType: "Edit"},
			}
			state.RecordToolUse(tool, map[string]interface{}{})
			if state.Batch != nil {
				t.Error("Non-file tool should reset batch")
			}
		})
	}
}

func TestIsInBatch_Threshold(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected bool
	}{
		{"count 0 - not in batch", 0, false},
		{"count 1 - not in batch", 1, false},
		{"count 2 - not in batch", 2, false},
		{"count 3 - in batch", 3, true},
		{"count 10 - in batch", 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := State{}
			params := map[string]interface{}{"file_path": "/tmp/test.go"}
			for i := 0; i < tt.count; i++ {
				state.RecordToolUse("Edit", params)
			}

			if state.IsInBatch() != tt.expected {
				t.Errorf("IsInBatch() = %v after %d ops, want %v", state.IsInBatch(), tt.count, tt.expected)
			}
		})
	}
}

func TestIsInBatch_NilBatch(t *testing.T) {
	state := State{}
	if state.IsInBatch() {
		t.Error("IsInBatch() should be false with nil Batch")
	}
}

func TestFilesProcessed_CapAt20(t *testing.T) {
	state := State{}

	for i := 0; i < 25; i++ {
		params := map[string]interface{}{"file_path": filepath.Join("/tmp", "file"+string(rune('a'+i))+".go")}
		state.RecordToolUse("Edit", params)
	}

	if len(state.Batch.FilesProcessed) != 20 {
		t.Errorf("FilesProcessed len = %d, want 20 (capped)", len(state.Batch.FilesProcessed))
	}
}

func TestRecordToolUse_ExtractsFilePath(t *testing.T) {
	state := State{}
	params := map[string]interface{}{"file_path": "/tmp/test/test.go"}
	state.RecordToolUse("Edit", params)

	if state.Batch.LastFile != "/tmp/test/test.go" {
		t.Errorf("LastFile = %q, want /tmp/test/test.go", state.Batch.LastFile)
	}
	if len(state.Batch.FilesProcessed) != 1 || state.Batch.FilesProcessed[0] != "/tmp/test/test.go" {
		t.Error("FilesProcessed should contain the file path")
	}
}

func TestRecordToolUse_NoFilePathParam(t *testing.T) {
	state := State{}
	params := map[string]interface{}{"content": "some data"}
	state.RecordToolUse("Edit", params)

	if state.Batch.LastFile != "" {
		t.Errorf("LastFile should be empty when no file_path param, got %q", state.Batch.LastFile)
	}
}

func TestRecordOperationStart(t *testing.T) {
	state := State{}
	state.RecordOperationStart("Bash", "go test ./...")

	if state.Operation == nil {
		t.Fatal("Operation should not be nil")
	}
	if state.Operation.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want Bash", state.Operation.ToolName)
	}
	if state.Operation.CommandPreview != "go test ./..." {
		t.Errorf("CommandPreview = %q, want 'go test ./...'", state.Operation.CommandPreview)
	}
	if state.Operation.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
}

func TestRecordOperationStart_TruncatesLongPreview(t *testing.T) {
	state := State{}
	longCmd := ""
	for i := 0; i < 100; i++ {
		longCmd += "x"
	}
	state.RecordOperationStart("Bash", longCmd)

	if len(state.Operation.CommandPreview) != 80 {
		t.Errorf("CommandPreview len = %d, want 80", len(state.Operation.CommandPreview))
	}
}

func TestGetOperationDuration(t *testing.T) {
	state := State{}

	// No operation
	if d := state.GetOperationDuration(); d != 0 {
		t.Errorf("GetOperationDuration() = %v with nil Operation, want 0", d)
	}

	// With operation
	state.Operation = &OperationState{
		ToolName:  "Bash",
		StartedAt: time.Now().Add(-45 * time.Second),
	}

	d := state.GetOperationDuration()
	if d < 44*time.Second || d > 46*time.Second {
		t.Errorf("GetOperationDuration() = %v, want ~45s", d)
	}
}

func TestResetBatch(t *testing.T) {
	state := State{
		Batch: &BatchState{Count: 5, ToolType: "Edit"},
	}
	state.ResetBatch()
	if state.Batch != nil {
		t.Error("ResetBatch() should set Batch to nil")
	}
}
