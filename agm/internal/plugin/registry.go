// Package plugin provides plugin functionality.
package plugin

import (
	"fmt"
	"sync"
)

// Registry manages registered task manager plugins
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]TaskManagerPlugin
}

// NewRegistry creates a new plugin registry
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]TaskManagerPlugin),
	}
}

// Register adds a plugin to the registry
// Returns error if plugin with same name already exists
func (r *Registry) Register(plugin TaskManagerPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Metadata().Name
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}

	r.plugins[name] = plugin
	return nil
}

// Get retrieves a plugin by name
// Returns nil if plugin not found
func (r *Registry) Get(name string) TaskManagerPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.plugins[name]
}

// List returns all registered plugin names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		names = append(names, name)
	}
	return names
}

// AutoDetect finds the first plugin that supports the given session
// Returns nil if no plugin supports the session
func (r *Registry) AutoDetect(sessionDir string) TaskManagerPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, plugin := range r.plugins {
		if plugin.SupportsSession(sessionDir) {
			return plugin
		}
	}
	return nil
}

// Global registry instance (initialized with built-in plugins)
var globalRegistry = NewRegistry()

// GetGlobalRegistry returns the global plugin registry
func GetGlobalRegistry() *Registry {
	return globalRegistry
}
