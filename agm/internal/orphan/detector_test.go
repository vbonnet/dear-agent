package orphan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Phase 6 (2026-03-18): TestDetectOrphans removed - tested obsolete YAML manifest loading
// Function loadManifestUUIDs() deleted in YAML backend removal

func TestInferSessionName(t *testing.T) {
	tests := []struct {
		name         string
		projectPath  string
		expectedName string
	}{
		{
			name:         "normal project path",
			projectPath:  "~/src/my-project",
			expectedName: "my-project",
		},
		{
			name:         "nested project path",
			projectPath:  "~/src/ws/oss/repos/ai-tools",
			expectedName: "ai-tools",
		},
		{
			name:         "empty path",
			projectPath:  "",
			expectedName: "unknown-session",
		},
		{
			name:         "root path",
			projectPath:  "/",
			expectedName: "unknown-session",
		},
		{
			name:         "current directory",
			projectPath:  ".",
			expectedName: "unknown-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferSessionName(tt.projectPath)
			if result != tt.expectedName {
				t.Errorf("Expected %q, got %q", tt.expectedName, result)
			}
		})
	}
}

func TestInferWorkspaceFromPath(t *testing.T) {
	tests := []struct {
		name              string
		projectPath       string
		expectedWorkspace string
	}{
		{
			name:              "oss workspace",
			projectPath:       "~/src/ws/oss/repos/ai-tools",
			expectedWorkspace: "oss",
		},
		{
			name:              "acme workspace",
			projectPath:       "~/src/ws/acme/project",
			expectedWorkspace: "acme",
		},
		{
			name:              "research workspace",
			projectPath:       "~/src/ws/research/experiment",
			expectedWorkspace: "research",
		},
		{
			name:              "no workspace pattern",
			projectPath:       "~/src/my-project",
			expectedWorkspace: "",
		},
		{
			name:              "empty path",
			projectPath:       "",
			expectedWorkspace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferWorkspaceFromPath(tt.projectPath)
			if result != tt.expectedWorkspace {
				t.Errorf("Expected workspace %q, got %q", tt.expectedWorkspace, result)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		expectedParts []string
	}{
		{
			name:          "unix path",
			path:          "/home/alice/src/ws/oss",
			expectedParts: []string{"home", "alice", "src", "ws", "oss"},
		},
		{
			name:          "relative path",
			path:          "src/ws/oss",
			expectedParts: []string{"src", "ws", "oss"},
		},
		{
			name:          "empty path",
			path:          "",
			expectedParts: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitPath(tt.path)
			if len(result) != len(tt.expectedParts) {
				t.Errorf("Expected %d parts, got %d", len(tt.expectedParts), len(result))
				return
			}
			for i, expected := range tt.expectedParts {
				if result[i] != expected {
					t.Errorf("Part %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestOrphanedSessionStruct(t *testing.T) {
	now := time.Now()
	orphan := &OrphanedSession{
		UUID:             "test-uuid-123",
		ProjectPath:      "~/src/test",
		LastModified:     now,
		ConversationPath: "~/.claude/projects/abc/test-uuid-123.jsonl",
		DetectedAt:       now,
		DetectionMethod:  DetectionMethodHistory,
		InferredName:     "test",
		HasConversation:  true,
		Workspace:        "oss",
		Status:           StatusOrphaned,
		ImportedAt:       nil,
	}

	// Validate struct fields
	if orphan.UUID != "test-uuid-123" {
		t.Errorf("Expected UUID %q, got %q", "test-uuid-123", orphan.UUID)
	}

	if orphan.Status != StatusOrphaned {
		t.Errorf("Expected status %q, got %q", StatusOrphaned, orphan.Status)
	}

	if orphan.DetectionMethod != DetectionMethodHistory {
		t.Errorf("Expected method %q, got %q", DetectionMethodHistory, orphan.DetectionMethod)
	}

	if !orphan.HasConversation {
		t.Error("Expected HasConversation to be true")
	}

	if orphan.ImportedAt != nil {
		t.Error("Expected ImportedAt to be nil for orphaned session")
	}
}

func TestDetectionReport(t *testing.T) {
	report := &OrphanDetectionReport{
		ScanStarted:       time.Now(),
		ScanCompleted:     time.Now().Add(time.Second),
		WorkspacesScanned: []string{"oss", "acme"},
		Orphans: []*OrphanedSession{
			{UUID: "orphan-1", Workspace: "oss"},
			{UUID: "orphan-2", Workspace: "oss"},
			{UUID: "orphan-3", Workspace: "acme"},
		},
		TotalOrphans: 3,
		ByWorkspace: map[string]int{
			"oss":  2,
			"acme": 1,
		},
		HistoryEntries: 100,
		ProjectsFound:  50,
		ManifestsFound: 47,
		Errors:         []DetectionError{},
	}

	// Validate report structure
	if report.TotalOrphans != 3 {
		t.Errorf("Expected 3 total orphans, got %d", report.TotalOrphans)
	}

	if report.ByWorkspace["oss"] != 2 {
		t.Errorf("Expected 2 orphans in oss workspace, got %d", report.ByWorkspace["oss"])
	}

	if report.ByWorkspace["acme"] != 1 {
		t.Errorf("Expected 1 orphan in acme workspace, got %d", report.ByWorkspace["acme"])
	}

	if len(report.Errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(report.Errors))
	}
}

// Helper function to copy manifest files for testing
func copyManifest(t *testing.T, srcDir, srcFile, dstDir, sessionID string) {
	t.Helper()

	// Read source manifest
	srcPath := filepath.Join(srcDir, srcFile)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("Failed to read manifest %s: %v", srcPath, err)
	}

	// Create session directory
	sessionDir := filepath.Join(dstDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Write to destination
	dstPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		t.Fatalf("Failed to write manifest %s: %v", dstPath, err)
	}
}
