package harnesseffort

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed harness-effort-defaults.yaml
var defaultsYAML []byte

// LoadDefaults loads the embedded canonical harness-effort defaults.
func LoadDefaults() (HarnessEffortConfig, error) {
	var cfg HarnessEffortConfig
	if err := yaml.Unmarshal(defaultsYAML, &cfg); err != nil {
		return HarnessEffortConfig{}, err
	}
	if cfg.ModelAliases == nil {
		cfg.ModelAliases = make(map[string]string)
	}
	if cfg.TaskTypes == nil {
		cfg.TaskTypes = make(map[string]TaskTypeConfig)
	}
	if cfg.Tiers == nil {
		cfg.Tiers = make(map[string]TierConfig)
	}
	return cfg, nil
}
