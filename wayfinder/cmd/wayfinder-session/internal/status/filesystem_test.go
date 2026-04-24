package status

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanPhaseFiles(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create test phase files
	testFiles := []string{
		"W0.md",
		"D1.md",
		"D2.md",
		"S4.md",
		"README.md", // Should be ignored
		"test.txt",  // Should be ignored
		"D99.md",    // Valid pattern but not in AllPhases()
	}

	for _, fname := range testFiles {
		content := "---\nphase: test\n---\n\nTest content"
		if err := os.WriteFile(filepath.Join(tmpDir, fname), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Scan directory
	phaseFiles, err := ScanPhaseFiles(tmpDir)
	if err != nil {
		t.Fatalf("ScanPhaseFiles failed: %v", err)
	}

	// Verify results
	if len(phaseFiles) != 5 { // W0, D1, D2, S4, D99
		t.Errorf("expected 5 phase files, got %d", len(phaseFiles))
	}

	// Verify sorting (should be in phase order)
	expectedOrder := []string{"W0", "D1", "D2", "S4", "D99"}
	for i, expected := range expectedOrder {
		if i >= len(phaseFiles) {
			break
		}
		if phaseFiles[i].Name != expected {
			t.Errorf("phaseFiles[%d].Name = %q, want %q", i, phaseFiles[i].Name, expected)
		}
	}

	// Verify none are validated (no signatures)
	for _, pf := range phaseFiles {
		if pf.Validated {
			t.Errorf("phase %s should not be validated", pf.Name)
		}
	}
}

func TestScanPhaseFiles_WithSignatures(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file with validation signature
	content := `---
phase: D1
validated: true
validated_at: 2026-01-24T12:00:00Z
validator_version: 1.0.0
checksum: sha256-abc123
---

Test content`

	if err := os.WriteFile(filepath.Join(tmpDir, "D1.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Create file without signature
	content2 := `---
phase: D2
---

Test content`

	if err := os.WriteFile(filepath.Join(tmpDir, "D2.md"), []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	// Scan directory
	phaseFiles, err := ScanPhaseFiles(tmpDir)
	if err != nil {
		t.Fatalf("ScanPhaseFiles failed: %v", err)
	}

	if len(phaseFiles) != 2 {
		t.Fatalf("expected 2 phase files, got %d", len(phaseFiles))
	}

	// D1 should be validated
	if !phaseFiles[0].Validated {
		t.Error("D1 should be validated")
	}
	if phaseFiles[0].ValidatedAt == nil {
		t.Error("D1 should have ValidatedAt timestamp")
	}

	// D2 should not be validated
	if phaseFiles[1].Validated {
		t.Error("D2 should not be validated")
	}
}

func TestDetectFromFilesystem(t *testing.T) {
	tmpDir := t.TempDir()

	// Create phase files in various states
	files := map[string]string{
		"W0.md": `---
phase: W0
validated: true
validated_at: 2026-01-24T10:00:00Z
validator_version: 1.0.0
checksum: sha256-w0hash
---

W0 content`,
		"D1.md": `---
phase: D1
validated: true
validated_at: 2026-01-24T11:00:00Z
validator_version: 1.0.0
checksum: sha256-d1hash
---

D1 content`,
		"D2.md": `---
phase: D2
---

D2 in progress`,
	}

	for fname, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, fname), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Detect status from filesystem
	status, err := DetectFromFilesystem(tmpDir)
	if err != nil {
		t.Fatalf("DetectFromFilesystem failed: %v", err)
	}

	// Verify status
	if status.ProjectPath != tmpDir {
		t.Errorf("ProjectPath = %q, want %q", status.ProjectPath, tmpDir)
	}

	if status.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", status.Status, StatusInProgress)
	}

	// Verify phases
	if len(status.Phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(status.Phases))
	}

	// W0 should be completed
	if status.Phases[0].Name != "W0" {
		t.Errorf("Phases[0].Name = %q, want W0", status.Phases[0].Name)
	}
	if status.Phases[0].Status != PhaseStatusCompleted {
		t.Errorf("W0 status = %q, want %q", status.Phases[0].Status, PhaseStatusCompleted)
	}

	// D1 should be completed
	if status.Phases[1].Name != "D1" {
		t.Errorf("Phases[1].Name = %q, want D1", status.Phases[1].Name)
	}
	if status.Phases[1].Status != PhaseStatusCompleted {
		t.Errorf("D1 status = %q, want %q", status.Phases[1].Status, PhaseStatusCompleted)
	}

	// D2 should be in progress
	if status.Phases[2].Name != "D2" {
		t.Errorf("Phases[2].Name = %q, want D2", status.Phases[2].Name)
	}
	if status.Phases[2].Status != PhaseStatusInProgress {
		t.Errorf("D2 status = %q, want %q", status.Phases[2].Status, PhaseStatusInProgress)
	}

	// Current phase should be D2 (first incomplete)
	if status.CurrentPhase != "D2" {
		t.Errorf("CurrentPhase = %q, want D2", status.CurrentPhase)
	}
}

func TestDetectFromFilesystem_AllCompleted(t *testing.T) {
	tmpDir := t.TempDir()

	// Create all completed phases
	content := `---
phase: test
validated: true
validated_at: 2026-01-24T12:00:00Z
validator_version: 1.0.0
checksum: sha256-hash
---

Content`

	for _, phase := range []string{"W0", "D1", "D2"} {
		fname := phase + ".md"
		if err := os.WriteFile(filepath.Join(tmpDir, fname), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	status, err := DetectFromFilesystem(tmpDir)
	if err != nil {
		t.Fatalf("DetectFromFilesystem failed: %v", err)
	}

	// Current phase should be last phase when all completed
	if status.CurrentPhase != "D2" {
		t.Errorf("CurrentPhase = %q, want D2 (last completed)", status.CurrentPhase)
	}

	// All phases should be completed
	for _, phase := range status.Phases {
		if phase.Status != PhaseStatusCompleted {
			t.Errorf("phase %s status = %q, want %q", phase.Name, phase.Status, PhaseStatusCompleted)
		}
	}
}

func TestDetectFromFilesystem_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	status, err := DetectFromFilesystem(tmpDir)
	if err != nil {
		t.Fatalf("DetectFromFilesystem failed: %v", err)
	}

	if len(status.Phases) != 0 {
		t.Errorf("expected 0 phases, got %d", len(status.Phases))
	}

	if status.CurrentPhase != "" {
		t.Errorf("CurrentPhase = %q, want empty string", status.CurrentPhase)
	}
}

func TestDetermineCurrentPhase(t *testing.T) {
	tests := []struct {
		name     string
		files    []PhaseFile
		expected string
	}{
		{
			name:     "empty list",
			files:    []PhaseFile{},
			expected: "",
		},
		{
			name: "first phase not validated",
			files: []PhaseFile{
				{Name: "W0", Validated: false},
				{Name: "D1", Validated: false},
			},
			expected: "W0",
		},
		{
			name: "first phase validated, second not",
			files: []PhaseFile{
				{Name: "W0", Validated: true},
				{Name: "D1", Validated: false},
			},
			expected: "D1",
		},
		{
			name: "all validated",
			files: []PhaseFile{
				{Name: "W0", Validated: true},
				{Name: "D1", Validated: true},
				{Name: "D2", Validated: true},
			},
			expected: "D2",
		},
		{
			name: "middle phase not validated",
			files: []PhaseFile{
				{Name: "W0", Validated: true},
				{Name: "D1", Validated: false},
				{Name: "D2", Validated: false},
			},
			expected: "D1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineCurrentPhase(tt.files)
			if result != tt.expected {
				t.Errorf("determineCurrentPhase() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPhaseIndex(t *testing.T) {
	tests := []struct {
		phase    string
		expected int
	}{
		{"W0", 0},
		{"D1", 1},
		{"D2", 2},
		{"D3", 3},
		{"D4", 4},
		{"S4", 5},
		{"S5", 6},
		{"S11", 12},
		{"X99", 999}, // Unknown phase
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := phaseIndex(tt.phase, WayfinderV1)
			if result != tt.expected {
				t.Errorf("phaseIndex(%q) = %d, want %d", tt.phase, result, tt.expected)
			}
		})
	}
}

func TestCheckSignature(t *testing.T) {
	tmpDir := t.TempDir()

	// Test file with valid signature
	content1 := `---
phase: D1
validated: true
validated_at: "2026-01-24T12:00:00Z"
validator_version: "1.0.0"
checksum: sha256-abc123
---

Content`

	path1 := filepath.Join(tmpDir, "test1.md")
	if err := os.WriteFile(path1, []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}

	validated, validatedAt, checksum, version := checkSignature(path1)
	if !validated {
		t.Error("expected validated = true")
	}
	if validatedAt == nil {
		t.Error("expected validatedAt to be set")
	} else {
		expected := time.Date(2026, 1, 24, 12, 0, 0, 0, time.UTC)
		if !validatedAt.Equal(expected) {
			t.Errorf("validatedAt = %v, want %v", validatedAt, expected)
		}
	}
	if checksum != "sha256-abc123" {
		t.Errorf("checksum = %q, want sha256-abc123", checksum)
	}
	if version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", version)
	}

	// Test file without signature
	content2 := `---
phase: D2
---

Content`

	path2 := filepath.Join(tmpDir, "test2.md")
	if err := os.WriteFile(path2, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	validated2, validatedAt2, _, _ := checkSignature(path2)
	if validated2 {
		t.Error("expected validated = false")
	}
	if validatedAt2 != nil {
		t.Error("expected validatedAt = nil")
	}
}
