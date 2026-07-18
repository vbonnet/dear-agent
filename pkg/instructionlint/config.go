package instructionlint

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type repositoryConfig struct {
	InstructionPolicy Policy `yaml:"instruction-policy"`
}

// Policy declares the active instruction surfaces and exact temporary debt.
type Policy struct {
	Surfaces       []Surface   `yaml:"surfaces"`
	Exclusions     []Exclusion `yaml:"exclusions"`
	ExclusionsFile string      `yaml:"exclusions-file"`
}

type exclusionFile struct {
	Exclusions []Exclusion `yaml:"exclusions"`
}

// Surface is one owned Git-tracked instruction path pattern.
type Surface struct {
	Match string `yaml:"match"`
	Owner string `yaml:"owner"`
}

// Exclusion suppresses an exact number of one known finding until its owner
// removes the underlying legacy text. Changed or missing text is reported.
type Exclusion struct {
	Path    string `yaml:"path"`
	Rule    string `yaml:"rule"`
	Excerpt string `yaml:"excerpt"`
	Count   int    `yaml:"count"`
	Owner   string `yaml:"owner"`
	Reason  string `yaml:"reason"`
}

func loadPolicy(configPath string) (Policy, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return Policy{}, fmt.Errorf("instructionlint: read policy: %w", err)
	}
	var config repositoryConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Policy{}, fmt.Errorf("instructionlint: parse policy: %w", err)
	}
	if err := loadPolicyExclusions(configPath, &config.InstructionPolicy); err != nil {
		return Policy{}, err
	}
	if err := validatePolicy(config.InstructionPolicy); err != nil {
		return Policy{}, err
	}
	return config.InstructionPolicy, nil
}

func loadPolicyExclusions(configPath string, policy *Policy) error {
	if policy.ExclusionsFile == "" {
		return nil
	}
	if len(policy.Exclusions) > 0 {
		return fmt.Errorf("instructionlint: use exclusions or exclusions-file, not both")
	}
	if !cleanRelativePath(policy.ExclusionsFile) {
		return fmt.Errorf("instructionlint: instruction-policy.exclusions-file must be a clean repository-relative path")
	}
	path := filepath.Join(filepath.Dir(configPath), filepath.FromSlash(policy.ExclusionsFile))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("instructionlint: read exclusions: %w", err)
	}
	var file exclusionFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("instructionlint: parse exclusions: %w", err)
	}
	policy.Exclusions = file.Exclusions
	return nil
}

func validatePolicy(policy Policy) error {
	if len(policy.Surfaces) == 0 {
		return fmt.Errorf("instructionlint: instruction-policy.surfaces must not be empty")
	}
	seenSurfaces := map[string]bool{}
	for i, surface := range policy.Surfaces {
		if !cleanPattern(surface.Match) {
			return fmt.Errorf("instructionlint: surfaces[%d].match must be a clean repository-relative pattern", i)
		}
		if seenSurfaces[surface.Match] {
			return fmt.Errorf("instructionlint: duplicate surface pattern %q", surface.Match)
		}
		seenSurfaces[surface.Match] = true
		if strings.TrimSpace(surface.Owner) == "" {
			return fmt.Errorf("instructionlint: surfaces[%d].owner must not be empty", i)
		}
	}
	seenExclusions := map[string]bool{}
	for i, exclusion := range policy.Exclusions {
		if !cleanRelativePath(exclusion.Path) {
			return fmt.Errorf("instructionlint: exclusions[%d].path must be a clean repository-relative path", i)
		}
		if !knownRule(exclusion.Rule) {
			return fmt.Errorf("instructionlint: exclusions[%d].rule %q is unknown", i, exclusion.Rule)
		}
		if strings.TrimSpace(exclusion.Excerpt) == "" || exclusion.Count <= 0 ||
			strings.TrimSpace(exclusion.Owner) == "" || strings.TrimSpace(exclusion.Reason) == "" {
			return fmt.Errorf("instructionlint: exclusions[%d] requires excerpt, positive count, owner, and reason", i)
		}
		key := exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt)
		if seenExclusions[key] {
			return fmt.Errorf("instructionlint: duplicate exclusion %q", key)
		}
		seenExclusions[key] = true
	}
	return nil
}

func cleanPattern(value string) bool {
	return value != "" && value != "." && value != ".." && !path.IsAbs(value) &&
		!strings.HasPrefix(value, "../") && path.Clean(value) == value
}

func cleanRelativePath(value string) bool {
	return cleanPattern(value) && !strings.ContainsAny(value, "*?[")
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
