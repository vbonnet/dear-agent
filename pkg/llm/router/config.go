// Package router implements role-based model routing on top of
// pkg/llm/provider. A workflow author names an intent ("research",
// "implementer") and the router picks a concrete model+provider, falling
// through a primary→secondary→tertiary chain when calls fail.
//
// See docs/adrs/ADR-012-provider-transport-layer.md for the design.
package router

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RoleSpec is the chain of models the router will try for one role.
// An empty rung is skipped; the router walks Primary → Secondary →
// Tertiary in order.
type RoleSpec struct {
	Primary   string `yaml:"primary,omitempty"`
	Secondary string `yaml:"secondary,omitempty"`
	Tertiary  string `yaml:"tertiary,omitempty"`
}

// Candidates returns the non-empty rungs of a RoleSpec in order.
// The router uses this to drive its fallback loop.
func (s RoleSpec) Candidates() []string {
	out := make([]string, 0, 3)
	for _, m := range [...]string{s.Primary, s.Secondary, s.Tertiary} {
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

// Config is the on-disk shape of config/roles.yaml. Version lets us
// evolve the schema without silently misreading an older file.
type Config struct {
	Version     int                 `yaml:"version"`
	DefaultRole string              `yaml:"default_role,omitempty"`
	Roles       map[string]RoleSpec `yaml:"roles"`
}

// LoadConfig reads and validates a roles config file. The validation is
// intentionally lenient: an empty Roles map is allowed (the router will
// fall through to literal model ids on every call), and unknown fields
// are ignored so newer config files don't break older binaries.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("router: read roles config %q: %w", path, err)
	}
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("router: parse roles config %q: %w", path, err)
	}
	return cfg, nil
}

// ParseConfig parses roles config YAML from a byte slice. Useful for
// tests and for callers that have already loaded the file themselves
// (e.g. an embedded fs).
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Version == 0 {
		// Treat missing version as v1; bump only when an incompatible
		// change ships.
		cfg.Version = 1
	}
	if cfg.Version != 1 {
		return nil, fmt.Errorf("unsupported roles config version: %d (this binary supports 1)", cfg.Version)
	}
	if cfg.Roles == nil {
		cfg.Roles = map[string]RoleSpec{}
	}
	if cfg.DefaultRole != "" {
		if _, ok := cfg.Roles[cfg.DefaultRole]; !ok {
			return nil, fmt.Errorf("default_role %q is not defined in roles", cfg.DefaultRole)
		}
	}
	for name, spec := range cfg.Roles {
		if name == "" {
			return nil, fmt.Errorf("empty role name")
		}
		if len(spec.Candidates()) == 0 {
			return nil, fmt.Errorf("role %q: no model candidates configured", name)
		}
	}
	return &cfg, nil
}
