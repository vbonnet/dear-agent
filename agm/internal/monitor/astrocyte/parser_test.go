package astrocyte

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDiagnosisFile(t *testing.T) {
	// Create temporary test files
	tempDir := t.TempDir()

	// Test case 1: Well-formed diagnosis file
	validDiagnosis := `## Incident Diagnosis: sessions-stuck - 2026-02-02T21-38-23

### Symptom
Permission prompt detected and auto-rejected after 0 minutes (immediate detection)

### Context
Testing the diagnosis parser with a sample file.

### Root Cause (Hypothesis)
**MEDIUM CONFIDENCE**: Test diagnosis file.

**Incident Type**: AUTO-RECOVERY SUCCESS (no user intervention needed)
**Recovery Time**: 3.0 seconds (excellent)
**Recovery Method**: ESC (Astrocyte daemon)
**Confidence**: **MEDIUM (70%)**
`

	validFile := filepath.Join(tempDir, "sessions-stuck-2026-02-02T21-38-23.md")
	if err := os.WriteFile(validFile, []byte(validDiagnosis), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	diag, err := ParseDiagnosisFile(validFile)
	if err != nil {
		t.Fatalf("ParseDiagnosisFile() failed: %v", err)
	}

	// Verify parsed fields
	if diag.SessionID != "sessions-stuck" {
		t.Errorf("SessionID = %q, want %q", diag.SessionID, "sessions-stuck")
	}

	expectedTime := time.Date(2026, 2, 2, 21, 38, 23, 0, time.UTC)
	if !diag.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", diag.Timestamp, expectedTime)
	}

	if diag.Type != "AUTO-RECOVERY SUCCESS (no user intervention needed)" {
		t.Errorf("Type = %q, want %q", diag.Type, "AUTO-RECOVERY SUCCESS (no user intervention needed)")
	}

	if !diag.RecoverySuccess {
		t.Error("RecoverySuccess = false, want true")
	}

	if diag.RecoveryTime != "3.0 seconds (excellent)" {
		t.Errorf("RecoveryTime = %q, want %q", diag.RecoveryTime, "3.0 seconds (excellent)")
	}

	if diag.RecoveryMethod != "ESC (Astrocyte daemon)" {
		t.Errorf("RecoveryMethod = %q, want %q", diag.RecoveryMethod, "ESC (Astrocyte daemon)")
	}

	if diag.Confidence != "MEDIUM" {
		t.Errorf("Confidence = %q, want %q", diag.Confidence, "MEDIUM")
	}
}

func TestParseDiagnosisFile_MinimalFile(t *testing.T) {
	tempDir := t.TempDir()

	// Minimal diagnosis file (just title)
	minimalDiagnosis := `## Incident Diagnosis: tool-usage-violations - 2026-02-02T21-38-30

### Symptom
Permission prompt detected.
`

	minimalFile := filepath.Join(tempDir, "minimal.md")
	if err := os.WriteFile(minimalFile, []byte(minimalDiagnosis), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	diag, err := ParseDiagnosisFile(minimalFile)
	if err != nil {
		t.Fatalf("ParseDiagnosisFile() failed: %v", err)
	}

	if diag.SessionID != "tool-usage-violations" {
		t.Errorf("SessionID = %q, want %q", diag.SessionID, "tool-usage-violations")
	}

	expectedTime := time.Date(2026, 2, 2, 21, 38, 30, 0, time.UTC)
	if !diag.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", diag.Timestamp, expectedTime)
	}

	if diag.Symptom == "" {
		t.Error("Symptom should not be empty")
	}
}

