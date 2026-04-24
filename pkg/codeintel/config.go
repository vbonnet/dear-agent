package codeintel

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFileName = ".codeintel.json"

// Registry holds the merged set of language specs (builtins + user overrides).
type Registry struct {
	Specs map[string]LanguageSpec
}

// NewRegistry creates a registry with built-in specs, optionally loading
// user overrides from .codeintel.json in the project directory.
func NewRegistry(cwd string) (*Registry, error) {
	r := &Registry{
		Specs: make(map[string]LanguageSpec, len(BuiltinSpecs)),
	}
	for k, v := range BuiltinSpecs {
		r.Specs[k] = v
	}

	configPath := filepath.Join(cwd, configFileName)
	if err := r.loadOverrides(configPath); err != nil {
		return nil, err
	}
	return r, nil
}

// loadOverrides reads a JSON config file and merges user-defined language specs
// into the registry. User specs override built-in specs with the same name.
func (r *Registry) loadOverrides(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no config file is fine
		}
		return err
	}

	var overrides struct {
		Languages map[string]LanguageSpec `json:"languages"`
	}
	if err := json.Unmarshal(data, &overrides); err != nil {
		return err
	}

	for k, v := range overrides.Languages {
		r.Specs[k] = v
	}
	return nil
}

// DetectLanguages scans cwd using the registry's specs.
func (r *Registry) DetectLanguages(cwd string) []LanguageSpec {
	var detected []LanguageSpec
	for _, spec := range r.Specs {
		if detectByManifest(cwd, spec) || detectBySourceGlob(cwd, spec) {
			detected = append(detected, spec)
		}
	}
	return detected
}

// Get returns the spec for a language name, or UnknownLanguage if not found.
func (r *Registry) Get(name string) LanguageSpec {
	if spec, ok := r.Specs[name]; ok {
		return spec
	}
	return UnknownLanguage
}
