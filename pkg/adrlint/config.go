package adrlint

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Policy is the ADR governance slice of .dear-agent.yml.
type Policy struct {
	Scopes     []Scope     `yaml:"scopes"`
	Aggregates []Aggregate `yaml:"aggregates"`
	Exclusions []Exclusion `yaml:"exclusions"`
	MaxLines   int         `yaml:"max-lines"`
}

// Scope declares one directory-local identity sequence and its complete index.
type Scope struct {
	Path     string `yaml:"path"`
	Index    string `yaml:"index"`
	MaxLines int    `yaml:"max-lines,omitempty"`
}

// Aggregate declares one self-contained ADR.md record.
type Aggregate struct {
	Path     string `yaml:"path"`
	MaxLines int    `yaml:"max-lines,omitempty"`
}

// Exclusion removes generated ADR-shaped paths for a documented reason.
type Exclusion struct {
	Match  string `yaml:"match"`
	Reason string `yaml:"reason"`
}

func loadPolicy(configPath string) (Policy, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return Policy{}, fmt.Errorf("adrlint: read policy: %w", err)
	}
	var sections map[string]yaml.Node
	if err := yaml.Unmarshal(data, &sections); err != nil {
		return Policy{}, fmt.Errorf("adrlint: parse policy: %w", err)
	}
	governance := sections["adr-governance"]
	encoded, err := yaml.Marshal(&governance)
	if err != nil {
		return Policy{}, fmt.Errorf("adrlint: encode adr-governance policy: %w", err)
	}
	var policy Policy
	decoder := yaml.NewDecoder(bytes.NewReader(encoded))
	decoder.KnownFields(true)
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("adrlint: parse adr-governance policy: %w", err)
	}
	if err := validatePolicy(policy); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func validatePolicy(policy Policy) error {
	if len(policy.Scopes) == 0 {
		return fmt.Errorf("adrlint: adr-governance.scopes must not be empty")
	}
	if policy.MaxLines <= 0 {
		return fmt.Errorf("adrlint: adr-governance.max-lines must be positive")
	}
	seen, err := validateScopes(policy.Scopes)
	if err != nil {
		return err
	}
	if err := validateAggregates(policy.Aggregates, seen); err != nil {
		return err
	}
	return validateExclusions(policy.Exclusions)
}

func validateScopes(scopes []Scope) (map[string]bool, error) {
	seen := map[string]bool{}
	for i, scope := range scopes {
		if !cleanRelativePath(scope.Path) {
			return nil, fmt.Errorf("adrlint: scopes[%d].path must be a clean repository-relative path", i)
		}
		if seen[scope.Path] {
			return nil, fmt.Errorf("adrlint: duplicate scope path %q", scope.Path)
		}
		seen[scope.Path] = true
		if scope.Index == "" || filepath.Base(scope.Index) != scope.Index || filepath.Clean(scope.Index) != scope.Index {
			return nil, fmt.Errorf("adrlint: scopes[%d].index must be one clean filename", i)
		}
		if adrLikeFilename(scope.Index) {
			return nil, fmt.Errorf("adrlint: scopes[%d].index must not use an ADR-shaped filename", i)
		}
		if scope.MaxLines < 0 {
			return nil, fmt.Errorf("adrlint: scopes[%d].max-lines must not be negative", i)
		}
	}
	return seen, nil
}

func validateAggregates(aggregates []Aggregate, seen map[string]bool) error {
	for i, aggregate := range aggregates {
		if !cleanRelativePath(aggregate.Path) || path.Base(aggregate.Path) != "ADR.md" {
			return fmt.Errorf("adrlint: aggregates[%d].path must be a clean repository-relative ADR.md path", i)
		}
		if aggregate.MaxLines < 0 {
			return fmt.Errorf("adrlint: aggregates[%d].max-lines must not be negative", i)
		}
		if seen[aggregate.Path] {
			return fmt.Errorf("adrlint: duplicate governed path %q", aggregate.Path)
		}
		seen[aggregate.Path] = true
	}
	return nil
}

func validateExclusions(exclusions []Exclusion) error {
	for i, exclusion := range exclusions {
		if strings.TrimSpace(exclusion.Match) == "" {
			return fmt.Errorf("adrlint: exclusions[%d].match must not be empty", i)
		}
		if err := validateGlob(exclusion.Match); err != nil {
			return fmt.Errorf("adrlint: exclusions[%d].match: %w", i, err)
		}
		if strings.TrimSpace(exclusion.Reason) == "" {
			return fmt.Errorf("adrlint: exclusions[%d].reason must not be empty", i)
		}
	}
	return nil
}

func cleanRelativePath(value string) bool {
	return value != "" && value != "." && value != ".." && !path.IsAbs(value) &&
		!strings.HasPrefix(value, "../") && path.Clean(value) == value
}

func validateGlob(pattern string) error {
	for segment := range strings.SplitSeq(strings.Trim(path.Clean(pattern), "/"), "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, "candidate"); err != nil {
			return err
		}
	}
	return nil
}

func globPathMatch(pattern, name string) bool {
	patternParts := strings.Split(strings.Trim(path.Clean(pattern), "/"), "/")
	nameParts := strings.Split(strings.Trim(path.Clean(name), "/"), "/")
	type state struct{ pattern, name int }
	memo := map[state]bool{}
	seen := map[state]bool{}
	var match func(int, int) bool
	match = func(patternIndex, nameIndex int) bool {
		key := state{pattern: patternIndex, name: nameIndex}
		if seen[key] {
			return memo[key]
		}
		seen[key] = true
		var result bool
		switch {
		case patternIndex == len(patternParts):
			result = nameIndex == len(nameParts)
		case patternParts[patternIndex] == "**":
			result = match(patternIndex+1, nameIndex) ||
				(nameIndex < len(nameParts) && match(patternIndex, nameIndex+1))
		case nameIndex < len(nameParts):
			segmentMatch, _ := path.Match(patternParts[patternIndex], nameParts[nameIndex])
			result = segmentMatch && match(patternIndex+1, nameIndex+1)
		}
		memo[key] = result
		return result
	}
	return match(0, 0)
}
