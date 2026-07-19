package docaudit

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type repositoryConfig struct {
	LivingDocs *Policy `yaml:"living-docs"`
}

// Policy is the living-docs slice of .dear-agent.yml.
type Policy struct {
	Baseline string    `yaml:"baseline"`
	Surfaces []Surface `yaml:"surfaces"`
}

// Surface declares ownership and age policy for one path set.
type Surface struct {
	Name       string `yaml:"name"`
	Match      string `yaml:"match"`
	Owner      string `yaml:"owner"`
	MaxAgeDays int    `yaml:"max-age-days"`
}

func loadPolicy(configPath string) (Policy, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return Policy{}, fmt.Errorf("docaudit: read policy: %w", err)
	}
	policy, present, err := parsePolicy(data)
	if err != nil {
		return Policy{}, err
	}
	if !present {
		return Policy{}, fmt.Errorf("docaudit: living-docs policy is missing")
	}
	return policy, nil
}

func loadPolicyAtRef(ctx context.Context, root, ref string) (Policy, bool, error) {
	data, present, err := loadFileAtRef(ctx, root, ref, ".dear-agent.yml")
	if err != nil || !present {
		return Policy{}, present, err
	}
	return parsePolicy(data)
}

func parsePolicy(data []byte) (Policy, bool, error) {
	var cfg repositoryConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Policy{}, false, fmt.Errorf("docaudit: parse policy: %w", err)
	}
	if cfg.LivingDocs == nil {
		return Policy{}, false, nil
	}
	if err := validatePolicy(*cfg.LivingDocs); err != nil {
		return Policy{}, false, err
	}
	return *cfg.LivingDocs, true, nil
}

func validatePolicy(policy Policy) error {
	baselineValue := strings.TrimSpace(policy.Baseline)
	baseline := filepath.ToSlash(filepath.Clean(baselineValue))
	if baseline == "." || baseline == "" || filepath.IsAbs(baselineValue) || baseline == ".." || strings.HasPrefix(baseline, "../") {
		return fmt.Errorf("docaudit: living-docs.baseline must be a repository-relative path")
	}
	if len(policy.Surfaces) == 0 {
		return fmt.Errorf("docaudit: living-docs.surfaces must not be empty")
	}
	names := make(map[string]bool, len(policy.Surfaces))
	for i, surface := range policy.Surfaces {
		label := fmt.Sprintf("living-docs.surfaces[%d]", i)
		name := strings.TrimSpace(surface.Name)
		if name == "" {
			return fmt.Errorf("docaudit: %s.name must not be empty", label)
		}
		if names[name] {
			return fmt.Errorf("docaudit: duplicate surface name %q", name)
		}
		names[name] = true
		if strings.TrimSpace(surface.Match) == "" {
			return fmt.Errorf("docaudit: %s.match must not be empty", label)
		}
		if err := validateGlob(surface.Match); err != nil {
			return fmt.Errorf("docaudit: %s.match: %w", label, err)
		}
		if strings.TrimSpace(surface.Owner) == "" {
			return fmt.Errorf("docaudit: %s.owner must not be empty", label)
		}
		if surface.MaxAgeDays <= 0 {
			return fmt.Errorf("docaudit: %s.max-age-days must be positive", label)
		}
	}
	return nil
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

func matchingSurface(name string, surfaces []Surface) (*Surface, error) {
	var matched *Surface
	for i := range surfaces {
		if !globPathMatch(surfaces[i].Match, name) {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("docaudit: tracked path %q matches both %q and %q", name, matched.Name, surfaces[i].Name)
		}
		matched = &surfaces[i]
	}
	return matched, nil
}

func globPathMatch(pattern, name string) bool {
	patternParts := strings.Split(strings.Trim(path.Clean(pattern), "/"), "/")
	nameParts := strings.Split(strings.Trim(path.Clean(name), "/"), "/")
	type state struct{ pattern, name int }
	memo := map[state]bool{}
	var match func(int, int) bool
	match = func(patternIndex, nameIndex int) bool {
		key := state{pattern: patternIndex, name: nameIndex}
		if result, ok := memo[key]; ok {
			return result
		}
		var result bool
		switch {
		case patternIndex == len(patternParts):
			result = nameIndex == len(nameParts)
		case patternParts[patternIndex] == "**":
			result = match(patternIndex+1, nameIndex) ||
				(nameIndex < len(nameParts) && match(patternIndex, nameIndex+1))
		case nameIndex < len(nameParts):
			segmentMatch, err := path.Match(patternParts[patternIndex], nameParts[nameIndex])
			if err != nil {
				return false
			}
			result = segmentMatch && match(patternIndex+1, nameIndex+1)
		}
		memo[key] = result
		return result
	}
	return match(0, 0)
}
