package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Test 1: Empty directory returns empty list
func TestScanDirectory_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	manifests, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil", err)
	}

	if len(manifests) != 0 {
		t.Errorf("scanDirectory() returned %d manifests, want 0", len(manifests))
	}
}

// Test 2: Non-existent directory returns empty list
func TestScanDirectory_NonExistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does-not-exist")

	manifests, err := scanDirectory(nonExistent)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil for non-existent dir", err)
	}

	if len(manifests) != 0 {
		t.Errorf("scanDirectory() returned %d manifests, want 0 for non-existent dir", len(manifests))
	}
}

// Test 3: Directory with valid manifests
func TestScanDirectory_ValidManifests(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test manifests
	sessions := []string{"session-1", "session-2", "session-3"}
	for _, sessionName := range sessions {
		sessionDir := filepath.Join(tmpDir, sessionName)
		if err := os.MkdirAll(sessionDir, 0755); err != nil {
			t.Fatal(err)
		}

		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifestContent := `---
schema_version: "2.0"
session_id: "test-uuid-` + sessionName + `"
name: "` + sessionName + `"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test-project"
  purpose: "test-purpose"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid-` + sessionName + `"
tmux:
  session_name: "` + sessionName + `"
`
		if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	manifests, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil", err)
	}

	if len(manifests) != 3 {
		t.Errorf("scanDirectory() returned %d manifests, want 3", len(manifests))
	}
}

// Test 4: Skips invalid manifests
func TestScanDirectory_InvalidManifests(t *testing.T) {
	tmpDir := t.TempDir()

	// Create one valid and one invalid manifest
	// Valid manifest
	validDir := filepath.Join(tmpDir, "valid-session")
	os.MkdirAll(validDir, 0755)
	validManifest := filepath.Join(validDir, "manifest.yaml")
	os.WriteFile(validManifest, []byte(`---
schema_version: "2.0"
session_id: "valid-uuid"
name: "valid-session"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "valid"
`), 0644)

	// Invalid manifest (bad YAML)
	invalidDir := filepath.Join(tmpDir, "invalid-session")
	os.MkdirAll(invalidDir, 0755)
	invalidManifest := filepath.Join(invalidDir, "manifest.yaml")
	os.WriteFile(invalidManifest, []byte("invalid: yaml: content: [[["), 0644)

	manifests, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil (should skip invalid)", err)
	}

	// Should only return the valid manifest
	if len(manifests) != 1 {
		t.Errorf("scanDirectory() returned %d manifests, want 1 (skipping invalid)", len(manifests))
	}

	if len(manifests) > 0 && manifests[0].SessionID != "valid-uuid" {
		t.Errorf("scanDirectory() returned wrong manifest, got SessionID=%s, want valid-uuid", manifests[0].SessionID)
	}
}

// Test 5: Nested subdirectories (should only find immediate children)
func TestScanDirectory_NestedSubdirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create immediate child with manifest
	immediateChild := filepath.Join(tmpDir, "immediate")
	os.MkdirAll(immediateChild, 0755)
	immediateManifest := filepath.Join(immediateChild, "manifest.yaml")
	os.WriteFile(immediateManifest, []byte(`---
schema_version: "2.0"
session_id: "immediate-uuid"
name: "immediate"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "immediate"
`), 0644)

	// Create nested child with manifest (should NOT be found)
	nestedChild := filepath.Join(tmpDir, "parent", "nested")
	os.MkdirAll(nestedChild, 0755)
	nestedManifest := filepath.Join(nestedChild, "manifest.yaml")
	os.WriteFile(nestedManifest, []byte(`---
schema_version: "2.0"
session_id: "nested-uuid"
name: "nested"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "nested"
`), 0644)

	manifests, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil", err)
	}

	// Should only find immediate child, not nested
	if len(manifests) != 1 {
		t.Errorf("scanDirectory() returned %d manifests, want 1 (only immediate children)", len(manifests))
	}

	if len(manifests) > 0 && manifests[0].SessionID != "immediate-uuid" {
		t.Errorf("scanDirectory() found wrong manifest, got SessionID=%s, want immediate-uuid", manifests[0].SessionID)
	}
}

