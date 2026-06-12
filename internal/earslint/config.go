package earslint

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// idPrefix is the optional EARS requirement ID at the start of a pattern
// regex. After markdown stripping, **FSG-01** becomes "FSG-01 " and sits
// before the EARS keyword. Including this prefix makes patterns work with
// both bare "When X shall Y" and prefixed "FSG-01 When X shall Y" forms.
const idPrefix = `(?:[A-Z][A-Z0-9-]*\d+\s+)?`

// Pattern is a single named EARS template expressed as a regular expression.
// The regex is matched against a markdown-stripped requirement line, so it
// should generally be case-insensitive (use the (?i) flag) and anchored at
// the start (^) to avoid matching mid-sentence.
type Pattern struct {
	Name  string `yaml:"name" json:"name"`
	Regex string `yaml:"regex" json:"regex"`
	// Description is human-readable documentation of the template, surfaced in
	// error help so authors learn the expected shape.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Config controls how the linter recognizes and validates requirements. It is
// intentionally data-only so it can be loaded from a file (.earslint.yml) and
// patterns can be customized per-project without recompiling.
type Config struct {
	// RequirementKeyword is the word that marks a line as a candidate
	// requirement (default "shall"). Lines without it are treated as prose.
	RequirementKeyword string `yaml:"requirement_keyword" json:"requirement_keyword"`
	// Patterns is the ordered set of accepted EARS templates. A requirement is
	// valid if it matches any one of them.
	Patterns []Pattern `yaml:"patterns" json:"patterns"`
}

// DefaultConfig returns the canonical EARS pattern set. These cover the five
// templates required by the SPEC phase gate plus the ubiquitous form, which is
// the base EARS template a plain "The <system> shall <behavior>" requirement
// uses.
func DefaultConfig() Config {
	return Config{
		RequirementKeyword: "shall",
		Patterns: []Pattern{
			{
				Name:        "event-driven",
				Regex:       `(?i)^` + idPrefix + `when\s+.+,?\s+the\s+.+\s+shall\s+.+`,
				Description: "When <trigger>, the <system> shall <response>",
			},
			{
				Name:        "state-driven",
				Regex:       `(?i)^` + idPrefix + `while\s+.+,?\s+the\s+.+\s+shall\s+.+`,
				Description: "While <state>, the <system> shall <behavior>",
			},
			{
				Name:        "feature-driven",
				Regex:       `(?i)^` + idPrefix + `where\s+.+,?\s+the\s+.+\s+shall\s+.+`,
				Description: "Where <feature>, the <system> shall <behavior>",
			},
			{
				Name:        "option",
				Regex:       `(?i)^` + idPrefix + `if\s+.+,?\s+(?:then\s+)?the\s+.+\s+shall\s+.+`,
				Description: "If <condition>, then the <system> shall <behavior>",
			},
			{
				Name:        "unwanted",
				Regex:       `(?i)^` + idPrefix + `the\s+.+\s+shall\s+not\s+.+`,
				Description: "The <system> shall not <behavior>",
			},
			{
				Name:        "ubiquitous",
				Regex:       `(?i)^` + idPrefix + `the\s+.+\s+shall\s+.+`,
				Description: "The <system> shall <behavior>",
			},
		},
	}
}

// LoadConfig reads a YAML config from path. Any fields omitted in the file
// fall back to DefaultConfig values, so a file can override just the patterns
// or just the keyword.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
	}
	def := DefaultConfig()
	if cfg.RequirementKeyword == "" {
		cfg.RequirementKeyword = def.RequirementKeyword
	}
	if len(cfg.Patterns) == 0 {
		cfg.Patterns = def.Patterns
	}
	return cfg, nil
}
