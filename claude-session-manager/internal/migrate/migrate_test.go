package migrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

func TestReadYAMLManifests_EmptyDirectory(t *testing.T) {
	// Create temporary empty directory
	tmpDir := t.TempDir()

	manifests, err := ReadYAMLManifests(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
}

func TestReadYAMLManifests_NonexistentDirectory(t *testing.T) {
	manifests, err := ReadYAMLManifests("/nonexistent/directory")
	if err != nil {
		t.Fatalf("expected no error for nonexistent directory, got: %v", err)
	}

	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
}

func TestReadYAMLManifests_ValidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test YAML files
	testManifests := []struct {
		filename string
		content  string
	}{
		{
			filename: "session-001.yaml",
			content: `schema_version: "2.0"
session_id: "session-001"
name: "test-session-1"
created_at: "2026-01-01T10:00:00Z"
updated_at: "2026-01-01T10:00:00Z"
lifecycle: ""
agent: "claude"
context:
  project: "/tmp/project1"
  purpose: "Testing migration"
  tags: ["test", "migration"]
  notes: "Test notes"
tmux:
  session_name: "test-session-1"
claude:
  uuid: "uuid-001"
`,
		},
		{
			filename: "session-002.yaml",
			content: `schema_version: "2.0"
session_id: "session-002"
name: "test-session-2"
created_at: "2026-01-02T10:00:00Z"
updated_at: "2026-01-02T10:00:00Z"
lifecycle: "archived"
agent: "gemini"
context:
  project: "/tmp/project2"
  purpose: ""
  tags: []
  notes: ""
tmux:
  session_name: "test-session-2"
claude:
  uuid: ""
engram_metadata:
  enabled: true
  query: "test query"
  engram_ids: ["engram-1", "engram-2"]
  loaded_at: "2026-01-02T11:00:00Z"
  count: 2
`,
		},
	}

	for _, tm := range testManifests {
		path := filepath.Join(tmpDir, tm.filename)
		if err := os.WriteFile(path, []byte(tm.content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Read manifests
	manifests, err := ReadYAMLManifests(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(manifests))
	}

	// Verify first manifest
	m1 := manifests[0]
	if m1.SessionID != "session-001" {
		t.Errorf("expected session_id=session-001, got %s", m1.SessionID)
	}
	if m1.Name != "test-session-1" {
		t.Errorf("expected name=test-session-1, got %s", m1.Name)
	}
	if len(m1.Context.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(m1.Context.Tags))
	}

	// Verify second manifest has engram metadata
	m2 := manifests[1]
	if m2.EngramMetadata == nil {
		t.Fatal("expected engram_metadata to be non-nil")
	}
	if !m2.EngramMetadata.Enabled {
		t.Error("expected engram_metadata.enabled=true")
	}
	if m2.EngramMetadata.Count != 2 {
		t.Errorf("expected engram_metadata.count=2, got %d", m2.EngramMetadata.Count)
	}
}

func TestReadYAMLManifests_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid YAML file
	invalidPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create valid YAML file
	validPath := filepath.Join(tmpDir, "valid.yaml")
	validContent := `schema_version: "2.0"
session_id: "valid-session"
name: "valid"
created_at: "2026-01-01T10:00:00Z"
updated_at: "2026-01-01T10:00:00Z"
lifecycle: ""
context:
  project: "/tmp/project"
tmux:
  session_name: "valid"
`
	if err := os.WriteFile(validPath, []byte(validContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Should skip invalid file and only return valid manifest
	manifests, err := ReadYAMLManifests(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(manifests) != 1 {
		t.Errorf("expected 1 manifest (invalid file skipped), got %d", len(manifests))
	}
}

func TestMigrateManifest(t *testing.T) {
	// Create in-memory database
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create test manifest
	now := time.Now().UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-001",
		Name:          "test-session",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     "",
		Agent:         "claude",
		Context: manifest.Context{
			Project: "/tmp/test-project",
			Purpose: "Integration testing",
			Tags:    []string{"test", "integration"},
			Notes:   "Test notes",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-001",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
	}

	// Migrate manifest
	if err := MigrateManifest(database, m); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify it was inserted
	retrieved, err := database.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("failed to retrieve session: %v", err)
	}

	// Validate migration
	if err := ValidateMigration(m, retrieved); err != nil {
		t.Errorf("validation failed: %v", err)
	}
}

func TestMigrateManifest_DuplicateSession(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "duplicate-session",
		Name:          "duplicate",
		CreatedAt:     now,
		UpdatedAt:     now,
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "duplicate",
		},
	}

	// First migration should succeed
	if err := MigrateManifest(database, m); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// Second migration should fail
	if err := MigrateManifest(database, m); err == nil {
		t.Error("expected error for duplicate session, got nil")
	}
}

