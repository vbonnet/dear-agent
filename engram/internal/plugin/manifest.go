package plugin

// Manifest represents a plugin's manifest file (plugin.yaml)
type Manifest struct {
	// Plugin name
	Name string `yaml:"name"`

	// Version
	Version string `yaml:"version"`

	// Description
	Description string `yaml:"description"`

	// Plugin pattern (guidance, tool, connector)
	Pattern string `yaml:"pattern"`

	// Permissions required
	Permissions Permissions `yaml:"permissions,omitempty"`

	// Commands (for tool/connector plugins)
	Commands []Command `yaml:"commands,omitempty"`

	// Engrams directory (for guidance plugins)
	EngramsDir string `yaml:"engrams_dir,omitempty"`

	// EventBus subscriptions
	EventBus EventBusConfig `yaml:"eventbus,omitempty"`

	// Integrity verification (Phase 1.3)
	Integrity Integrity `yaml:"integrity,omitempty"`
}

// Integrity defines file integrity verification
type Integrity struct {
	// Hash algorithm (e.g., "sha256")
	Algorithm string `yaml:"algorithm"`

	// File hashes (relative path -> hash)
	// Example: {"plugin.yaml": "abc123...", "bin/tool": "def456..."}
	Files map[string]string `yaml:"files"`
}

// Permissions defines what a plugin can access
type Permissions struct {
	// Filesystem access
	Filesystem []string `yaml:"filesystem,omitempty"`

	// Network access
	Network []string `yaml:"network,omitempty"`

	// Environment variables
	Environment []string `yaml:"environment,omitempty"`

	// External commands
	Commands []string `yaml:"commands,omitempty"`
}

// Command defines a plugin command
type Command struct {
	// Command name
	Name string `yaml:"name"`

	// Description
	Description string `yaml:"description"`

	// Executable path (relative to plugin directory)
	Executable string `yaml:"executable"`

	// Arguments
	Args []string `yaml:"args,omitempty"`
}

// EventBusConfig defines EventBus subscriptions
type EventBusConfig struct {
	// Events to subscribe to
	Subscribe []string `yaml:"subscribe,omitempty"`

	// Events to publish
	Publish []string `yaml:"publish,omitempty"`
}

// PluginPattern constants
const (
	PatternGuidance  = "guidance"
	PatternTool      = "tool"
	PatternConnector = "connector"
)