func TestParseDiagnosisFile_MalformedFile(t *testing.T) {
	tempDir := t.TempDir()

	// Malformed file (no title)
	malformedDiagnosis := `This is not a valid diagnosis file.

No title or structure.
`

	malformedFile := filepath.Join(tempDir, "malformed.md")
	if err := os.WriteFile(malformedFile, []byte(malformedDiagnosis), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	diag, err := ParseDiagnosisFile(malformedFile)
	if err != nil {
		t.Fatalf("ParseDiagnosisFile() should not fail on malformed file: %v", err)
	}

	// Should gracefully handle malformed file
	// Session ID should be extracted from filename
	if diag.SessionID != "malformed" {
		t.Errorf("SessionID = %q, want %q (from filename)", diag.SessionID, "malformed")
	}
}

func TestParseDiagnosisFile_NonExistentFile(t *testing.T) {
	_, err := ParseDiagnosisFile("/nonexistent/file.md")
	if err == nil {
		t.Error("ParseDiagnosisFile() should fail for non-existent file")
	}
}

func TestParseDiagnosisDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple diagnosis files
	files := map[string]string{
		"session1-2026-01-01T10-00-00.md": `## Incident Diagnosis: session1 - 2026-01-01T10:00:00

### Symptom
Galloping detected.

**Incident Type**: Zero-Token Galloping
`,
		"session2-2026-01-02T15-30-00.md": `## Incident Diagnosis: session2 - 2026-01-02T15:30:00

### Symptom
Permission prompt stuck.

**Recovery Time**: 5.0 seconds
`,
		"invalid.txt": "This should be ignored (not .md)",
	}

	for filename, content := range files {
		path := filepath.Join(tempDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	diagnoses, err := ParseDiagnosisDirectory(tempDir)
	if err != nil {
		// Partial errors are ok if we got some diagnoses
		t.Logf("ParseDiagnosisDirectory() returned error: %v", err)
	}

	// Should have parsed 2 .md files (ignore .txt file)
	if len(diagnoses) != 2 {
		t.Errorf("ParseDiagnosisDirectory() parsed %d files, want 2", len(diagnoses))
	}

	// Verify session IDs
	sessionIDs := make(map[string]bool)
	for _, diag := range diagnoses {
		sessionIDs[diag.SessionID] = true
	}

	if !sessionIDs["session1"] || !sessionIDs["session2"] {
		t.Errorf("ParseDiagnosisDirectory() missing expected sessions: %v", sessionIDs)
	}
}

func TestInferHangType(t *testing.T) {
	tests := []struct {
		symptom  string
		existing string
		want     string
	}{
		{
			symptom:  "Permission prompt detected and auto-rejected",
			existing: "",
			want:     "Permission Prompt",
		},
		{
			symptom:  "Stuck in 'Galloping...' state with 0 tokens downloaded",
			existing: "",
			want:     "Zero-Token Galloping",
		},
		{
			symptom:  "Session stuck on prompt",
			existing: "",
			want:     "Stuck Prompt",
		},
		{
			symptom:  "API timeout after 30 seconds",
			existing: "",
			want:     "API Stall",
		},
		{
			symptom:  "System deadlock detected",
			existing: "",
			want:     "Deadlock",
		},
		{
			symptom:  "Unknown issue",
			existing: "",
			want:     "Unknown",
		},
		{
			symptom:  "Some symptom",
			existing: "Predefined Type",
			want:     "Predefined Type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.symptom, func(t *testing.T) {
			got := inferHangType(tt.symptom, tt.existing)
			if got != tt.want {
				t.Errorf("inferHangType(%q, %q) = %q, want %q",
					tt.symptom, tt.existing, got, tt.want)
			}
		})
	}
}

func TestExtractSessionFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{
			filename: "/path/to/sessions-stuck-2026-02-02T21-38-23.md",
			want:     "sessions-stuck",
		},
		{
			filename: "tool-usage-violations-2026-02-03T15-30-00.md",
			want:     "tool-usage-violations",
		},
		{
			filename: "/tmp/autonomous-swarm-coordinator-2026-01-30T10-00.md",
			want:     "autonomous-swarm-coordinator",
		},
		{
			filename: "simple-name.md",
			want:     "simple-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractSessionFromFilename(tt.filename)
			if got != tt.want {
				t.Errorf("extractSessionFromFilename(%q) = %q, want %q",
					tt.filename, got, tt.want)
			}
		})
	}
}

func TestFilterBySession(t *testing.T) {
	diagnoses := []*Diagnosis{
		{SessionID: "session1"},
		{SessionID: "session2"},
		{SessionID: "session1"},
		{SessionID: "session3"},
	}

	filtered := FilterBySession(diagnoses, "session1")

	if len(filtered) != 2 {
		t.Errorf("FilterBySession() returned %d diagnoses, want 2", len(filtered))
	}

	for _, diag := range filtered {
		if diag.SessionID != "session1" {
			t.Errorf("FilterBySession() returned wrong session: %q", diag.SessionID)
		}
	}
}

func TestFilterByTimeRange(t *testing.T) {
	diagnoses := []*Diagnosis{
		{Timestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2026, 1, 4, 10, 0, 0, 0, time.UTC)},
	}

	start := time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 3, 23, 0, 0, 0, time.UTC)

	filtered := FilterByTimeRange(diagnoses, start, end)

	// Should include only Jan 2 and Jan 3 (between start and end)
	if len(filtered) != 2 {
		t.Errorf("FilterByTimeRange() returned %d diagnoses, want 2", len(filtered))
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input string
		want  time.Time
	}{
		{
			input: "2026-02-02T21:38:23",
			want:  time.Date(2026, 2, 2, 21, 38, 23, 0, time.UTC),
		},
		{
			input: "2026-02-02T21:38",
			want:  time.Date(2026, 2, 2, 21, 38, 0, 0, time.UTC),
		},
		{
			input: "2026-02-02T21-38-23",
			want:  time.Date(2026, 2, 2, 21, 38, 23, 0, time.UTC),
		},
		{
			input: "2026-02-02",
			want:  time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			input: "invalid-timestamp",
			want:  time.Time{}, // Zero time
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTimestamp(tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("parseTimestamp(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Integration test with real diagnosis files
func TestParseDiagnosisFile_RealFiles(t *testing.T) {
	diagnosisDir := "~/.agm/astrocyte/diagnoses"

	// Check if directory exists (skip test if not)
	if _, err := os.Stat(diagnosisDir); os.IsNotExist(err) {
		t.Skip("Astrocyte diagnoses directory not found, skipping integration test")
	}

	// Try to parse all real diagnosis files
	diagnoses, err := ParseDiagnosisDirectory(diagnosisDir)
	if err != nil {
		t.Logf("ParseDiagnosisDirectory() returned error: %v", err)
	}

	if len(diagnoses) == 0 {
		t.Skip("No diagnosis files found in directory")
	}

	t.Logf("Successfully parsed %d diagnosis files", len(diagnoses))

	// Verify at least some diagnoses have required fields
	validCount := 0
	for _, diag := range diagnoses {
		if diag.SessionID != "" && !diag.Timestamp.IsZero() {
			validCount++
		}
	}

	if validCount == 0 {
		t.Error("No valid diagnoses found (missing SessionID or Timestamp)")
	}

	t.Logf("Found %d valid diagnoses with SessionID and Timestamp", validCount)
}
