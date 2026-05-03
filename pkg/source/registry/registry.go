// Package registry exposes the source-adapter registry — the small
// indirection that lets binaries pick a backend by name at runtime.
// It is the seed of Phase 5.6 (plugin packaging): the registry knows
// only the adapter Name and a factory function, so additional
// backends can be added by importing their package and calling
// Register from an init().
//
// Today the registry ships with the four in-tree backends (sqlite,
// obsidian, llm-wiki, openviking-stub). The intent for 5.6 is that a
// real plugin distribution becomes:
//
//  1. Author a Go module that imports `pkg/source` and declares its
//     own `init() { registry.Register("foo", openFoo) }`.
//  2. Build a binary that imports both `pkg/source/registry` and the
//     plugin module — the init runs at link time.
//
// The registry is intentionally process-local. A genuine "load the
// plugin from a .so" story would require Go's plugin package and a
// strict ABI; we punt that until at least one third-party adapter
// asks for it. Until then, link-time registration is the integration.
package registry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// Factory constructs an adapter from a backend-specific config string.
// The shape of the string is the adapter's own contract; for sqlite it
// is a path, for obsidian a vault directory, for openviking a URL.
// Adapters that need richer configuration should accept JSON or YAML
// in the string and parse it themselves.
type Factory func(config string) (source.Adapter, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register adds (or replaces) a Factory for the named backend. Calling
// twice with the same name overwrites the previous registration —
// useful when a test wants to swap in a fixture adapter for the same
// name a binary normally uses. Concurrent callers race on the last
// write; the registry isn't a synchronization primitive.
func Register(name string, f Factory) {
	if name == "" || f == nil {
		return
	}
	mu.Lock()
	factories[name] = f
	mu.Unlock()
}

// Open returns an adapter for the named backend, configured with the
// given config string. Returns an "unknown backend" error if the name
// has not been registered — this is the signal that a binary was built
// without the requested adapter.
func Open(name, config string) (source.Adapter, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("source/registry: unknown backend %q (registered: %v)", name, Names())
	}
	return f(config)
}

// Names returns the registered backend names in alphabetical order.
// Used by Open's error message and by CLI help text.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Has reports whether the named backend is registered. Cheaper than
// calling Open + checking the error.
func Has(name string) bool {
	mu.RLock()
	_, ok := factories[name]
	mu.RUnlock()
	return ok
}

// Reset clears the registry. Test-only; not exported via godoc-friendly
// surface but available inside the module for tests that swap factories
// in and out.
func Reset() {
	mu.Lock()
	factories = make(map[string]Factory)
	mu.Unlock()
}
