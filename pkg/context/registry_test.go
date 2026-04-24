package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	// Create temp registry file
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "test-model"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources:
      - "Test benchmark"
    notes: "Test model"
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
  - model_id: "default"
    provider: "unknown"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: "Default fallback"
    confidence: "N/A"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Test loading registry
	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	assert.Equal(t, registryPath, registry.GetPath())

	// Test model lookup
	model := registry.GetModel("test-model")
	require.NotNil(t, model)
	assert.Equal(t, "test-model", model.ModelID)
	assert.Equal(t, "test", model.Provider)
	assert.Equal(t, 100000, model.MaxContextTokens)
	assert.Equal(t, 0.60, model.SweetSpotThreshold)
}

func TestGetModel(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "claude-sonnet-4.5"
    provider: "anthropic"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
  - model_id: "claude-sonnet-4-5"
    provider: "anthropic"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: "Alias"
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
  - model_id: "default"
    provider: "unknown"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "N/A"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	tests := []struct {
		name     string
		modelID  string
		expected string
	}{
		{"exact match", "claude-sonnet-4.5", "claude-sonnet-4.5"},
		{"normalized match", "claude_sonnet_4_5", "claude-sonnet-4-5"}, // Matches alias
		{"case insensitive", "CLAUDE-SONNET-4.5", "claude-sonnet-4.5"},
		{"not found fallback", "unknown-model", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := registry.GetModel(tt.modelID)
			require.NotNil(t, model)
			assert.Equal(t, tt.expected, model.ModelID)
		})
	}
}

func TestGetThresholds(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "test-model"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	thresholds, err := registry.GetThresholds("test-model")
	require.NoError(t, err)
	assert.NotNil(t, thresholds)

	assert.Equal(t, 60000, thresholds.SweetSpotTokens)
	assert.Equal(t, 0.60, thresholds.SweetSpotPercentage)
	assert.Equal(t, 70000, thresholds.WarningTokens)
	assert.Equal(t, 0.70, thresholds.WarningPercentage)
	assert.Equal(t, 80000, thresholds.DangerTokens)
	assert.Equal(t, 0.80, thresholds.DangerPercentage)
	assert.Equal(t, 90000, thresholds.CriticalTokens)
	assert.Equal(t, 0.90, thresholds.CriticalPercentage)
	assert.Equal(t, 100000, thresholds.MaxTokens)
}

func TestListModels(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "model-a"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
  - model_id: "model-b"
    provider: "test"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.50
    warning_threshold: 0.60
    danger_threshold: 0.70
    critical_threshold: 0.80
    benchmark_sources: []
    notes: ""
    confidence: "MEDIUM"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	models := registry.ListModels()
	assert.Len(t, models, 2)
	assert.Contains(t, models, "model-a")
	assert.Contains(t, models, "model-b")
}

func TestGetModelCapabilities(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "model-with-caps"
    provider: "test"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
    capabilities:
      context_stability: high
      self_eval_quality: medium
      long_context: true
    scaffolding_overrides:
      skip_context_resets: true
      evaluator_required: false
      lite_process_eligible: true
  - model_id: "model-without-caps"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.50
    warning_threshold: 0.60
    danger_threshold: 0.70
    critical_threshold: 0.80
    benchmark_sources: []
    notes: ""
    confidence: "LOW"
    last_updated: "2026-03-18T00:00:00Z"
  - model_id: "default"
    provider: "unknown"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "N/A"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	t.Run("model with capabilities", func(t *testing.T) {
		caps, err := registry.GetModelCapabilities("model-with-caps")
		require.NoError(t, err)
		assert.Equal(t, "high", caps.ContextStability)
		assert.Equal(t, "medium", caps.SelfEvalQuality)
		assert.True(t, caps.LongContext)
	})

	t.Run("model without capabilities returns safe defaults", func(t *testing.T) {
		caps, err := registry.GetModelCapabilities("model-without-caps")
		require.NoError(t, err)
		assert.Equal(t, "medium", caps.ContextStability)
		assert.Equal(t, "medium", caps.SelfEvalQuality)
		assert.False(t, caps.LongContext)
	})

	t.Run("unknown model returns default capabilities", func(t *testing.T) {
		caps, err := registry.GetModelCapabilities("nonexistent-model")
		require.NoError(t, err) // Falls back to "default" model
		assert.Equal(t, "medium", caps.ContextStability)
	})
}

func TestGetScaffoldingOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "model-with-overrides"
    provider: "test"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
    scaffolding_overrides:
      skip_context_resets: true
      evaluator_required: false
      lite_process_eligible: true
  - model_id: "model-without-overrides"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.50
    warning_threshold: 0.60
    danger_threshold: 0.70
    critical_threshold: 0.80
    benchmark_sources: []
    notes: ""
    confidence: "LOW"
    last_updated: "2026-03-18T00:00:00Z"
  - model_id: "default"
    provider: "unknown"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "N/A"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	t.Run("model with overrides", func(t *testing.T) {
		overrides, err := registry.GetScaffoldingOverrides("model-with-overrides")
		require.NoError(t, err)
		assert.True(t, overrides.SkipContextResets)
		assert.False(t, overrides.EvaluatorRequired)
		assert.True(t, overrides.LiteProcessEligible)
	})

	t.Run("model without overrides returns safe defaults", func(t *testing.T) {
		overrides, err := registry.GetScaffoldingOverrides("model-without-overrides")
		require.NoError(t, err)
		assert.False(t, overrides.SkipContextResets)
		assert.True(t, overrides.EvaluatorRequired)
		assert.False(t, overrides.LiteProcessEligible)
	})
}

