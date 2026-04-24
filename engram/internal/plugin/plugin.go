// Package plugin implements the Engram plugin system for extensibility.
//
// Plugins extend Engram's functionality through three patterns:
//
//   - Guidance: Provide structured guidance for AI agents (e.g., personas, patterns)
//   - Tool: Execute commands and operations (e.g., multi-persona-review, analyze)
//   - Connector: Integrate with external systems (e.g., GitHub, Jira)
//
// Each plugin is a directory containing:
//
//   - manifest.yaml: Plugin metadata and command definitions
//   - README.md: Documentation and usage examples
//   - bin/: Optional executable scripts
//   - engrams/: Optional pattern engrams
//
// Plugin loading and execution flow:
//  1. Loader discovers plugins in configured paths
//  2. Manifest parser reads and validates manifest.yaml
//  3. Registry stores loaded plugins and provides lookup
//  4. Executor runs plugin commands with sandboxing
//
// Example plugin structure:
//
//	plugins/my-plugin/
//	├── manifest.yaml    # Plugin definition
//	├── README.md        # Documentation
//	├── bin/
//	│   └── analyze.sh   # Tool command
//	└── engrams/
//	    └── patterns.ai.md
//
// Plugins communicate with the platform through:
//   - EventBus: Publish/subscribe events
//   - Command execution: Execute with validated permissions
//   - Engram system: Store and retrieve patterns
//
// Security is enforced through sandboxing (see internal/security).
//
// Error Logging:
//
// The plugin system uses structured logging (via Logger) with context-aware error tracking.
// All errors include relevant context such as plugin name, command, operation, and paths.
//
// Log levels:
//   - DEBUG: Detailed information for debugging (plugin discovery, command lookups)
//   - INFO: General operational messages (plugin loading, command execution)
//   - WARN: Recoverable errors (invalid plugins, missing files)
//   - ERROR: Critical errors (permission failures, execution errors)
//
// Example error logging:
//
//	logger.Error(ctx, "Failed to load plugin",
//	    WithPath(pluginPath).WithOperation("load_plugin"), err)
//
// Logs are emitted as JSON to stderr for structured consumption by monitoring tools.
package plugin

// Plugin represents a loaded plugin
type Plugin struct {
	// Plugin directory path
	Path string

	// Parsed manifest
	Manifest Manifest
}
