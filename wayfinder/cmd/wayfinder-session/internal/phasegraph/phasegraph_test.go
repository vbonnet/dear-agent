package phasegraph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Run("loads valid YAML config", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "phase-dependencies.yaml")

		yamlContent := `
dependencies:
  CHARTER: {}
  PROBLEM:
    CHARTER: summary
  BUILD:
    PLAN: full
    CHARTER: summary
v1_to_v2:
  D1: PROBLEM
  S8: BUILD
`
		err := os.WriteFile(configPath, []byte(yamlContent), 0o644)
		if err != nil {
			t.Fatalf("writing test config: %v", err)
		}

		cfg, err := LoadConfig(configPath)
		if err != nil {
			t.Fatalf("LoadConfig() error: %v", err)
		}

		if len(cfg.Dependencies) != 3 {
			t.Errorf("expected 3 phases, got %d", len(cfg.Dependencies))
		}

		if len(cfg.V1ToV2) != 2 {
			t.Errorf("expected 2 V1 mappings, got %d", len(cfg.V1ToV2))
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path/config.yaml")
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "bad.yaml")

		err := os.WriteFile(configPath, []byte(":::invalid:::yaml"), 0o644)
		if err != nil {
			t.Fatalf("writing test config: %v", err)
		}

		_, err = LoadConfig(configPath)
		if err == nil {
			t.Fatal("expected error for invalid YAML, got nil")
		}
	})

	t.Run("returns error for invalid strategy", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "bad-strategy.yaml")

		yamlContent := `
dependencies:
  BUILD:
    PLAN: invalid_strategy
`
		err := os.WriteFile(configPath, []byte(yamlContent), 0o644)
		if err != nil {
			t.Fatalf("writing test config: %v", err)
		}

		_, err = LoadConfig(configPath)
		if err == nil {
			t.Fatal("expected error for invalid strategy, got nil")
		}
	})
}

func TestParseConfig(t *testing.T) {
	t.Run("initializes nil maps", func(t *testing.T) {
		cfg, err := ParseConfig([]byte("{}"))
		if err != nil {
			t.Fatalf("ParseConfig() error: %v", err)
		}

		if cfg.Dependencies == nil {
			t.Error("Dependencies should be initialized, not nil")
		}

		if cfg.V1ToV2 == nil {
			t.Error("V1ToV2 should be initialized, not nil")
		}
	})
}

func TestGetDependencies(t *testing.T) {
	yamlContent := `
dependencies:
  CHARTER: {}
  PROBLEM:
    CHARTER: summary
  DESIGN:
    PROBLEM: summary
    RESEARCH: full
  BUILD:
    PLAN: full
    CHARTER: summary
    DESIGN: summary
  RETRO:
    CHARTER: summary
    BUILD: full
    SPEC: summary
v1_to_v2:
  D1: PROBLEM
  D3: DESIGN
  S8: BUILD
  S11: RETRO
`
	cfg, err := ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseConfig() error: %v", err)
	}

	t.Run("CHARTER has no dependencies", func(t *testing.T) {
		deps := cfg.GetDependencies("CHARTER")
		if len(deps) != 0 {
			t.Errorf("CHARTER should have 0 deps, got %d", len(deps))
		}
	})

	t.Run("PROBLEM depends on CHARTER as summary", func(t *testing.T) {
		deps := cfg.GetDependencies("PROBLEM")
		if len(deps) != 1 {
			t.Fatalf("PROBLEM should have 1 dep, got %d", len(deps))
		}

		if deps["CHARTER"] != Summary {
			t.Errorf("PROBLEM->CHARTER should be summary, got %q", deps["CHARTER"])
		}
	})

	t.Run("DESIGN has mixed strategies", func(t *testing.T) {
		deps := cfg.GetDependencies("DESIGN")
		if len(deps) != 2 {
			t.Fatalf("DESIGN should have 2 deps, got %d", len(deps))
		}

		if deps["PROBLEM"] != Summary {
			t.Errorf("DESIGN->PROBLEM should be summary, got %q", deps["PROBLEM"])
		}

		if deps["RESEARCH"] != Full {
			t.Errorf("DESIGN->RESEARCH should be full, got %q", deps["RESEARCH"])
		}
	})

	t.Run("BUILD has three dependencies", func(t *testing.T) {
		deps := cfg.GetDependencies("BUILD")
		if len(deps) != 3 {
			t.Fatalf("BUILD should have 3 deps, got %d", len(deps))
		}

		if deps["PLAN"] != Full {
			t.Errorf("BUILD->PLAN should be full, got %q", deps["PLAN"])
		}

		if deps["CHARTER"] != Summary {
			t.Errorf("BUILD->CHARTER should be summary, got %q", deps["CHARTER"])
		}

		if deps["DESIGN"] != Summary {
			t.Errorf("BUILD->DESIGN should be summary, got %q", deps["DESIGN"])
		}
	})

	t.Run("resolves V1 names automatically", func(t *testing.T) {
		deps := cfg.GetDependencies("S8")
		if len(deps) != 3 {
			t.Fatalf("S8 (BUILD) should have 3 deps, got %d", len(deps))
		}

		if deps["PLAN"] != Full {
			t.Errorf("S8->PLAN should be full, got %q", deps["PLAN"])
		}
	})

	t.Run("returns empty map for unknown phase", func(t *testing.T) {
		deps := cfg.GetDependencies("UNKNOWN")
		if len(deps) != 0 {
			t.Errorf("UNKNOWN should have 0 deps, got %d", len(deps))
		}
	})

	t.Run("returns copy not original map", func(t *testing.T) {
		deps := cfg.GetDependencies("BUILD")
		deps["INJECTED"] = Full

		original := cfg.GetDependencies("BUILD")
		if _, ok := original["INJECTED"]; ok {
			t.Error("mutation of returned map should not affect original")
		}
	})
}