func TestModelsYAMLParsesWithCapabilities(t *testing.T) {
	registryPath := filepath.Join(".", "models.yaml")
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Skip("models.yaml not found in current directory")
	}

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	// Verify Opus 4.6 has capabilities set
	model := registry.GetModel("claude-opus-4.6")
	require.NotNil(t, model)
	require.NotNil(t, model.Capabilities, "claude-opus-4.6 should have capabilities")
	assert.Equal(t, "high", model.Capabilities.ContextStability)
	assert.True(t, model.Capabilities.LongContext)

	require.NotNil(t, model.ScaffoldingOverrides, "claude-opus-4.6 should have scaffolding overrides")
	assert.True(t, model.ScaffoldingOverrides.SkipContextResets)
	assert.False(t, model.ScaffoldingOverrides.EvaluatorRequired)
	assert.True(t, model.ScaffoldingOverrides.LiteProcessEligible)

	// Verify default model has no capabilities (nil)
	defaultModel := registry.GetModel("default")
	require.NotNil(t, defaultModel)
	assert.Nil(t, defaultModel.Capabilities, "default model should have no capabilities")
	assert.Nil(t, defaultModel.ScaffoldingOverrides, "default model should have no scaffolding overrides")
}

func TestNormalizeModelID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet-4.5", "claude-sonnet-4.5"},
		{"claude_sonnet_4_5", "claude-sonnet-4-5"},
		{"CLAUDE-SONNET-4.5", "claude-sonnet-4.5"},
		{"Claude_Sonnet_4_5", "claude-sonnet-4-5"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeModelID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadRegistryErrors(t *testing.T) {
	t.Run("nonexistent file", func(t *testing.T) {
		_, err := loadRegistry("/nonexistent/path/models.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read registry file")
	})

	t.Run("invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		registryPath := filepath.Join(tmpDir, "models.yaml")
		err := os.WriteFile(registryPath, []byte("{{invalid yaml"), 0644)
		require.NoError(t, err)

		_, err = loadRegistry(registryPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse registry YAML")
	})

	t.Run("empty models list", func(t *testing.T) {
		tmpDir := t.TempDir()
		registryPath := filepath.Join(tmpDir, "models.yaml")
		err := os.WriteFile(registryPath, []byte("models: []\n"), 0644)
		require.NoError(t, err)

		registry, err := loadRegistry(registryPath)
		require.NoError(t, err)
		assert.Len(t, registry.ListModels(), 0)
	})
}

func TestGetThresholdsNoDefault(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "only-model"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	model := registry.GetModel("nonexistent")
	assert.Nil(t, model)

	_, err = registry.GetThresholds("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")

	_, err = registry.GetModelCapabilities("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")

	_, err = registry.GetScaffoldingOverrides("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestNewRegistryWithExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "explicit-model"
    provider: "test"
    max_context_tokens: 100000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := NewRegistry(registryPath)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	assert.Equal(t, registryPath, registry.GetPath())

	model := registry.GetModel("explicit-model")
	require.NotNil(t, model)
	assert.Equal(t, "explicit-model", model.ModelID)
}

func TestGetCurrentFile(t *testing.T) {
	result := getCurrentFile()
	assert.Equal(t, ".", result)
}

func TestModelsYAMLAllEntriesParse(t *testing.T) {
	registryPath := filepath.Join(".", "models.yaml")
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Skip("models.yaml not found in current directory")
	}

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	models := registry.ListModels()
	assert.Greater(t, len(models), 0, "should have at least one model")

	for _, modelID := range models {
		t.Run(modelID, func(t *testing.T) {
			model := registry.GetModel(modelID)
			require.NotNil(t, model)
			assert.NotEmpty(t, model.ModelID)
			assert.NotEmpty(t, model.Provider)
			assert.Greater(t, model.MaxContextTokens, 0)
			assert.Greater(t, model.SweetSpotThreshold, 0.0)
			assert.Greater(t, model.WarningThreshold, 0.0)
			assert.Greater(t, model.DangerThreshold, 0.0)
			assert.Greater(t, model.CriticalThreshold, 0.0)

			assert.LessOrEqual(t, model.SweetSpotThreshold, model.WarningThreshold)
			assert.LessOrEqual(t, model.WarningThreshold, model.DangerThreshold)
			assert.LessOrEqual(t, model.DangerThreshold, model.CriticalThreshold)

			thresholds, err := registry.GetThresholds(modelID)
			require.NoError(t, err)
			assert.Greater(t, thresholds.MaxTokens, 0)

			caps, err := registry.GetModelCapabilities(modelID)
			require.NoError(t, err)
			assert.NotNil(t, caps)

			overrides, err := registry.GetScaffoldingOverrides(modelID)
			require.NoError(t, err)
			assert.NotNil(t, overrides)
		})
	}
}

func TestModelsYAMLProviderCoverage(t *testing.T) {
	registryPath := filepath.Join(".", "models.yaml")
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Skip("models.yaml not found in current directory")
	}

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	providers := make(map[string]bool)
	for _, modelID := range registry.ListModels() {
		model := registry.GetModel(modelID)
		providers[model.Provider] = true
	}

	assert.True(t, providers["anthropic"], "should have anthropic models")
	assert.True(t, providers["google"], "should have google models")
	assert.True(t, providers["openai"], "should have openai models")
	assert.True(t, providers["unknown"], "should have default/unknown provider")
}
