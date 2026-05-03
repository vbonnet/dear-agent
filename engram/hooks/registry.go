package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

var (
	// ErrHookNotFound is returned when a hook is not found in the registry
	ErrHookNotFound = errors.New("hook not found")
	// ErrInvalidHook is returned when a hook fails validation
	ErrInvalidHook = errors.New("invalid hook")
	// ErrDuplicateHook is returned when attempting to register a hook with an existing name
	ErrDuplicateHook = errors.New("duplicate hook name")
)

// Registry manages verification hooks
type Registry interface {
	// Register adds a hook to the registry
	Register(hook Hook) error

	// Unregister removes a hook from the registry
	Unregister(name string) error

	// GetHooksByEvent returns hooks for specific event type, sorted by priority (descending)
	GetHooksByEvent(event HookEvent) []Hook

	// Load reads hooks from ~/.engram/hooks.toml
	Load() error

	// Save writes hooks to ~/.engram/hooks.toml
	Save() error

	// GetHook returns a specific hook by name
	GetHook(name string) (*Hook, error)

	// ListAll returns all registered hooks
	ListAll() []Hook
}

// registryImpl is the thread-safe implementation of Registry
type registryImpl struct {
	mu    sync.RWMutex
	hooks map[string]Hook // Keyed by hook name
	path  string          // Path to hooks.toml file
}

// hookFile represents the TOML file structure
type hookFile struct {
	Hooks []Hook `toml:"hooks"`
}

// NewRegistry creates a new hook registry with the default path
func NewRegistry() Registry {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir unavailable
		homeDir = "."
	}
	path := filepath.Join(homeDir, ".engram", "hooks.toml")
	return NewRegistryWithPath(path)
}

// NewRegistryWithPath creates a new hook registry with a custom path
func NewRegistryWithPath(path string) Registry {
	return &registryImpl{
		hooks: make(map[string]Hook),
		path:  path,
	}
}

// Register adds a hook to the registry
func (r *registryImpl) Register(hook Hook) error {
	if err := validateHook(hook); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidHook, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate
	if _, exists := r.hooks[hook.Name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateHook, hook.Name)
	}

	r.hooks[hook.Name] = hook
	return nil
}

// Unregister removes a hook from the registry
func (r *registryImpl) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.hooks[name]; !exists {
		return fmt.Errorf("%w: %s", ErrHookNotFound, name)
	}

	delete(r.hooks, name)
	return nil
}

// GetHooksByEvent returns hooks for specific event type, sorted by priority (descending)
func (r *registryImpl) GetHooksByEvent(event HookEvent) []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Hook
	for _, hook := range r.hooks {
		if hook.Event == event {
			result = append(result, hook)
		}
	}

	// Sort by priority (higher first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// GetHook returns a specific hook by name
func (r *registryImpl) GetHook(name string) (*Hook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hook, exists := r.hooks[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrHookNotFound, name)
	}

	return &hook, nil
}

// ListAll returns all registered hooks
func (r *registryImpl) ListAll() []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Hook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		result = append(result, hook)
	}

	// Sort by event, then priority
	sort.Slice(result, func(i, j int) bool {
		if result[i].Event != result[j].Event {
			return result[i].Event < result[j].Event
		}
		return result[i].Priority > result[j].Priority
	})

	return result
}

// Load reads hooks from the TOML file
func (r *registryImpl) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(r.path); os.IsNotExist(err) {
		// File doesn't exist, start with empty registry
		r.hooks = make(map[string]Hook)
		return nil
	}

	// Read file
	data, err := os.ReadFile(r.path)
	if err != nil {
		return fmt.Errorf("failed to read hooks file: %w", err)
	}

	// Parse TOML
	var hf hookFile
	if err := toml.Unmarshal(data, &hf); err != nil {
		return fmt.Errorf("failed to parse hooks file: %w", err)
	}

	// Validate and load hooks
	newHooks := make(map[string]Hook)
	for _, hook := range hf.Hooks {
		if err := validateHook(hook); err != nil {
			return fmt.Errorf("invalid hook %s: %w", hook.Name, err)
		}
		if _, exists := newHooks[hook.Name]; exists {
			return fmt.Errorf("duplicate hook name: %s", hook.Name)
		}
		newHooks[hook.Name] = hook
	}

	r.hooks = newHooks
	return nil
}

// Save writes hooks to the TOML file atomically
func (r *registryImpl) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Convert map to slice
	hooks := make([]Hook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		hooks = append(hooks, hook)
	}

	// Sort for consistent output
	sort.Slice(hooks, func(i, j int) bool {
		if hooks[i].Event != hooks[j].Event {
			return hooks[i].Event < hooks[j].Event
		}
		return hooks[i].Priority > hooks[j].Priority
	})

	hf := hookFile{Hooks: hooks}

	// Marshal to TOML
	data, err := toml.Marshal(hf)
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tempPath := r.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, r.path); err != nil {
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// validateHook validates hook configuration
func validateHook(hook Hook) error {
	// Name validation
	if hook.Name == "" {
		return errors.New("hook name cannot be empty")
	}

	// Event validation
	if hook.Event != HookEventSessionCompletion &&
		hook.Event != HookEventPhaseCompletion &&
		hook.Event != HookEventPreCommit {
		return fmt.Errorf("invalid event: %s", hook.Event)
	}

	// Priority validation
	if hook.Priority < 1 || hook.Priority > 100 {
		return fmt.Errorf("priority must be between 1 and 100, got %d", hook.Priority)
	}

	// Type validation
	if hook.Type != HookTypeBinary &&
		hook.Type != HookTypeSkill &&
		hook.Type != HookTypeScript {
		return fmt.Errorf("invalid type: %s", hook.Type)
	}

	// Command validation
	if hook.Command == "" {
		return errors.New("command cannot be empty")
	}

	// Timeout validation
	if hook.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative, got %d", hook.Timeout)
	}
	if hook.Timeout == 0 {
		hook.Timeout = 60 // Default timeout
	}
	if hook.Timeout > 600 {
		return fmt.Errorf("timeout cannot exceed 600 seconds, got %d", hook.Timeout)
	}

	return nil
}
