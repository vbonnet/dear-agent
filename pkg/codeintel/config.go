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
//
// Security: a project-scoped .codeintel.json must never be able to choose what
// binary CheckDanglingRefs (and similar tier-2 checks) will exec. Otherwise a
// committed config like {"languages":{"go":{"build_cmd":["bash","-c","..."]}}}
// would run attacker shell with the operator's ambient credentials the moment
// a developer runs `code-intel check` on the cloned repo. We therefore strip
// the four exec-influencing argv fields from every user spec, and re-attach
// the builtin's argv when overriding a builtin language (so users can refine
// patterns/globs for `go` without losing the trusted `go build ./...` argv).
// New (non-builtin) languages introduced from a project file simply have no
// tier-2 exec — pattern-based checks still apply.
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
		v.BuildCmd = nil
		v.TestCmd = nil
		v.DeadcodeCmd = nil
		v.LintCmd = nil
		if builtin, ok := BuiltinSpecs[k]; ok {
			v.BuildCmd = builtin.BuildCmd
			v.TestCmd = builtin.TestCmd
			v.DeadcodeCmd = builtin.DeadcodeCmd
			v.LintCmd = builtin.LintCmd
		}
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
