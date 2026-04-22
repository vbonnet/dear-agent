package backend

import (
	"fmt"
	"os"
	"sync"
)

// BackendFactory is a function that creates a new backend instance
type BackendFactory func() (Backend, error)

// registry holds registered backend factories
var registry = struct {
	sync.RWMutex
	factories map[string]BackendFactory
}{
	factories: make(map[string]BackendFactory),
}

// Register registers a backend factory with the given name.
// Returns an error if factory is nil or if the name is already registered.
func Register(name string, factory BackendFactory) error {
	registry.Lock()
	defer registry.Unlock()

	if factory == nil {
		return fmt.Errorf("backend: Register factory is nil for backend %s", name)
	}
	if _, exists := registry.factories[name]; exists {
		return fmt.Errorf("backend: Register called twice for backend %s", name)
	}
	registry.factories[name] = factory
	return nil
}

// GetBackend returns a backend instance based on the AGM_SESSION_BACKEND environment variable
// If the environment variable is not set, it defaults to "tmux"
// Returns an error if the requested backend is not registered
func GetBackend() (Backend, error) {
	backendName := os.Getenv("AGM_SESSION_BACKEND")
	if backendName == "" {
		backendName = "tmux" // Default backend
	}

	registry.RLock()
	factory, exists := registry.factories[backendName]
	registry.RUnlock()

	if !exists {
		return nil, fmt.Errorf("backend %q is not registered (available: %v)", backendName, ListBackends())
	}

	return factory()
}

// GetBackendByName returns a backend instance for the specified backend name
// This is useful for testing or when you need to explicitly specify a backend
// Returns an error if the requested backend is not registered
func GetBackendByName(name string) (Backend, error) {
	registry.RLock()
	factory, exists := registry.factories[name]
	registry.RUnlock()

	if !exists {
		return nil, fmt.Errorf("backend %q is not registered (available: %v)", name, ListBackends())
	}

	return factory()
}

// ListBackends returns a list of all registered backend names
func ListBackends() []string {
	registry.RLock()
	defer registry.RUnlock()

	backends := make([]string, 0, len(registry.factories))
	for name := range registry.factories {
		backends = append(backends, name)
	}
	return backends
}

// IsRegistered checks if a backend with the given name is registered
func IsRegistered(name string) bool {
	registry.RLock()
	defer registry.RUnlock()

	_, exists := registry.factories[name]
	return exists
}

// unregisterAll removes all registered backends (used for testing)
func unregisterAll() {
	registry.Lock()
	defer registry.Unlock()
	registry.factories = make(map[string]BackendFactory)
}
