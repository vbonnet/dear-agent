package benchmarks

import (
	"fmt"
	"sort"
	"sync"
)

// Registry maps Suite identifiers to Benchmark implementations.
type Registry struct {
	mu     sync.RWMutex
	suites map[Suite]Benchmark
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{suites: make(map[Suite]Benchmark)}
}

// Register adds a Benchmark to the registry. It returns an error if a
// benchmark for the same Suite is already registered, so that wiring mistakes
// surface early rather than silently shadowing each other.
func (r *Registry) Register(b Benchmark) error {
	if b == nil {
		return fmt.Errorf("benchmarks: cannot register nil Benchmark")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s := b.Suite()
	if _, exists := r.suites[s]; exists {
		return fmt.Errorf("benchmarks: suite %q already registered", s)
	}
	r.suites[s] = b
	return nil
}

// Get returns the Benchmark for a Suite, or an error if none is registered.
func (r *Registry) Get(s Suite) (Benchmark, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.suites[s]
	if !ok {
		return nil, fmt.Errorf("benchmarks: no implementation registered for suite %q", s)
	}
	return b, nil
}

// List returns the registered Suite identifiers in stable order.
func (r *Registry) List() []Suite {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Suite, 0, len(r.suites))
	for s := range r.suites {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// DefaultRegistry is the package-level Registry populated by suite
// implementations in their init() functions. Callers that want isolation
// (e.g. tests) should use NewRegistry instead.
var DefaultRegistry = NewRegistry()
