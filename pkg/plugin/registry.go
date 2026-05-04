package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/audit"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// Registry is the central composition surface. Callers Register()
// plugins (compiled-in or matched against filesystem manifests), then
// extract runtime values:
//
//   - Hooks() returns a workflow.Hooks whose four DEAR callbacks fan
//     out to every registered HookProvider, per the rules in
//     ADR-014 §D3.
//   - ApplyChecks(target) walks every registered CheckProvider and
//     calls target.Register on each returned audit.Check. Idempotency
//     and validation are delegated to audit.Registry.
//   - Plugins() returns a snapshot for inspection ("dear-agent plugins
//     list" in Phase 2).
//
// Registry is safe for concurrent use after construction. Register
// takes a write lock; the read methods use a read lock and copy the
// returned slice so callers cannot mutate registry state.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	order   []string // registration order, used for hook fan-out determinism
}

// NewRegistry returns an empty Registry. Most callers want one
// Registry per binary process; tests and host-of-many scenarios may
// construct several.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
	}
}

// Register adds p to r. Returns an error when:
//
//   - p is nil.
//   - p.Manifest() fails Validate.
//   - The manifest declares a capability the Go value does not
//     implement (e.g. CapabilityHooks but no HookProvider).
//   - A plugin with the same Manifest.Name is already registered.
//
// Registration order is preserved and used as the hook fan-out order
// (ADR-014 §D3): two HookProviders register A then B → A's OnEnforce
// runs first.
func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return errors.New("plugin: Registry.Register: nil plugin")
	}
	m := p.Manifest()
	if err := m.Validate(); err != nil {
		return fmt.Errorf("plugin: Registry.Register: %w", err)
	}
	if err := checkCapabilityCoherence(p, m); err != nil {
		return fmt.Errorf("plugin: Registry.Register: %s: %w", m.Name, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plugins[m.Name]; ok {
		return fmt.Errorf("plugin: Registry.Register: plugin %q already registered", m.Name)
	}
	r.plugins[m.Name] = p
	r.order = append(r.order, m.Name)
	return nil
}

// checkCapabilityCoherence ensures every capability the manifest
// declares has a matching Go interface, and (defensively) that every
// Go interface satisfied has a matching declared capability. The
// second direction catches the easier-to-make bug where a plugin's
// code grows a CheckProvider implementation but the manifest still
// only declares "hooks".
func checkCapabilityCoherence(p Plugin, m Manifest) error {
	declared := make(map[Capability]bool, len(m.Capabilities))
	for _, c := range m.Capabilities {
		declared[c] = true
	}
	_, hasHooks := p.(HookProvider)
	_, hasChecks := p.(CheckProvider)
	if declared[CapabilityHooks] && !hasHooks {
		return fmt.Errorf("declares %q but does not implement HookProvider", CapabilityHooks)
	}
	if declared[CapabilityChecks] && !hasChecks {
		return fmt.Errorf("declares %q but does not implement CheckProvider", CapabilityChecks)
	}
	if hasHooks && !declared[CapabilityHooks] {
		return fmt.Errorf("implements HookProvider but does not declare %q in manifest", CapabilityHooks)
	}
	if hasChecks && !declared[CapabilityChecks] {
		return fmt.Errorf("implements CheckProvider but does not declare %q in manifest", CapabilityChecks)
	}
	return nil
}

// Plugins returns a snapshot of registered plugins, sorted by name for
// stable presentation. The slice is owned by the caller. Use this for
// inspection / listing; use Hooks and ApplyChecks for runtime wiring.
func (r *Registry) Plugins() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest().Name < out[j].Manifest().Name
	})
	return out
}

// Lookup returns the plugin registered under name, or (nil, false).
func (r *Registry) Lookup(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// hookProvidersInOrder snapshots the HookProviders in registration
// order. Held briefly under the read lock; the returned slice is
// safe to use without the lock because *Plugin values are immutable
// post-Register (we never mutate a Plugin after storing it).
func (r *Registry) hookProvidersInOrder() []HookProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HookProvider, 0, len(r.order))
	for _, name := range r.order {
		if hp, ok := r.plugins[name].(HookProvider); ok {
			out = append(out, hp)
		}
	}
	return out
}

