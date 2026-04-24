package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry manages the model configuration database.
type Registry struct {
	models map[string]*ModelConfig
	path   string
}

// registryData is the YAML structure for models.yaml
type registryData struct {
	Models []ModelConfig `yaml:"models"`
}

// NewRegistry creates a new registry from the given YAML file path.
// If path is empty, uses default location: ~/.engram/context/models.yaml
// Falls back to embedded models.yaml in package directory.
func NewRegistry(path string) (*Registry, error) {
	if path == "" {
		// Try default location first
		home, err := os.UserHomeDir()
		if err == nil {
			defaultPath := filepath.Join(home, ".engram", "context", "models.yaml")
			if _, err := os.Stat(defaultPath); err == nil {
				path = defaultPath
			}
		}

		// If still empty, use embedded version (relative to package)
		if path == "" {
			// Assumes models.yaml is in the same directory as registry.go
			path = filepath.Join(filepath.Dir(getCurrentFile()), "models.yaml")
		}
	}

	return loadRegistry(path)
}

// getCurrentFile returns the current source file path (for finding models.yaml)
func getCurrentFile() string {
	// This will be compiled into the binary, so we need to use a build-time constant
	// or environment variable. For now, assume models.yaml is in the same package.
	return "."
}

// loadRegistry loads the registry from a YAML file.
func loadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry file %s: %w", path, err)
	}

	var rd registryData
	if err := yaml.Unmarshal(data, &rd); err != nil {
		return nil, fmt.Errorf("failed to parse registry YAML: %w", err)
	}

	// Build index by model ID
	models := make(map[string]*ModelConfig)
	for i := range rd.Models {
		model := &rd.Models[i]
		models[model.ModelID] = model
	}

	return &Registry{
		models: models,
		path:   path,
	}, nil
}

// GetModel retrieves a model configuration by ID.
// Returns nil if model not found.
func (r *Registry) GetModel(modelID string) *ModelConfig {
	// Try exact match first
	if model, ok := r.models[modelID]; ok {
		return model
	}

	// Try normalized match (hyphens vs underscores, case-insensitive)
	normalizedID := normalizeModelID(modelID)
	for id, model := range r.models {
		if normalizeModelID(id) == normalizedID {
			return model
		}
	}

	// Not found - return default configuration
	return r.models["default"]
}

// normalizeModelID converts model ID to canonical form for matching.
func normalizeModelID(id string) string {
	// Lowercase and replace underscores with hyphens
	id = strings.ToLower(id)
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// GetThresholds calculates absolute token thresholds for a model.
func (r *Registry) GetThresholds(modelID string) (*Thresholds, error) {
	model := r.GetModel(modelID)
	if model == nil {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}

	return &Thresholds{
		SweetSpotTokens:     int(float64(model.MaxContextTokens) * model.SweetSpotThreshold),
		SweetSpotPercentage: model.SweetSpotThreshold,
		WarningTokens:       int(float64(model.MaxContextTokens) * model.WarningThreshold),
		WarningPercentage:   model.WarningThreshold,
		DangerTokens:        int(float64(model.MaxContextTokens) * model.DangerThreshold),
		DangerPercentage:    model.DangerThreshold,
		CriticalTokens:      int(float64(model.MaxContextTokens) * model.CriticalThreshold),
		CriticalPercentage:  model.CriticalThreshold,
		MaxTokens:           model.MaxContextTokens,
	}, nil
}

// ListModels returns all model IDs in the registry.
func (r *Registry) ListModels() []string {
	ids := make([]string, 0, len(r.models))
	for id := range r.models {
		ids = append(ids, id)
	}
	return ids
}

// GetModelCapabilities returns the parsed capabilities for a given model.
// If the model has no capabilities defined, returns safe defaults (all zero values).
func (r *Registry) GetModelCapabilities(modelID string) (*ModelCapabilities, error) {
	model := r.GetModel(modelID)
	if model == nil {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}

	if model.Capabilities != nil {
		return model.Capabilities, nil
	}

	// Return safe defaults when no capabilities are defined
	return &ModelCapabilities{
		ContextStability: "medium",
		SelfEvalQuality:  "medium",
		LongContext:      false,
	}, nil
}

// GetScaffoldingOverrides returns the scaffolding overrides for a given model.
// If the model has no overrides defined, returns safe defaults (no overrides).
func (r *Registry) GetScaffoldingOverrides(modelID string) (*ScaffoldingOverrides, error) {
	model := r.GetModel(modelID)
	if model == nil {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}

	if model.ScaffoldingOverrides != nil {
		return model.ScaffoldingOverrides, nil
	}

	// Return safe defaults: no overrides (most conservative behavior)
	return &ScaffoldingOverrides{
		SkipContextResets:   false,
		EvaluatorRequired:   true,
		LiteProcessEligible: false,
	}, nil
}

// GetPath returns the file path of the loaded registry.
func (r *Registry) GetPath() string {
	return r.path
}
