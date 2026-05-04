package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
)

// FactoryFunc constructs a provider for a given (family, model) pair.
// The router calls this on first use of each model and caches the result
// for the process lifetime. In production this is wired to
// provider.Factory.NewProvider; tests pass a fake.
type FactoryFunc func(family, model string) (provider.Provider, error)

// Options configures a Router.
type Options struct {
	// Config holds the role → model mapping. Required.
	Config *Config

	// Resolver maps model ids to (family, model). If nil, the router
	// uses provider.NewResolver().
	Resolver *provider.Resolver

	// Factory builds providers on demand. If nil, the router uses a
	// thin wrapper around provider.NewFactory().
	Factory FactoryFunc

	// CircuitBreaker is the per-model circuit breaker policy. Zero
	// values pick sensible defaults: 3 consecutive failures trips,
	// 30 s cooldown.
	CircuitBreaker provider.CircuitBreakerConfig
}

// Router routes a role to a concrete provider call, falling through the
// configured candidate chain on failure. One Router per process is the
// expected shape.
type Router struct {
	cfg      *Config
	resolver *provider.Resolver
	factory  FactoryFunc
	cbCfg    provider.CircuitBreakerConfig

	mu        sync.Mutex
	providers map[string]*entry // key: family|model
}

type entry struct {
	prov *provider.CircuitBreaker
}

// New constructs a Router from Options. Returns an error if Config is
// missing or invalid.
func New(opts Options) (*Router, error) {
	if opts.Config == nil {
		return nil, errors.New("router: Options.Config is required")
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = provider.NewResolver()
	}
	factory := opts.Factory
	if factory == nil {
		f := provider.NewFactory()
		factory = func(family, model string) (provider.Provider, error) {
			return f.NewProvider(family, model)
		}
	}
	return &Router{
		cfg:       opts.Config,
		resolver:  resolver,
		factory:   factory,
		cbCfg:     opts.CircuitBreaker,
		providers: make(map[string]*entry),
	}, nil
}

// Generate routes the request to the first candidate model whose circuit
// breaker is closed and whose call succeeds. The request's Model field
// is overridden with whatever the resolver returned for the chosen
// candidate; callers don't need to populate it.
//
// If role is empty, the configured default_role is used. If neither is
// set, returns an error.
func (r *Router) Generate(ctx context.Context, role string, req *provider.GenerateRequest) (*provider.GenerateResponse, error) {
	if req == nil {
		return nil, errors.New("router: request cannot be nil")
	}
	resolvedRole, spec, err := r.resolveRole(role)
	if err != nil {
		return nil, err
	}
	candidates := spec.Candidates()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("router: role %q has no model candidates", resolvedRole)
	}

	var attempts []string
	var lastErr error
	for _, modelID := range candidates {
		family, model, rerr := r.resolver.Resolve(modelID)
		if rerr != nil {
			lastErr = fmt.Errorf("resolve %q: %w", modelID, rerr)
			attempts = append(attempts, modelID+" (resolve error)")
			continue
		}
		prov, perr := r.providerFor(family, model)
		if perr != nil {
			lastErr = fmt.Errorf("provider %s/%s: %w", family, model, perr)
			attempts = append(attempts, modelID+" (provider error)")
			continue
		}

		// Override the request's Model with the resolved name and tag
		// the cost record's Component with the role for budget tracking.
		callReq := *req
		callReq.Model = model
		callReq.Metadata = mergeMetadata(callReq.Metadata, map[string]any{
			"router_role":  resolvedRole,
			"router_model": modelID,
		})

		resp, callErr := prov.Generate(ctx, &callReq)
		if callErr == nil {
			return resp, nil
		}

		// Don't burn through the chain on cancellation — the caller
		// asked us to stop.
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			return nil, callErr
		}

		lastErr = callErr
		attempts = append(attempts, fmt.Sprintf("%s (%s)", modelID, summariseErr(callErr)))
	}

	return nil, fmt.Errorf("router: role %q exhausted all %d candidates [%s]: %w",
		resolvedRole, len(candidates), strings.Join(attempts, ", "), lastErr)
}

// GenerateForModel bypasses role lookup and routes a literal model id
// through the resolver + circuit breaker. Used by AIExecutor when a
// workflow node specifies AINode.Model directly.
func (r *Router) GenerateForModel(ctx context.Context, modelID string, req *provider.GenerateRequest) (*provider.GenerateResponse, error) {
	if req == nil {
		return nil, errors.New("router: request cannot be nil")
	}
	if modelID == "" {
		return nil, errors.New("router: model id cannot be empty")
	}
	family, model, err := r.resolver.Resolve(modelID)
	if err != nil {
		return nil, err
	}
	prov, err := r.providerFor(family, model)
	if err != nil {
		return nil, err
	}
	callReq := *req
	callReq.Model = model
	callReq.Metadata = mergeMetadata(callReq.Metadata, map[string]any{
		"router_model": modelID,
	})
	return prov.Generate(ctx, &callReq)
}

// HasRole reports whether a role name is defined in the config.
func (r *Router) HasRole(name string) bool {
	if name == "" {
		return false
	}
	_, ok := r.cfg.Roles[name]
	return ok
}

// DefaultRole returns the configured default role, or "" if none is set.
func (r *Router) DefaultRole() string {
	return r.cfg.DefaultRole
}

func (r *Router) resolveRole(name string) (string, RoleSpec, error) {
	if name == "" {
		name = r.cfg.DefaultRole
	}
	if name == "" {
		return "", RoleSpec{}, errors.New("router: no role specified and no default_role configured")
	}
	spec, ok := r.cfg.Roles[name]
	if !ok {
		return "", RoleSpec{}, fmt.Errorf("router: unknown role %q", name)
	}
	return name, spec, nil
}

func (r *Router) providerFor(family, model string) (provider.Provider, error) {
	key := family + "|" + model
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.providers[key]; ok {
		return e.prov, nil
	}

	raw, err := r.factory(family, model)
	if err != nil {
		return nil, err
	}
	cb := provider.NewCircuitBreaker(raw, r.cbCfg)
	r.providers[key] = &entry{prov: cb}
	return cb, nil
}

// summariseErr returns a single-line representation of err truncated
// to a sane length. Used for the chain-of-attempts diagnostic in error
// messages — the full error chain is still wrapped via %w.
func summariseErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const maxLen = 120
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "…"
	}
	// Collapse newlines/tabs to keep the chain on one line.
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\t", " ")
	return msg
}

func mergeMetadata(base, extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return base
	}
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

