package manager

import (
	"fmt"
	"sync"
)

// BackendFactory creates a Backend instance.
type BackendFactory func() (Backend, error)

// Registry manages available backend implementations.
type Registry struct {
	mu       sync.RWMutex
	backends map[string]BackendFactory
}

// NewRegistry creates an empty backend registry.
func NewRegistry() *Registry {
	return &Registry{
		backends: make(map[string]BackendFactory),
	}
}

// Register adds a backend factory under the given name.
// Returns an error if the name is already registered or factory is nil.
func (r *Registry) Register(name string, factory BackendFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if factory == nil {
		return fmt.Errorf("manager: Register factory is nil for backend %s", name)
	}
	if _, exists := r.backends[name]; exists {
		return fmt.Errorf("manager: Register called twice for backend %s", name)
	}
	r.backends[name] = factory
	return nil
}

// Get creates a Backend instance by name.
func (r *Registry) Get(name string) (Backend, error) {
	r.mu.RLock()
	factory, exists := r.backends[name]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("backend %q is not registered (available: %v)", name, r.List())
	}
	return factory()
}

// List returns all registered backend names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is the global registry for backend registration.
var DefaultRegistry = NewRegistry()

// GetDefault returns a backend from the default registry by name.
// If name is empty, defaults to "tmux".
func GetDefault(name string) (Backend, error) {
	if name == "" {
		name = "tmux"
	}
	return DefaultRegistry.Get(name)
}