// Hooks returns a workflow.Hooks whose four DEAR callbacks fan out to
// every registered HookProvider in registration order. If no providers
// implement a given phase (e.g. nobody set OnDefine), the corresponding
// field on the returned Hooks is nil and the runner skips it — same
// shape as a hand-built Hooks value.
//
// Composition rules (ADR-014 §D3):
//
//   - OnDefine, OnEnforce: short-circuit on first non-nil error. Later
//     providers do not run.
//   - OnAudit, OnResolve: every provider runs; errors are collected
//     with errors.Join. The substrate guarantee is that audit emission
//     is unconditional, so we cannot let one provider's failure block
//     the others.
//
// The function captures the snapshot of providers at the time Hooks()
// is called; subsequent Register calls do not retroactively appear in
// an already-extracted Hooks. Callers that need late-bound composition
// should call Hooks() after their final Register.
func (r *Registry) Hooks() workflow.Hooks {
	providers := r.hookProvidersInOrder()
	if len(providers) == 0 {
		return workflow.Hooks{}
	}

	// Pre-compute which providers implement each phase. Keeping the
	// lists separate lets us return a Hooks whose unused fields stay
	// nil — same observable shape the runner sees from a hand-built
	// Hooks that left a phase unset.
	var (
		defineProviders  []func(context.Context, workflow.DefinePayload) error
		enforceProviders []func(context.Context, workflow.EnforcePayload) error
		auditProviders   []func(context.Context, workflow.AuditPayload) error
		resolveProviders []func(context.Context, workflow.ResolvePayload) error
		defineNames      []string
		enforceNames     []string
		auditNames       []string
		resolveNames     []string
	)
	for _, hp := range providers {
		h := hp.Hooks()
		name := hp.Manifest().Name
		if h.OnDefine != nil {
			defineProviders = append(defineProviders, h.OnDefine)
			defineNames = append(defineNames, name)
		}
		if h.OnEnforce != nil {
			enforceProviders = append(enforceProviders, h.OnEnforce)
			enforceNames = append(enforceNames, name)
		}
		if h.OnAudit != nil {
			auditProviders = append(auditProviders, h.OnAudit)
			auditNames = append(auditNames, name)
		}
		if h.OnResolve != nil {
			resolveProviders = append(resolveProviders, h.OnResolve)
			resolveNames = append(resolveNames, name)
		}
	}

	var out workflow.Hooks
	if len(defineProviders) > 0 {
		out.OnDefine = composeShortCircuit(defineProviders, defineNames, "OnDefine")
	}
	if len(enforceProviders) > 0 {
		out.OnEnforce = composeShortCircuit(enforceProviders, enforceNames, "OnEnforce")
	}
	if len(auditProviders) > 0 {
		out.OnAudit = composeAccumulate(auditProviders, auditNames, "OnAudit")
	}
	if len(resolveProviders) > 0 {
		out.OnResolve = composeAccumulate(resolveProviders, resolveNames, "OnResolve")
	}
	return out
}

// composeShortCircuit returns a callback that runs every provider in
// order and returns the first non-nil error (wrapped with the failing
// plugin's name). Used for OnDefine and OnEnforce, where an error has
// to fail the run / fail the node before later phases observe stale
// state.
func composeShortCircuit[P any](
	providers []func(context.Context, P) error,
	names []string,
	phase string,
) func(context.Context, P) error {
	// Defensive copies — protects against callers mutating the slice
	// via the captured backing array. Cheap enough at registry build time.
	provCopy := make([]func(context.Context, P) error, len(providers))
	copy(provCopy, providers)
	nameCopy := make([]string, len(names))
	copy(nameCopy, names)
	return func(ctx context.Context, p P) error {
		for i, fn := range provCopy {
			if err := fn(ctx, p); err != nil {
				return fmt.Errorf("plugin %q %s: %w", nameCopy[i], phase, err)
			}
		}
		return nil
	}
}

// composeAccumulate returns a callback that runs every provider
// unconditionally and joins their errors via errors.Join. Used for
// OnAudit and OnResolve, where the substrate guarantee (audit emission
// is unconditional, ADR-010 §D3) and the already-failing-run shape
// (resolve runs after a terminal failure) make short-circuiting
// strictly worse than continuing.
func composeAccumulate[P any](
	providers []func(context.Context, P) error,
	names []string,
	phase string,
) func(context.Context, P) error {
	provCopy := make([]func(context.Context, P) error, len(providers))
	copy(provCopy, providers)
	nameCopy := make([]string, len(names))
	copy(nameCopy, names)
	return func(ctx context.Context, p P) error {
		var errs []error
		for i, fn := range provCopy {
			if err := fn(ctx, p); err != nil {
				errs = append(errs, fmt.Errorf("plugin %q %s: %w", nameCopy[i], phase, err))
			}
		}
		return errors.Join(errs...)
	}
}

// ApplyChecks walks every registered CheckProvider and registers each
// returned audit.Check into target via target.Register. Validation,
// idempotency, and metadata coherence are the audit registry's job;
// ApplyChecks just threads the originating plugin's name through any
// returned error so operators can tell which plugin's check collided
// with which.
//
// Returns the first error encountered. The audit registry already
// rejects duplicate IDs, so a second plugin registering the same
// check ID as a first will fail this call. We do not auto-rollback
// previously-registered checks — operators tend to want to know "the
// first collision happened between A and B", and a partial-register
// state is the same kind of partial state the existing audit registry
// already produces if a caller registers two checks one-at-a-time.
func (r *Registry) ApplyChecks(target *audit.Registry) error {
	if target == nil {
		return errors.New("plugin: Registry.ApplyChecks: nil target audit.Registry")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, name := range r.order {
		cp, ok := r.plugins[name].(CheckProvider)
		if !ok {
			continue
		}
		for _, c := range cp.Checks() {
			if err := target.Register(c); err != nil {
				return fmt.Errorf("plugin: ApplyChecks: plugin %q: %w", name, err)
			}
		}
	}
	return nil
}
