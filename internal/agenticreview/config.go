package agenticreview

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the repository-root-relative single source of truth for
// the gate's policy. The merge loop, the required check, and every reviewer
// read this one file, so the quorum a human sees in the repository is the
// quorum that actually decides merges.
const DefaultConfigPath = ".github/agentic-review.yml"

// configFile mirrors the on-disk schema. Durations are strings so the file can
// say "45m" rather than a bare number whose unit lives only in a comment.
type configFile struct {
	Families        []string `yaml:"families"`
	Quorum          *int     `yaml:"quorum"`
	VerdictTimeout  string   `yaml:"verdict-timeout"`
	DispatchTimeout string   `yaml:"dispatch-timeout"`
}

// LoadConfig reads and validates the gate policy.
//
// Every knob is required and unknown keys are refused. A policy file is the
// kind of thing a typo silently weakens — an omitted timeout defaulting to
// zero would age every in-flight reviewer out on the spot and open the gate,
// and a misspelled "qorum" would leave the real quorum at whatever the code
// happened to default to. Both fail loudly here instead.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read agentic review config: %w", err)
	}

	var file configFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	cfg := Config{}
	for _, name := range file.Families {
		cfg.Families = append(cfg.Families, Family(name))
	}
	if file.Quorum == nil {
		return Config{}, fmt.Errorf("%s: quorum is required", path)
	}
	cfg.Quorum = *file.Quorum

	if cfg.VerdictTimeout, err = parseDuration(path, "verdict-timeout", file.VerdictTimeout); err != nil {
		return Config{}, err
	}
	if cfg.DispatchTimeout, err = parseDuration(path, "dispatch-timeout", file.DispatchTimeout); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func parseDuration(path, key, value string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("%s: %s is required", path, key)
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %s: %w", path, key, err)
	}
	return d, nil
}
