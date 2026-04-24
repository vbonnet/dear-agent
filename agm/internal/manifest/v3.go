// Package manifest provides manifest functionality.
package manifest

import (
	"fmt"
	"time"
)

// ManifestV3 represents a Claude session manifest (v3 schema)
// Extends v2 with harness tracking for multi-harness support (AGM evolution)
type ManifestV3 struct {
	// v2 fields (unchanged for backward compatibility)
	SchemaVersion string    `yaml:"schema_version"` // "3.0"
	SessionID     string    `yaml:"session_id"`
	Name          string    `yaml:"name"`
	CreatedAt     time.Time `yaml:"created_at"`
	UpdatedAt     time.Time `yaml:"updated_at"`
	Lifecycle     string    `yaml:"lifecycle"` // "" (active/stopped) or "archived"
	Context       Context   `yaml:"context"`
	Claude        Claude    `yaml:"claude"`
	Tmux          Tmux      `yaml:"tmux"`

	// v3 additions for multi-harness tracking
	Harness        string          `yaml:"harness"`         // Current active harness (claude-code, gemini-cli, codex-cli, etc.)
	Model          string          `yaml:"model,omitempty"` // Model within the harness
	HarnessHistory []HarnessSwitch `yaml:"harness_history"` // Historical harness switches
}

// HarnessSwitch records a harness switch event in the session history
type HarnessSwitch struct {
	Timestamp   time.Time `yaml:"timestamp"`    // When the switch occurred
	FromHarness string    `yaml:"from_harness"` // Harness before the switch
	ToHarness   string    `yaml:"to_harness"`   // Harness after the switch
}

// Validate validates the v3 manifest fields
// Performs v2-compatible validation plus v3-specific checks
func (m *ManifestV3) Validate() error {
	// Reuse v2 validation logic for common fields
	// Create temporary v2 manifest for validation
	v2 := Manifest{
		SchemaVersion: m.SchemaVersion,
		SessionID:     m.SessionID,
		Name:          m.Name,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		Lifecycle:     m.Lifecycle,
		Context:       m.Context,
		Claude:        m.Claude,
		Tmux:          m.Tmux,
	}

	// Run v2 validation (validates session_id, name, timestamps, etc.)
	if err := v2.Validate(); err != nil {
		return err
	}

	// v3-specific validation
	if m.Harness == "" {
		return fmt.Errorf("harness field cannot be empty")
	}

	// Warn if harness is not in known set (permissive validation)
	knownHarnesses := map[string]bool{
		"claude-code":  true,
		"gemini-cli":   true,
		"codex-cli":    true,
		"opencode-cli": true,
	}
	if !knownHarnesses[m.Harness] {
		// Note: This is a warning, not an error (forward compatible)
		// In production, this would log to stderr via logMigration or similar
		// For now, we allow unknown harnesses
	}

	// Verify harness_history is not nil (can be empty array)
	if m.HarnessHistory == nil {
		return fmt.Errorf("harness_history cannot be nil (use empty array [])")
	}

	// Validate each harness switch entry
	for i, sw := range m.HarnessHistory {
		if sw.Timestamp.IsZero() {
			return fmt.Errorf("harness_history[%d]: timestamp cannot be zero", i)
		}
		if sw.FromHarness == "" {
			return fmt.Errorf("harness_history[%d]: from_harness cannot be empty", i)
		}
		if sw.ToHarness == "" {
			return fmt.Errorf("harness_history[%d]: to_harness cannot be empty", i)
		}
	}

	return nil
}
