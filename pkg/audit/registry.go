package audit

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the set of Checks (and Refiners) the runner can
// invoke. The package ships a process-global Default registry that
// built-in checks register into via init(); custom callers can build
// their own with NewRegistry for isolation in tests or alternate
// universes (e.g. a CLI that exposes only a curated subset).
//
// Registration is idempotent at the check level: calling Register
// with a check whose ID already exists returns an error rather than
// silently overwriting. This avoids the common subtle-bug pattern
// where two init()s race on registration order.
type Registry struct {
	mu       sync.RWMutex
	checks   map[string]Check
	refiners map[string]Refiner
}

// NewRegistry returns an empty registry. Callers wire in their own
// checks via Register; tests use this to avoid touching the global
// Default.
func NewRegistry() *Registry {
	return &Registry{
		checks:   make(map[string]Check),
		refiners: make(map[string]Refiner),
	}
}

// Default is the process-global registry. Built-in checks register
// here from their package init(). The audit runner uses Default when
// no explicit *Registry is wired.
var Default = NewRegistry()

// Register adds a Check to r. Returns an error when c.Meta() is
// invalid or when a Check with the same ID is already registered.
// The error path is the recommended one: panics in init() are
// hostile to consumers that compose registries.
func (r *Registry) Register(c Check) error {
	if c == nil {
		return fmt.Errorf("audit: Registry.Register: nil check")
	}
	meta := c.Meta()
	if err := meta.Validate(); err != nil {
		return fmt.Errorf("audit: Registry.Register: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.checks[meta.ID]; ok {
		return fmt.Errorf("audit: Registry.Register: check %q already registered", meta.ID)
	}
	r.checks[meta.ID] = c
	return nil
}

// MustRegister is the panic-on-error variant for use in init() of
// built-in check packages where a duplicate is a programming error.
// User code should prefer Register.
func (r *Registry) MustRegister(c Check) {
	if err := r.Register(c); err != nil {
		panic(err)
	}
}

// RegisterRefiner adds a Refiner to r. Same idempotency rule as
// Register.
func (r *Registry) RegisterRefiner(rf Refiner) error {
	if rf == nil {
		return fmt.Errorf("audit: Registry.RegisterRefiner: nil refiner")
	}
	name := rf.Name()
	if name == "" {
		return fmt.Errorf("audit: Registry.RegisterRefiner: empty refiner name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.refiners[name]; ok {
		return fmt.Errorf("audit: Registry.RegisterRefiner: refiner %q already registered", name)
	}
	r.refiners[name] = rf
	return nil
}

// Lookup returns the Check registered under id, or (nil, false) if
// none. Safe for concurrent use.
func (r *Registry) Lookup(id string) (Check, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.checks[id]
	return c, ok
}

// Checks returns a snapshot of all registered checks, sorted by ID.
// The returned slice is owned by the caller — modifying it does not
// affect the registry.
func (r *Registry) Checks() []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Check, 0, len(r.checks))
	for _, c := range r.checks {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
}

// Refiners returns a snapshot of all registered refiners, sorted by
// Name.
func (r *Registry) Refiners() []Refiner {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Refiner, 0, len(r.refiners))
	for _, rf := range r.refiners {
		out = append(out, rf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ChecksForCadence returns a snapshot of registered checks whose
// recommended cadence equals c, sorted by ID. Operators that want
// "the daily defaults for this repo" call this with CadenceDaily.
func (r *Registry) ChecksForCadence(c Cadence) []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Check, 0, len(r.checks))
	for _, ch := range r.checks {
		if ch.Meta().Cadence == c {
			out = append(out, ch)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
}
