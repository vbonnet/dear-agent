package presets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCorePresets(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	presetDir := filepath.Join(homeDir, ".wayfinder", "presets")
	if _, err := os.Stat(presetDir); os.IsNotExist(err) {
		t.Skip("~/.wayfinder/presets not installed; skipping core preset tests")
	}
	loader := NewLoaderWithDir(presetDir)

	corePresets := []string{"high-quality", "fast-iteration", "research-heavy"}

	for _, name := range corePresets {
		t.Run(name, func(t *testing.T) {
			preset, err := loader.Load(name)
			if err != nil {
				t.Fatalf("failed to load %s preset: %v", name, err)
			}

			// Verify basic fields
			if preset.Name != name {
				t.Errorf("preset name mismatch: got %s, want %s", preset.Name, name)
			}

			if preset.Version != "1.0" {
				t.Errorf("preset version mismatch: got %s, want 1.0", preset.Version)
			}

			// Verify test coverage is within valid range
			if preset.TestCoverage.MinimumPercentage < 0 || preset.TestCoverage.MinimumPercentage > 100 {
				t.Errorf("invalid test coverage percentage: %d", preset.TestCoverage.MinimumPercentage)
			}

			// Verify economic tuning multipliers are positive
			if preset.EconomicTuning.ReputationMultiplier <= 0 {
				t.Errorf("reputation multiplier must be >0, got %f", preset.EconomicTuning.ReputationMultiplier)
			}

			if preset.EconomicTuning.TokenCostMultiplier <= 0 {
				t.Errorf("token cost multiplier must be >0, got %f", preset.EconomicTuning.TokenCostMultiplier)
			}
		})
	}
}

func TestPresetDifferentiation(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	presetDir := filepath.Join(homeDir, ".wayfinder", "presets")
	if _, err := os.Stat(presetDir); os.IsNotExist(err) {
		t.Skip("~/.wayfinder/presets not installed; skipping preset differentiation tests")
	}
	loader := NewLoaderWithDir(presetDir)

	highQuality, _ := loader.Load("high-quality")
	fastIteration, _ := loader.Load("fast-iteration")
	researchHeavy, _ := loader.Load("research-heavy")

	// Verify test coverage differentiation
	if highQuality.TestCoverage.MinimumPercentage <= fastIteration.TestCoverage.MinimumPercentage {
		t.Errorf("high-quality should have higher coverage than fast-iteration")
	}

	if fastIteration.TestCoverage.MinimumPercentage <= researchHeavy.TestCoverage.MinimumPercentage {
		t.Errorf("fast-iteration should have higher coverage than research-heavy")
	}

	// Verify S9 validation depth differentiation
	if highQuality.PhaseGates.S9ValidationDepth != "comprehensive" {
		t.Errorf("high-quality should have comprehensive validation")
	}

	if fastIteration.PhaseGates.S9ValidationDepth != "standard" {
		t.Errorf("fast-iteration should have standard validation")
	}

	if researchHeavy.PhaseGates.S9ValidationDepth != "minimal" {
		t.Errorf("research-heavy should have minimal validation")
	}

	// Verify deploy gate differentiation
	if highQuality.PhaseGates.DeployGate != "blocking" {
		t.Errorf("high-quality should have blocking deploy gate")
	}

	if fastIteration.PhaseGates.DeployGate != "advisory" {
		t.Errorf("fast-iteration should have advisory deploy gate")
	}

	if researchHeavy.PhaseGates.DeployGate != "none" {
		t.Errorf("research-heavy should have no deploy gate")
	}
}

func TestInvalidPresetName(t *testing.T) {
	loader := NewLoader()

	_, err := loader.Load("INVALID-UPPERCASE")
	if err == nil {
		t.Error("should reject uppercase preset name")
	}
}

func TestFileSizeLimit(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	loader := NewLoaderWithDir(tempDir)

	// Create a preset file >100KB
	largeFile := filepath.Join(tempDir, "large-preset.yaml")
	data := make([]byte, 101*1024) // 101KB
	for i := range data {
		data[i] = '#' // Fill with comment characters
	}
	os.WriteFile(largeFile, data, 0644)

	_, err := loader.Load("large-preset")
	if err == nil {
		t.Error("should reject files >100KB")
	}

	if !containsString(err.Error(), "exceeds maximum size") {
		t.Errorf("error should mention size limit, got: %v", err)
	}
}

func TestReservedNames(t *testing.T) {
	tempDir := t.TempDir()
	loader := NewLoaderWithDir(tempDir)

	reserved := []string{"none", "default", "auto"}

	for _, name := range reserved {
		// Create a preset file with reserved name
		content := []byte(`version: "1.0"
name: "` + name + `"
description: "Test reserved name"
test_coverage:
  minimum_percentage: 80
  minimum_test_count: 5
  enforce_creative_tests: true
spec_alignment:
  allow_drift: false
  checkpoint_auditor_strictness: "maximum"
  freeze_during_build: true
phase_gates:
  s8_build_verification: true
  s9_validation_depth: "comprehensive"
  s9_halt_on_minor_issues: true
  deploy_gate: "blocking"
retrospective:
  mandatory: true
  structured_learnings: true
economic_tuning:
  reputation_multiplier: 1.0
  token_cost_multiplier: 1.0
  penalty_severity: "high"
`)

		presetPath := filepath.Join(tempDir, name+".yaml")
		os.WriteFile(presetPath, content, 0644)

		_, err := loader.Load(name)
		if err == nil {
			t.Errorf("should reject reserved name '%s'", name)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