func TestResolveV1Name(t *testing.T) {
	yamlContent := `
dependencies:
  PROBLEM: {}
  BUILD: {}
v1_to_v2:
  D1: PROBLEM
  D2: RESEARCH
  D3: DESIGN
  D4: SPEC
  S8: BUILD
  S11: RETRO
`
	cfg, err := ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseConfig() error: %v", err)
	}

	t.Run("maps V1 to V2", func(t *testing.T) {
		tests := map[string]string{
			"D1":  "PROBLEM",
			"D2":  "RESEARCH",
			"D3":  "DESIGN",
			"D4":  "SPEC",
			"S8":  "BUILD",
			"S11": "RETRO",
		}

		for v1, expectedV2 := range tests {
			got := cfg.ResolveV1Name(v1)
			if got != expectedV2 {
				t.Errorf("ResolveV1Name(%q) = %q, want %q", v1, got, expectedV2)
			}
		}
	})

	t.Run("returns V2 names unchanged", func(t *testing.T) {
		v2Names := []string{"CHARTER", "PROBLEM", "BUILD", "RETRO"}
		for _, name := range v2Names {
			got := cfg.ResolveV1Name(name)
			if got != name {
				t.Errorf("ResolveV1Name(%q) = %q, want %q", name, got, name)
			}
		}
	})

	t.Run("returns unknown names unchanged", func(t *testing.T) {
		got := cfg.ResolveV1Name("UNKNOWN")
		if got != "UNKNOWN" {
			t.Errorf("ResolveV1Name(UNKNOWN) = %q, want UNKNOWN", got)
		}
	})
}

func TestPhases(t *testing.T) {
	yamlContent := `
dependencies:
  CHARTER: {}
  PROBLEM:
    CHARTER: summary
  BUILD:
    PLAN: full
`
	cfg, err := ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseConfig() error: %v", err)
	}

	phases := cfg.Phases()
	if len(phases) != 3 {
		t.Errorf("expected 3 phases, got %d", len(phases))
	}

	// Check all expected phases are present (order not guaranteed)
	phaseSet := make(map[string]bool)
	for _, p := range phases {
		phaseSet[p] = true
	}

	for _, expected := range []string{"CHARTER", "PROBLEM", "BUILD"} {
		if !phaseSet[expected] {
			t.Errorf("expected phase %q not found in Phases()", expected)
		}
	}
}

func TestLoadConfigFromRealFile(t *testing.T) {
	// Test with the actual config file if it exists at the expected path
	// This is a smoke test to verify the real config parses correctly
	realPath := filepath.Join(
		"..", "..", "..", "..", "..", "config", "phase-dependencies.yaml",
	)

	if _, err := os.Stat(realPath); os.IsNotExist(err) {
		t.Skip("real config file not found at expected relative path")
	}

	cfg, err := LoadConfig(realPath)
	if err != nil {
		t.Fatalf("LoadConfig() on real config: %v", err)
	}

	// Verify key properties of the real config
	if len(cfg.Dependencies) < 5 {
		t.Errorf("real config should have at least 5 phases, got %d", len(cfg.Dependencies))
	}

	if len(cfg.V1ToV2) < 8 {
		t.Errorf("real config should have at least 8 V1 mappings, got %d", len(cfg.V1ToV2))
	}

	// CHARTER should have no dependencies
	charterDeps := cfg.GetDependencies("CHARTER")
	if len(charterDeps) != 0 {
		t.Errorf("CHARTER should have 0 deps in real config, got %d", len(charterDeps))
	}

	// BUILD should have dependencies
	buildDeps := cfg.GetDependencies("BUILD")
	if len(buildDeps) == 0 {
		t.Error("BUILD should have deps in real config")
	}
}
