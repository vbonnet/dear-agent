// Package acceptance loads the acceptance-criteria: section of
// .dear-agent.yml and exposes it as a typed list of exit conditions
// for the DEAR Define phase.
//
// Acceptance criteria are machine-checkable conditions a task must
// satisfy to be considered complete. They are NOT a blocking gate:
// the loader simply makes the conditions explicit so workers can see
// what they're being held to and the Audit phase can verify after the
// fact. Treat criteria as documentation that happens to be executable.
//
// Schema (under .dear-agent.yml > acceptance-criteria):
//
//	acceptance-criteria:
//	  - type: tests-pass
//	    command: "go test ./..."
//	  - type: lint-clean
//	    command: "golangci-lint run ./..."
//	  - type: no-regressions
//	    description: "No existing tests broken"
//
// Recognized types are validated at load time so a typo surfaces
// immediately rather than at task-completion time. Callers that want
// the raw rows can call LoadFile and walk the slice directly.
package acceptance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Type identifies a kind of acceptance criterion. The set is closed —
// adding a new criterion type requires updating this package so the
// validator and the worker-side checker stay in sync.
type Type string

const (
	// TypeTestsPass means "the named test command exits 0".
	TypeTestsPass Type = "tests-pass"
	// TypeLintClean means "the named lint command exits 0".
	TypeLintClean Type = "lint-clean"
	// TypeNoRegressions means "no test that previously passed now fails".
	// Verification is the Audit phase's job; the criterion is the
	// declaration that this property must hold.
	TypeNoRegressions Type = "no-regressions"
	// TypeCustom is an escape hatch: a free-form criterion identified
	// only by a description and (optionally) a command. Use sparingly —
	// the more criteria are typed, the more the Audit phase can do.
	TypeCustom Type = "custom"
)

// IsValid reports whether t is a known criterion type.
func (t Type) IsValid() bool {
	switch t {
	case TypeTestsPass, TypeLintClean, TypeNoRegressions, TypeCustom:
		return true
	default:
		return false
	}
}

// Criterion is one row from acceptance-criteria:.
type Criterion struct {
	Type        Type   `yaml:"type"`
	Command     string `yaml:"command,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// String returns a human-readable form suitable for showing a worker
// at task start.
func (c Criterion) String() string {
	parts := []string{string(c.Type)}
	if c.Command != "" {
		parts = append(parts, fmt.Sprintf("command=%q", c.Command))
	}
	if c.Description != "" {
		parts = append(parts, fmt.Sprintf("description=%q", c.Description))
	}
	return strings.Join(parts, " ")
}

// File is the slice of .dear-agent.yml fields the acceptance loader
// cares about. yaml.v3 ignores unknown keys so the struct can stay
// narrow alongside other consumers (audit, output routing).
type File struct {
	AcceptanceCriteria []Criterion `yaml:"acceptance-criteria,omitempty"`
}

// Load reads .dear-agent.yml from repoRoot and returns the parsed
// acceptance-criteria list. Returns an empty slice (not an error) when
// the file is absent or has no acceptance-criteria: block — repos that
// don't declare criteria simply don't have any.
func Load(repoRoot string) ([]Criterion, error) {
	path := filepath.Join(repoRoot, ".dear-agent.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("acceptance: read %s: %w", path, err)
	}
	return ParseBytes(data)
}

// ParseBytes parses raw YAML bytes (the contents of .dear-agent.yml)
// and returns the validated acceptance-criteria list. Exposed for
// callers that already have the file content in memory (tests, hooks).
func ParseBytes(data []byte) ([]Criterion, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("acceptance: parse: %w", err)
	}
	if err := Validate(f.AcceptanceCriteria); err != nil {
		return nil, err
	}
	return f.AcceptanceCriteria, nil
}

// Validate checks each criterion for shape errors (unknown type,
// missing fields the type requires). Returns the first error
// encountered with the row index so the user can find it.
func Validate(crits []Criterion) error {
	for i, c := range crits {
		if !c.Type.IsValid() {
			return fmt.Errorf("acceptance-criteria[%d]: unknown type %q (want one of: tests-pass, lint-clean, no-regressions, custom)", i, c.Type)
		}
		switch c.Type {
		case TypeTestsPass, TypeLintClean:
			if strings.TrimSpace(c.Command) == "" {
				return fmt.Errorf("acceptance-criteria[%d]: type %q requires a non-empty command", i, c.Type)
			}
		case TypeNoRegressions:
			// Description-only is fine; this criterion is declarative.
		case TypeCustom:
			if strings.TrimSpace(c.Description) == "" && strings.TrimSpace(c.Command) == "" {
				return fmt.Errorf("acceptance-criteria[%d]: type %q requires either description or command", i, c.Type)
			}
		}
	}
	return nil
}
