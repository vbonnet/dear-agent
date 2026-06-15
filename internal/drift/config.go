package drift

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseConfig decodes a YAML deploy-target config. It is strict about unknown
// fields so a typo in a target spec fails loudly rather than silently skipping
// a check the operator believed was running.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parsing drift config: %w", err)
	}
	if len(cfg.Targets) == 0 {
		return Config{}, fmt.Errorf("drift config has no targets")
	}
	for i, t := range cfg.Targets {
		if t.Name == "" {
			return Config{}, fmt.Errorf("target %d has no name", i)
		}
		if t.Deployed == "" {
			return Config{}, fmt.Errorf("target %q has no deployed path", t.Name)
		}
		if t.Source == "" {
			return Config{}, fmt.Errorf("target %q has no source path", t.Name)
		}
	}
	return cfg, nil
}
