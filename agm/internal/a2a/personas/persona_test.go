package personas

import (
	"os"
	"path/filepath"
	"testing"
)

const validAIMD = `---
name: test-persona
displayName: Test Persona
description: A test persona
expertise:
  - testing
  - go
---

# Test Persona Content
This is the markdown content.
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------------------------------------------------------------------------
// LoadFromFile
// ---------------------------------------------------------------------------

func TestLoadFromFile_Valid(t *testing.T) {
	path := writeFile(t, t.TempDir(), "test-persona.ai.md", validAIMD)

	p, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile returned error: %v", err)
	}

	if p.Name != "test-persona" {
		t.Errorf("Name = %q, want %q", p.Name, "test-persona")
	}
	if p.DisplayName != "Test Persona" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Test Persona")
	}
	if p.Description != "A test persona" {
		t.Errorf("Description = %q, want %q", p.Description, "A test persona")
	}
	if len(p.Expertise) != 2 || p.Expertise[0] != "testing" || p.Expertise[1] != "go" {
		t.Errorf("Expertise = %v, want [testing go]", p.Expertise)
	}
	if p.Content == "" {
		t.Error("Content should not be empty")
	}
	if p.SourcePath != path {
		t.Errorf("SourcePath = %q, want %q", p.SourcePath, path)
	}
}

func TestLoadFromFile_Defaults(t *testing.T) {
	path := writeFile(t, t.TempDir(), "minimal.ai.md", validAIMD)

	p, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile returned error: %v", err)
	}

	if p.Version != "1.0.0" {
		t.Errorf("default Version = %q, want %q", p.Version, "1.0.0")
	}
	if p.Tier != "tier2" {
		t.Errorf("default Tier = %q, want %q", p.Tier, "tier2")
	}
	if p.Maturity != "stable" {
		t.Errorf("default Maturity = %q, want %q", p.Maturity, "stable")
	}
}

func TestLoadFromFile_MissingFrontmatter(t *testing.T) {
	content := "# No frontmatter here\nJust markdown.\n"
	path := writeFile(t, t.TempDir(), "bad.ai.md", content)

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for missing frontmatter, got nil")
	}
}

// ---------------------------------------------------------------------------
// IsExperimental / IsStable
// ---------------------------------------------------------------------------

func TestIsExperimental(t *testing.T) {
	p := &Persona{Maturity: "experimental"}
	if !p.IsExperimental() {
		t.Error("IsExperimental() = false, want true")
	}
	if p.IsStable() {
		t.Error("IsStable() = true, want false for experimental persona")
	}
}

func TestIsStable(t *testing.T) {
	p := &Persona{Maturity: "stable"}
	if !p.IsStable() {
		t.Error("IsStable() = false, want true")
	}
	if p.IsExperimental() {
		t.Error("IsExperimental() = true, want false for stable persona")
	}
}

// ---------------------------------------------------------------------------
// ToMap
// ---------------------------------------------------------------------------

func TestToMap(t *testing.T) {
	p := &Persona{
		Name:             "test",
		DisplayName:      "Test",
		Version:          "2.0.0",
		Description:      "desc",
		Expertise:        []string{"go"},
		SeverityLevels:   []string{"high"},
		FocusAreas:       []string{"security"},
		GitHistoryAccess: true,
		Tier:             "tier1",
		Maturity:         "stable",
	}

	m := p.ToMap()

	expectedKeys := []string{
		"name", "displayName", "version", "maturity", "tier",
		"description", "expertise", "severityLevels", "focusAreas",
		"gitHistoryAccess",
	}
	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("ToMap missing key %q", k)
		}
	}

	if m["name"] != "test" {
		t.Errorf("ToMap[name] = %v, want %q", m["name"], "test")
	}
	if m["displayName"] != "Test" {
		t.Errorf("ToMap[displayName] = %v, want %q", m["displayName"], "Test")
	}
	if m["version"] != "2.0.0" {
		t.Errorf("ToMap[version] = %v, want %q", m["version"], "2.0.0")
	}
	if m["gitHistoryAccess"] != true {
		t.Errorf("ToMap[gitHistoryAccess] = %v, want true", m["gitHistoryAccess"])
	}
}

// ---------------------------------------------------------------------------
// NewLoader
// ---------------------------------------------------------------------------

func TestNewLoader_EmptyPath(t *testing.T) {
	_, err := NewLoader("")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestNewLoader_ValidPath(t *testing.T) {
	loader, err := NewLoader("/some/path")
	if err != nil {
		t.Fatalf("NewLoader returned error: %v", err)
	}
	if loader == nil {
		t.Fatal("NewLoader returned nil loader")
	}
}

// ---------------------------------------------------------------------------
// Loader.ListPersonas
// ---------------------------------------------------------------------------

func TestListPersonas(t *testing.T) {
	dir := t.TempDir()

	persona1 := `---
name: alpha
displayName: Alpha
description: First persona
---

# Alpha
`
	persona2 := `---
name: beta
displayName: Beta
description: Second persona
maturity: experimental
---

# Beta
`
	writeFile(t, dir, "alpha.ai.md", persona1)
	writeFile(t, dir, "beta.ai.md", persona2)

	loader, err := NewLoader(dir)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}

	personas, err := loader.ListPersonas()
	if err != nil {
		t.Fatalf("ListPersonas: %v", err)
	}

	if len(personas) != 2 {
		t.Fatalf("ListPersonas returned %d personas, want 2", len(personas))
	}

	names := map[string]bool{}
	for _, p := range personas {
		names[p.Name] = true
	}
	if !names["alpha"] {
		t.Error("ListPersonas missing persona 'alpha'")
	}
	if !names["beta"] {
		t.Error("ListPersonas missing persona 'beta'")
	}
}

// ---------------------------------------------------------------------------
// NewTracker
// ---------------------------------------------------------------------------

func TestNewTracker(t *testing.T) {
	tracker := NewTracker("sess-123", "review")

	if tracker.sessionID != "sess-123" {
		t.Errorf("sessionID = %q, want %q", tracker.sessionID, "sess-123")
	}
	if tracker.phase != "review" {
		t.Errorf("phase = %q, want %q", tracker.phase, "review")
	}
	if tracker.startTime.IsZero() {
		t.Error("startTime should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Tracker.RecordTokens
// ---------------------------------------------------------------------------

func TestRecordTokens(t *testing.T) {
	tracker := NewTracker("sess-1", "phase-1")

	tracker.RecordTokens(100, 50)
	if tracker.inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", tracker.inputTokens)
	}
	if tracker.outputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", tracker.outputTokens)
	}

	// Accumulates
	tracker.RecordTokens(200, 75)
	if tracker.inputTokens != 300 {
		t.Errorf("inputTokens after second call = %d, want 300", tracker.inputTokens)
	}
	if tracker.outputTokens != 125 {
		t.Errorf("outputTokens after second call = %d, want 125", tracker.outputTokens)
	}
}

// ---------------------------------------------------------------------------
// Tracker.OnReviewComplete (telemetry disabled)
// ---------------------------------------------------------------------------

func TestOnReviewComplete_TelemetryDisabled(t *testing.T) {
	tracker := NewTracker("sess-1", "review")
	persona := &Persona{Name: "test", Version: "1.0.0", Maturity: "stable", Tier: "tier1"}
	issues := []Issue{
		{Severity: "high", Message: "something"},
		{Severity: "low", Message: "minor"},
	}

	err := tracker.OnReviewComplete(persona, issues, false)
	if err != nil {
		t.Fatalf("OnReviewComplete with telemetryEnabled=false should return nil, got: %v", err)
	}
}
