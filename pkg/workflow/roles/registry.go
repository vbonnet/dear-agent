// Package roles is the workflow engine's role-based model registry.
//
// AI nodes declare a role (research, implementer, reviewer, …); the
// registry resolves the role to a primary/secondary/tertiary model tier
// based on capability, capacity, and cost. Migrating Opus 4.7 → Opus 5.0
// is a one-line edit to roles.yaml.
//
// The registry is loaded with one of three sources, in precedence order
// (matching ROADMAP.md "Role-based model mapping"):
//
//  1. Path passed to LoadFile (typically $DEAR_AGENT_ROLES).
//  2. ./.dear-agent/roles.yaml in the current directory.
//  3. ~/.config/dear-agent/roles.yaml in the user's home.
//  4. Built-in defaults (BuiltinRegistry).
//
// Resolution is via Resolver — see resolver.go for the algorithm.
package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Registry is the parsed roles.yaml content. A nil or zero-value
// Registry is treated as "no roles defined" — Resolver.Resolve will
// fall back to BuiltinRegistry.
type Registry struct {
	Version  int             `yaml:"version,omitempty"`
	Defaults Defaults        `yaml:"defaults,omitempty"`
	Roles    map[string]Role `yaml:"roles"`
}

// Defaults are the fallback values applied to a tier when the per-tier
// fields are unset. Every field is optional — empty strings/zero values
// signal "no default at this layer".
type Defaults struct {
	Effort     string `yaml:"effort,omitempty"`
	MaxContext int    `yaml:"max_context,omitempty"`
}

// Role is one entry in the registry — a logical role with up to three
// model tiers. Every tier is optional; Resolver tries them in order and
// returns the first that satisfies the node's filters.
type Role struct {
	Description  string   `yaml:"description,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty"`
	Primary      *Tier    `yaml:"primary,omitempty"`
	Secondary    *Tier    `yaml:"secondary,omitempty"`
	Tertiary     *Tier    `yaml:"tertiary,omitempty"`
}

// CostPerMTok is the per-million-token cost split between input and
// output tokens. Used by the budget filter — a node with max_dollars set
// rejects tiers whose minimum cost (input cost only) exceeds the cap.
type CostPerMTok struct {
	Input  float64 `yaml:"input,omitempty"`
	Output float64 `yaml:"output,omitempty"`
}

// Tier is one model option within a role. Model is the canonical model
// id (matches pkg/costtrack); Effort and MaxContext override the
// registry defaults; Capabilities adds tier-specific capabilities on
// top of the role-level set; CostPerMTok funds the cost filter.
type Tier struct {
	Model        string      `yaml:"model"`
	Effort       string      `yaml:"effort,omitempty"`
	MaxContext   int         `yaml:"max_context,omitempty"`
	Capabilities []string    `yaml:"capabilities,omitempty"`
	CostPerMTok  CostPerMTok `yaml:"cost_per_mtok,omitempty"`
}

// LoadFile parses a roles.yaml file at path and validates the result.
// Returns an error if the file is missing, malformed, or fails Validate.
func LoadFile(path string) (*Registry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled config
	if err != nil {
		return nil, fmt.Errorf("roles: read %s: %w", path, err)
	}
	return LoadBytes(data)
}

// LoadBytes parses YAML from an in-memory buffer. Used by tests and
// callers embedding a registry via go:embed.
func LoadBytes(data []byte) (*Registry, error) {
	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("roles: parse yaml: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// AutoLoad walks the resolution order documented above and returns the
// first registry it can read. If none of the user-provided paths
// resolve, returns BuiltinRegistry. Errors from os.ReadFile are silent
// (a missing file is the expected case); parse errors do propagate.
//
// envPath, when non-empty, is the first path tried (typically
// os.Getenv("DEAR_AGENT_ROLES")). cwd is the current working directory
// (usually os.Getwd()); home is the user's home dir.
func AutoLoad(envPath, cwd, home string) (*Registry, string, error) {
	candidates := make([]string, 0, 3)
	if envPath != "" {
		candidates = append(candidates, envPath)
	}
	if cwd != "" {
		candidates = append(candidates, filepath.Join(cwd, ".dear-agent", "roles.yaml"))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".config", "dear-agent", "roles.yaml"))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		reg, err := LoadFile(p)
		if err != nil {
			return nil, p, err
		}
		return reg, p, nil
	}
	return BuiltinRegistry(), "<builtin>", nil
}

