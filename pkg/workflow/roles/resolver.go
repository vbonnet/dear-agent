package roles

import (
	"errors"
	"fmt"
)

// Resolver maps a role name (and an optional set of node-level filters)
// to a concrete model id. It is the choke-point where Phase 1 commits to
// "AI nodes declare a role, the resolver picks a model".
//
// The resolver is stateless beyond its Registry and Overrides — every
// call is O(tiers) ≤ 3. Concurrent use is safe.
type Resolver struct {
	// Registry is the parsed roles.yaml. nil falls back to BuiltinRegistry.
	Registry *Registry

	// Overrides is the highest-precedence layer (CLI flags). Keyed by
	// role name; the value is the model id that wins regardless of
	// tier. The runner emits a deprecation warning whenever an override
	// is consulted so operators can see them in `workflow lint`.
	Overrides map[string]string

	// Capacity is a pluggable filter — returns true if the named model
	// has remaining capacity (rate limit, quota, …). nil means "always
	// available". Used to short-circuit a tier whose primary is
	// rate-limited at the moment.
	Capacity CapacityChecker
}

// CapacityChecker is the optional interface a runner can plug in to
// reject a tier whose model is currently saturated. The default (nil)
// is "always available".
type CapacityChecker interface {
	HasCapacity(model string) bool
}

// Request describes the per-call resolution input. Matches the
// "Resolution algorithm" pseudocode in ROADMAP.md.
type Request struct {
	// Role is the role name from the AI node. Empty falls back to Model.
	Role string

	// Model is the legacy back-compat field. When set and Role is
	// empty, Resolve returns it verbatim with TierName="model" — used
	// by AI nodes that haven't migrated to roles yet.
	Model string

	// ModelOverride is the per-node escape hatch. When set, Resolve
	// returns it verbatim with TierName="override" so the audit log can
	// see the bypass.
	ModelOverride string

	// RequiredCapabilities filters the tier set: a tier whose
	// capabilities don't superset this list is skipped.
	RequiredCapabilities []string

	// MaxDollars, when > 0, rejects tiers whose minimum input cost (per
	// million input tokens) exceeds the cap. The check is intentionally
	// crude — proper accounting needs the AIExecutor wrapper, but this
	// catches "you asked for $0.10 but the cheapest tier costs $15/MTok".
	MaxDollars float64
}

// Resolved is the answer the resolver returns: the resolved model id,
// the tier name that produced it, and a reference to the Tier struct
// itself so the caller can pass effort, max_context, etc. through to
// the AI executor.
type Resolved struct {
	Model     string
	TierName  string // "primary"|"secondary"|"tertiary"|"override"|"model"
	RoleName  string
	Effort    string
	Capabilities []string
}

// ErrNoModelAvailable is returned when no tier passes the configured
// filters. Wrapped with %w so callers can errors.Is on it.
var ErrNoModelAvailable = errors.New("roles: no model available")

// Resolve runs the algorithm:
//
//   1. ModelOverride wins if set.
//   2. Role lookup; for each tier in primary, secondary, tertiary:
//      - capability filter
//      - capacity filter
//      - cost filter (max_dollars vs cost_per_mtok.input)
//      - first match wins
//   3. Falls back to Model verbatim if Role is empty.
//   4. Returns ErrNoModelAvailable if nothing matches.
//
// Cost is O(tiers); tiers ≤ 3.
//
//nolint:gocyclo // straight-line filter chain; splitting hurts readability
func (r *Resolver) Resolve(req Request) (Resolved, error) {
	if req.ModelOverride != "" {
		return Resolved{
			Model:    req.ModelOverride,
			TierName: "override",
			RoleName: req.Role,
		}, nil
	}
	if req.Role == "" {
		if req.Model == "" {
			return Resolved{}, fmt.Errorf("%w: neither role nor model set", ErrNoModelAvailable)
		}
		return Resolved{Model: req.Model, TierName: "model"}, nil
	}

	reg := r.Registry
	if reg == nil {
		reg = BuiltinRegistry()
	}
	if name, ok := r.Overrides[req.Role]; ok && name != "" {
		return Resolved{
			Model:    name,
			TierName: "override",
			RoleName: req.Role,
		}, nil
	}

	role, ok := reg.Lookup(req.Role)
	if !ok {
		return Resolved{}, fmt.Errorf("%w: unknown role %q", ErrNoModelAvailable, req.Role)
	}

	for _, nt := range orderedTiers(role) {
		if !satisfiesCapabilities(role.Capabilities, nt.tier.Capabilities, req.RequiredCapabilities) {
			continue
		}
		if r.Capacity != nil && !r.Capacity.HasCapacity(nt.tier.Model) {
			continue
		}
		if !withinBudget(req.MaxDollars, nt.tier) {
			continue
		}
		effort := nt.tier.Effort
		if effort == "" {
			effort = reg.Defaults.Effort
		}
		return Resolved{
			Model:    nt.tier.Model,
			TierName: nt.name,
			RoleName: req.Role,
			Effort:   effort,
			Capabilities: mergeCapabilities(role.Capabilities, nt.tier.Capabilities),
		}, nil
	}

	return Resolved{}, fmt.Errorf("%w: role %q has no tier passing filters", ErrNoModelAvailable, req.Role)
}

// satisfiesCapabilities returns true if the union of role+tier
// capabilities covers every capability in required. Order does not
// matter; duplicates are fine.
func satisfiesCapabilities(roleCaps, tierCaps, required []string) bool {
	if len(required) == 0 {
		return true
	}
	have := make(map[string]struct{}, len(roleCaps)+len(tierCaps))
	for _, c := range roleCaps {
		have[c] = struct{}{}
	}
	for _, c := range tierCaps {
		have[c] = struct{}{}
	}
	for _, c := range required {
		if _, ok := have[c]; !ok {
			return false
		}
	}
	return true
}

// withinBudget returns true if maxDollars is unset (≤0) or the tier's
// declared input-cost-per-MTok is at or below the cap. The check is a
// floor (input cost only); the budget meter does the precise accounting
// during execution.
func withinBudget(maxDollars float64, tier *Tier) bool {
	if maxDollars <= 0 {
		return true
	}
	if tier.CostPerMTok.Input <= 0 {
		return true // no cost data → trust the operator's roles.yaml
	}
	return tier.CostPerMTok.Input <= maxDollars
}

// mergeCapabilities concatenates role + tier capabilities for the
// Resolved struct. The audit log uses this to record "what the node
// was promised" so a future capability-mismatch bug is debuggable.
func mergeCapabilities(roleCaps, tierCaps []string) []string {
	if len(roleCaps) == 0 {
		return append([]string(nil), tierCaps...)
	}
	if len(tierCaps) == 0 {
		return append([]string(nil), roleCaps...)
	}
	out := make([]string, 0, len(roleCaps)+len(tierCaps))
	out = append(out, roleCaps...)
	out = append(out, tierCaps...)
	return out
}