// Test 6: Files in root directory are ignored
func TestScanDirectory_IgnoresRootFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest.yaml in root (should be ignored)
	rootManifest := filepath.Join(tmpDir, "manifest.yaml")
	os.WriteFile(rootManifest, []byte("some content"), 0644)

	// Create proper manifest in subdirectory
	sessionDir := filepath.Join(tmpDir, "session")
	os.MkdirAll(sessionDir, 0755)
	sessionManifest := filepath.Join(sessionDir, "manifest.yaml")
	os.WriteFile(sessionManifest, []byte(`---
schema_version: "2.0"
session_id: "session-uuid"
name: "session"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "session"
`), 0644)

	manifests, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil", err)
	}

	// Should only find subdirectory manifest, not root
	if len(manifests) != 1 {
		t.Errorf("scanDirectory() returned %d manifests, want 1 (ignoring root files)", len(manifests))
	}
}

// Test 7: List() integration test
func TestList_IntegrationWithGlob(t *testing.T) {
	tmpDir := t.TempDir()

	// Create main directory manifests
	for i := 1; i <= 2; i++ {
		sessionDir := filepath.Join(tmpDir, "main-"+string(rune('0'+i)))
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifestContent := `---
schema_version: "2.0"
session_id: "main-uuid-` + string(rune('0'+i)) + `"
name: "main-` + string(rune('0'+i)) + `"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "main"
`
		os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	}

	// Create archive directory manifest
	archiveDir := filepath.Join(tmpDir, ".archive-old-format")
	os.MkdirAll(archiveDir, 0755)
	archivedSessionDir := filepath.Join(archiveDir, "archived")
	os.MkdirAll(archivedSessionDir, 0755)
	archivedManifest := filepath.Join(archivedSessionDir, "manifest.yaml")
	os.WriteFile(archivedManifest, []byte(`---
schema_version: "2.0"
session_id: "archived-uuid"
name: "archived"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "archived"
`), 0644)

	manifests, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	// Should find 2 main + 1 archived = 3 total
	if len(manifests) != 3 {
		t.Errorf("List() returned %d manifests, want 3 (2 main + 1 archived)", len(manifests))
	}
}

// Test 8: Stress test with many manifests
func TestScanDirectory_ManyManifests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 100 manifests
	for i := 0; i < 100; i++ {
		sessionName := fmt.Sprintf("session-%03d", i)
		sessionDir := filepath.Join(tmpDir, sessionName)
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		sessionID := fmt.Sprintf("uuid-%03d", i)
		manifestContent := `---
schema_version: "2.0"
session_id: "` + sessionID + `"
name: "` + sessionName + `"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "session"
`
		os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	}

	manifests, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v, want nil", err)
	}

	if len(manifests) != 100 {
		t.Errorf("scanDirectory() returned %d manifests, want 100", len(manifests))
	}
}

// Test 9: Benchmark with 100 manifests
func BenchmarkScanDirectory_100(b *testing.B) {
	tmpDir := b.TempDir()

	// Setup: Create 100 test manifests
	for i := 0; i < 100; i++ {
		sessionDir := filepath.Join(tmpDir, "bench-"+string(rune('0'+i%10))+string(rune('0'+i/10)))
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifestContent := `---
schema_version: "2.0"
session_id: "bench-uuid"
name: "bench-session"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "bench"
`
		os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = scanDirectory(tmpDir)
	}
}

// Test 10: Benchmark with 1000 manifests
func BenchmarkScanDirectory_1000(b *testing.B) {
	tmpDir := b.TempDir()

	// Setup: Create 1000 test manifests
	for i := 0; i < 1000; i++ {
		sessionDir := filepath.Join(tmpDir, "stress-"+string(rune('0'+i%10))+string(rune('0'+(i/10)%10))+string(rune('0'+i/100)))
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifestContent := `---
schema_version: "2.0"
session_id: "stress-uuid"
name: "stress-session"
created_at: 2026-01-28T00:00:00Z
updated_at: 2026-01-28T00:00:00Z
lifecycle: ""
context:
  project: "test"
  purpose: "test"
  tags: []
  notes: ""
claude:
  uuid: "claude-uuid"
tmux:
  session_name: "stress"
`
		os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = scanDirectory(tmpDir)
	}
}
