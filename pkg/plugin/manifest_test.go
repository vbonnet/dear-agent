package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validManifest returns a Manifest that passes Validate, used as the
// starting point for negative tests. Each negative test mutates one
// field, so a regression that adds a new required field will surface
// in the positive test rather than silently passing the negatives.
func validManifest() Manifest {
	return Manifest{
		APIVersion:   APIVersionV1,
		Kind:         KindPlugin,
		Name:         "dear-agent.test.example",
		Version:      "0.1.0",
		Description:  "test plugin",
		Author:       "test",
		Capabilities: []Capability{CapabilityHooks},
	}
}

func TestManifest_Validate_OK(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate on valid manifest: %v", err)
	}
}

func TestManifest_Validate_RejectsBadAPIVersion(t *testing.T) {
	m := validManifest()
	m.APIVersion = "dear-agent.io/v0"
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "APIVersion") {
		t.Fatalf("expected APIVersion error, got %v", err)
	}
}

func TestManifest_Validate_RejectsBadKind(t *testing.T) {
	m := validManifest()
	m.Kind = "Bundle"
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "Kind") {
		t.Fatalf("expected Kind error, got %v", err)
	}
}

func TestManifest_Validate_RejectsEmptyName(t *testing.T) {
	m := validManifest()
	m.Name = ""
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "Name") {
		t.Fatalf("expected Name error, got %v", err)
	}
}

func TestManifest_Validate_RejectsControlCharInName(t *testing.T) {
	m := validManifest()
	m.Name = "bad\x01name"
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("expected control-char error, got %v", err)
	}
}

func TestManifest_Validate_RejectsWhitespaceName(t *testing.T) {
	m := validManifest()
	m.Name = "  trimmed  "
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("expected whitespace error, got %v", err)
	}
}

func TestManifest_Validate_RejectsEmptyVersion(t *testing.T) {
	m := validManifest()
	m.Version = ""
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "Version") {
		t.Fatalf("expected Version error, got %v", err)
	}
}

func TestManifest_Validate_RejectsUnknownCapability(t *testing.T) {
	m := validManifest()
	m.Capabilities = []Capability{"telepathy"}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "telepathy") {
		t.Fatalf("expected unknown-capability error, got %v", err)
	}
}

func TestManifest_Validate_RejectsAbsoluteFSReadPath(t *testing.T) {
	m := validManifest()
	m.Permissions.FSRead = []string{"/etc/secrets"}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path error, got %v", err)
	}
}

func TestManifest_Validate_RejectsAbsoluteFSWritePath(t *testing.T) {
	m := validManifest()
	m.Permissions.FSWrite = []string{"/var/run"}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path error, got %v", err)
	}
}

func TestManifest_HasCapability(t *testing.T) {
	m := validManifest()
	m.Capabilities = []Capability{CapabilityHooks, CapabilityChecks}
	if !m.HasCapability(CapabilityHooks) {
		t.Errorf("expected HasCapability(hooks) = true")
	}
	if !m.HasCapability(CapabilityChecks) {
		t.Errorf("expected HasCapability(checks) = true")
	}
	if m.HasCapability("not-declared") {
		t.Errorf("expected HasCapability(not-declared) = false")
	}
}

func TestCapability_IsValid(t *testing.T) {
	cases := map[Capability]bool{
		CapabilityHooks:  true,
		CapabilityChecks: true,
		// Reserved-but-not-implemented names: must validate as false in
		// Phase 1 so manifests declaring them fail Validate.
		"events":     false,
		"node_kinds": false,
		"sources":    false,
		"":           false,
	}
	for c, want := range cases {
		if got := c.IsValid(); got != want {
			t.Errorf("Capability(%q).IsValid() = %v, want %v", c, got, want)
		}
	}
}

func TestLoadManifest_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yaml")
	yaml := `api_version: dear-agent.io/v1
kind: Plugin
name: dear-agent.test.example
version: 0.1.0
description: test
capabilities:
  - hooks
permissions:
  fs_read: ["docs/**"]
  network: false
config:
  threshold: 5
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Name != "dear-agent.test.example" {
		t.Errorf("Name = %q", m.Name)
	}
	if got, ok := m.Config["threshold"]; !ok {
		t.Errorf("Config.threshold missing")
	} else if got != 5 {
		// gopkg.in/yaml.v3 decodes YAML integers into Go int.
		t.Errorf("Config.threshold = %v (%T), want 5", got, got)
	}
}

func TestLoadManifest_MissingFile(t *testing.T) {
	_, err := LoadManifest(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadManifest_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte("not: valid: yaml: ::"), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLoadManifest_FailsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yaml")
	yaml := `api_version: dear-agent.io/v1
kind: Plugin
name: ""
version: 0.1.0
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "validate") {
		t.Fatalf("expected validate error, got %v", err)
	}
}