func TestMigrateAll(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Truncate(time.Second)

	// Create test manifests
	manifests := []*manifest.Manifest{
		{
			SchemaVersion: "2.0",
			SessionID:     "session-1",
			Name:          "session-1",
			CreatedAt:     now,
			UpdatedAt:     now,
			Context: manifest.Context{
				Project: "/tmp/project1",
			},
			Tmux: manifest.Tmux{
				SessionName: "session-1",
			},
		},
		{
			SchemaVersion: "2.0",
			SessionID:     "session-2",
			Name:          "session-2",
			CreatedAt:     now,
			UpdatedAt:     now,
			Context: manifest.Context{
				Project: "/tmp/project2",
			},
			Tmux: manifest.Tmux{
				SessionName: "session-2",
			},
		},
		{
			SchemaVersion: "2.0",
			SessionID:     "session-3",
			Name:          "session-3",
			CreatedAt:     now,
			UpdatedAt:     now,
			Lifecycle:     "archived",
			Context: manifest.Context{
				Project: "/tmp/project3",
			},
			Tmux: manifest.Tmux{
				SessionName: "session-3",
			},
			EngramMetadata: &manifest.EngramMetadata{
				Enabled:   true,
				Query:     "test query",
				EngramIDs: []string{"e1", "e2"},
				LoadedAt:  now,
				Count:     2,
			},
		},
	}

	// Migrate all
	result, err := MigrateAll(database, manifests, false)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify results
	if result.TotalFiles != 3 {
		t.Errorf("expected TotalFiles=3, got %d", result.TotalFiles)
	}
	if result.SuccessCount != 3 {
		t.Errorf("expected SuccessCount=3, got %d", result.SuccessCount)
	}
	if result.ErrorCount != 0 {
		t.Errorf("expected ErrorCount=0, got %d", result.ErrorCount)
	}
	if result.SessionsMigrated != 3 {
		t.Errorf("expected SessionsMigrated=3, got %d", result.SessionsMigrated)
	}

	// Verify all sessions exist in database
	for _, m := range manifests {
		retrieved, err := database.GetSession(m.SessionID)
		if err != nil {
			t.Errorf("failed to retrieve session %s: %v", m.SessionID, err)
			continue
		}

		if err := ValidateMigration(m, retrieved); err != nil {
			t.Errorf("validation failed for %s: %v", m.SessionID, err)
		}
	}
}

func TestMigrateAll_DryRun(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Truncate(time.Second)
	manifests := []*manifest.Manifest{
		{
			SchemaVersion: "2.0",
			SessionID:     "dry-run-session",
			Name:          "dry-run",
			CreatedAt:     now,
			UpdatedAt:     now,
			Context: manifest.Context{
				Project: "/tmp/dryrun",
			},
			Tmux: manifest.Tmux{
				SessionName: "dry-run",
			},
		},
	}

	// Run dry-run migration
	result, err := MigrateAll(database, manifests, true)
	if err != nil {
		t.Fatalf("dry-run migration failed: %v", err)
	}

	if result.SuccessCount != 1 {
		t.Errorf("expected SuccessCount=1, got %d", result.SuccessCount)
	}

	// Verify nothing was actually inserted
	_, err = database.GetSession("dry-run-session")
	if err == nil {
		t.Error("expected session to not exist after dry-run, but it does")
	}
}

func TestMigrateAll_WithErrors(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Truncate(time.Second)

	// Create manifests with one invalid (missing required field)
	manifests := []*manifest.Manifest{
		{
			SchemaVersion: "2.0",
			SessionID:     "valid-session",
			Name:          "valid",
			CreatedAt:     now,
			UpdatedAt:     now,
			Context: manifest.Context{
				Project: "/tmp/valid",
			},
			Tmux: manifest.Tmux{
				SessionName: "valid",
			},
		},
		{
			// Missing SessionID - should fail validation
			SchemaVersion: "2.0",
			SessionID:     "", // Invalid
			Name:          "invalid",
			CreatedAt:     now,
			UpdatedAt:     now,
			Context:       manifest.Context{},
		},
	}

	result, err := MigrateAll(database, manifests, false)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if result.SuccessCount != 1 {
		t.Errorf("expected SuccessCount=1, got %d", result.SuccessCount)
	}
	if result.ErrorCount != 1 {
		t.Errorf("expected ErrorCount=1, got %d", result.ErrorCount)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error entry, got %d", len(result.Errors))
	}
}

