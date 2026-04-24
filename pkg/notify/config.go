package notify

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config defines the notification dispatch configuration.
type Config struct {
	Dispatchers []DispatcherConfig `yaml:"dispatchers"`
}

// DispatcherConfig defines a single dispatcher backend.
type DispatcherConfig struct {
	// Type is one of: log, webhook, tmux, desktop.
	Type string `yaml:"type"`
	// Enabled controls whether this dispatcher is active.
	Enabled *bool `yaml:"enabled,omitempty"`
	// Webhook-specific fields.
	URL        string        `yaml:"url,omitempty"`
	MaxRetries int           `yaml:"max_retries,omitempty"`
	RetryDelay time.Duration `yaml:"retry_delay,omitempty"`
	// Tmux-specific fields.
	Target string `yaml:"target,omitempty"`
}

// IsEnabled returns whether this dispatcher config is enabled (default true).
func (dc *DispatcherConfig) IsEnabled() bool {
	return dc.Enabled == nil || *dc.Enabled
}

// LoadConfig reads and parses a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// BuildDispatchers creates Dispatcher instances from config.
func BuildDispatchers(cfg *Config, logger *slog.Logger) ([]Dispatcher, error) {
	var dispatchers []Dispatcher
	for _, dc := range cfg.Dispatchers {
		if !dc.IsEnabled() {
			continue
		}
		d, err := buildDispatcher(dc, logger)
		if err != nil {
			return nil, fmt.Errorf("build dispatcher %q: %w", dc.Type, err)
		}
		dispatchers = append(dispatchers, d)
	}
	return dispatchers, nil
}

func buildDispatcher(dc DispatcherConfig, logger *slog.Logger) (Dispatcher, error) {
	switch dc.Type {
	case "log":
		return NewLogDispatcher(logger), nil
	case "webhook":
		if dc.URL == "" {
			return nil, fmt.Errorf("webhook dispatcher requires url")
		}
		var opts []WebhookOption
		if dc.MaxRetries > 0 {
			opts = append(opts, WithMaxRetries(dc.MaxRetries))
		}
		if dc.RetryDelay > 0 {
			opts = append(opts, WithRetryDelay(dc.RetryDelay))
		}
		return NewWebhookDispatcher(dc.URL, opts...), nil
	case "tmux":
		return NewTmuxDispatcher(dc.Target), nil
	case "desktop":
		return NewDesktopDispatcher(), nil
	default:
		return nil, fmt.Errorf("unknown dispatcher type: %s", dc.Type)
	}
}