// Validate checks that the registry's roles are well-formed. Returns
// nil on the empty (zero) registry — that is treated as "fall back to
// built-ins".
func (r *Registry) Validate() error {
	if r == nil {
		return nil
	}
	for name, role := range r.Roles {
		if name == "" {
			return fmt.Errorf("roles: empty role name")
		}
		// At least one tier must be defined; a role with no tiers can
		// never resolve.
		if role.Primary == nil && role.Secondary == nil && role.Tertiary == nil {
			return fmt.Errorf("role %q: must define at least one tier (primary/secondary/tertiary)", name)
		}
		for tierName, tier := range tiers(role) {
			if tier == nil {
				continue
			}
			if tier.Model == "" {
				return fmt.Errorf("role %q tier %q: model is required", name, tierName)
			}
		}
	}
	return nil
}

// RoleNames returns the role names in deterministic alphabetical order.
// Useful for `roles list` output.
func (r *Registry) RoleNames() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.Roles))
	for name := range r.Roles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Lookup returns the named role and ok=true if it exists. The returned
// Role is a copy — the caller may mutate it without affecting the
// registry.
func (r *Registry) Lookup(name string) (Role, bool) {
	if r == nil {
		return Role{}, false
	}
	role, ok := r.Roles[name]
	return role, ok
}

// tiers iterates over a role's tiers in resolution order. The map's
// stable iteration order is what makes the resolver predictable.
func tiers(role Role) map[string]*Tier {
	return map[string]*Tier{
		"primary":   role.Primary,
		"secondary": role.Secondary,
		"tertiary":  role.Tertiary,
	}
}

// orderedTiers returns the role's tiers in resolution order. Used by
// Resolver to deterministically iterate primary → secondary → tertiary.
func orderedTiers(role Role) []namedTier {
	out := make([]namedTier, 0, 3)
	if role.Primary != nil {
		out = append(out, namedTier{name: "primary", tier: role.Primary})
	}
	if role.Secondary != nil {
		out = append(out, namedTier{name: "secondary", tier: role.Secondary})
	}
	if role.Tertiary != nil {
		out = append(out, namedTier{name: "tertiary", tier: role.Tertiary})
	}
	return out
}

type namedTier struct {
	name string
	tier *Tier
}

// BuiltinRegistry returns the compiled-in default registry. Used as a
// last-resort fallback by AutoLoad and as the seed for tests that don't
// want to wire a roles.yaml. Kept small on purpose — operators are
// expected to override per-environment.
func BuiltinRegistry() *Registry {
	return &Registry{
		Version: 1,
		Defaults: Defaults{
			Effort:     "high",
			MaxContext: 200000,
		},
		Roles: map[string]Role{
			"research": {
				Description:  "Long-context document analysis with citations",
				Capabilities: []string{"long_context", "citations"},
				Primary: &Tier{
					Model:      "claude-opus-4-7",
					Effort:     "max",
					MaxContext: 1000000,
				},
				Secondary: &Tier{
					Model:      "gemini-3.1-pro",
					Effort:     "high",
					MaxContext: 1000000,
				},
				Tertiary: &Tier{
					Model:  "gpt-5.5-pro",
					Effort: "high",
				},
			},
			"implementer": {
				Description:  "Code synthesis with tool use and patch application",
				Capabilities: []string{"tool_use", "code_synthesis"},
				Primary: &Tier{
					Model:  "claude-sonnet-4-6",
					Effort: "high",
				},
				Secondary: &Tier{
					Model:  "claude-opus-4-7",
					Effort: "high",
				},
			},
			"reviewer": {
				Description:  "Adversarial critique of artifacts produced by implementer",
				Capabilities: []string{"reasoning"},
				Primary: &Tier{
					Model:  "claude-opus-4-7",
					Effort: "max",
				},
			},
		},
	}
}