func TestMigrateAll_SkipExisting(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "existing-session",
		Name:          "existing",
		CreatedAt:     now,
		UpdatedAt:     now,
		Context: manifest.Context{
			Project: "/tmp/existing",
		},
		Tmux: manifest.Tmux{
			SessionName: "existing",
		},
	}

	// Insert session first
	if err := database.CreateSession(m); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Try to migrate again
	result, err := MigrateAll(database, []*manifest.Manifest{m}, false)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if result.SkippedCount != 1 {
		t.Errorf("expected SkippedCount=1, got %d", result.SkippedCount)
	}
	if result.SuccessCount != 0 {
		t.Errorf("expected SuccessCount=0, got %d", result.SuccessCount)
	}
}

func TestIntegration_YAMLToSQLite(t *testing.T) {
	// Create temporary directory with test YAML files
	tmpDir := t.TempDir()

	testFiles := []struct {
		filename string
		content  string
	}{
		{
			filename: "session-001.yaml",
			content: `schema_version: "2.0"
session_id: "integration-001"
name: "integration-test-1"
created_at: "2026-02-01T10:00:00Z"
updated_at: "2026-02-01T11:00:00Z"
lifecycle: ""
agent: "claude"
context:
  project: "/home/user/projects/test1"
  purpose: "Integration testing"
  tags: ["integration", "test", "sqlite"]
  notes: "Full integration test"
tmux:
  session_name: "integration-test-1"
claude:
  uuid: "claude-uuid-001"
`,
		},
		{
			filename: "session-002.yaml",
			content: `schema_version: "2.0"
session_id: "integration-002"
name: "integration-test-2"
created_at: "2026-02-02T10:00:00Z"
updated_at: "2026-02-02T11:00:00Z"
lifecycle: "archived"
agent: "gemini"
context:
  project: "/home/user/projects/test2"
  purpose: "Archived session test"
  tags: []
  notes: ""
tmux:
  session_name: "integration-test-2"
claude:
  uuid: ""
engram_metadata:
  enabled: true
  query: "integration test query"
  engram_ids: ["engram-001", "engram-002", "engram-003"]
  loaded_at: "2026-02-02T10:30:00Z"
  count: 3
`,
		},
		{
			filename: "session-003.yaml",
			content: `schema_version: "2.0"
session_id: "integration-003"
name: "integration-test-3"
created_at: "2026-02-03T10:00:00Z"
updated_at: "2026-02-03T11:00:00Z"
lifecycle: ""
agent: "gpt"
context:
  project: "/home/user/projects/test3"
  purpose: "Minimal test"
  tags: ["minimal"]
  notes: "Testing minimal configuration"
tmux:
  session_name: "integration-test-3"
claude:
  uuid: ""
`,
		},
	}

	// Write test files
	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.filename)
		if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Read YAML manifests
	manifests, err := ReadYAMLManifests(tmpDir)
	if err != nil {
		t.Fatalf("failed to read manifests: %v", err)
	}

	if len(manifests) != 3 {
		t.Fatalf("expected 3 manifests, got %d", len(manifests))
	}

	// Create in-memory SQLite database
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Migrate all manifests
	result, err := MigrateAll(database, manifests, false)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify migration results
	if result.TotalFiles != 3 {
		t.Errorf("expected TotalFiles=3, got %d", result.TotalFiles)
	}
	if result.SuccessCount != 3 {
		t.Errorf("expected SuccessCount=3, got %d", result.SuccessCount)
	}
	if result.ErrorCount != 0 {
		t.Errorf("expected ErrorCount=0, got %d", result.ErrorCount)
	}
	if result.SessionsMigrated != 3 {
		t.Errorf("expected SessionsMigrated=3, got %d", result.SessionsMigrated)
	}

	// Validate each migrated session
	for _, originalManifest := range manifests {
		retrieved, err := database.GetSession(originalManifest.SessionID)
		if err != nil {
			t.Fatalf("failed to retrieve session %s: %v", originalManifest.SessionID, err)
		}

		if err := ValidateMigration(originalManifest, retrieved); err != nil {
			t.Errorf("validation failed for %s: %v", originalManifest.SessionID, err)
		}
	}

	// Verify specific fields for session-002 (with engram metadata)
	session002, err := database.GetSession("integration-002")
	if err != nil {
		t.Fatalf("failed to retrieve session-002: %v", err)
	}

	if session002.EngramMetadata == nil {
		t.Fatal("expected engram_metadata to be non-nil for session-002")
	}
	if !session002.EngramMetadata.Enabled {
		t.Error("expected engram_metadata.enabled=true for session-002")
	}
	if session002.EngramMetadata.Count != 3 {
		t.Errorf("expected engram_metadata.count=3, got %d", session002.EngramMetadata.Count)
	}
	if len(session002.EngramMetadata.EngramIDs) != 3 {
		t.Errorf("expected 3 engram IDs, got %d", len(session002.EngramMetadata.EngramIDs))
	}
}
