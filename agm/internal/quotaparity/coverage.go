// Package quotaparity defines executable coverage for quota and cost monitoring
// surfaces across supported harnesses and model families.
package quotaparity

import (
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/internal/pricing"
)

// HarnessSurface describes the quota monitoring data surfaces available for an
// active harness. "Unavailable" surfaces are explicit so status and cost code
// can degrade honestly instead of silently using Claude-specific assumptions.
type HarnessSurface struct {
	Harness         string
	ContextSource   string
	CostSource      string
	RateLimitSource string
	Persistence     string
	Degradation     string
}

// ModelFamilyCoverage describes quota accounting for a supported model family.
type ModelFamilyCoverage struct {
	Family       string
	DefaultModel string
	PricePolicy  string
	Priced       bool
	PriceSource  string
	PriceAsOf    string
}

// ActiveHarnessSurfaces returns quota monitoring surfaces for each active
// harness.
func ActiveHarnessSurfaces() []HarnessSurface {
	active := agent.ActiveHarnesses()
	out := make([]HarnessSurface, 0, len(active))
	for _, harness := range active {
		surface, ok := SurfaceForHarness(harness)
		if ok {
			out = append(out, surface)
		}
	}
	return out
}

// SurfaceForHarness returns the quota monitoring surface for a harness.
func SurfaceForHarness(harness string) (HarnessSurface, bool) {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return HarnessSurface{
			Harness:         "claude-code",
			ContextSource:   "statusline context_window, manifest context_usage, conversation JSONL",
			CostSource:      "statusline total_cost_usd, manifest last_known_cost, token estimate",
			RateLimitSource: "statusline rate_limits.five_hour",
			Persistence:     "manifest context_usage, last_known_cost, last_known_model",
			Degradation:     "manifest and token-estimate fallback when statusline is missing or stale",
		}, true
	case "codex-cli":
		return HarnessSurface{
			Harness:         "codex-cli",
			ContextSource:   "manifest context_usage and cost_tracking",
			CostSource:      "manifest cost_tracking priced by shared model table",
			RateLimitSource: "explicitly unavailable from Codex CLI",
			Persistence:     "manifest context_usage and cost_tracking",
			Degradation:     "unknown context and unpriced cost are displayed as unavailable, not Opus defaults",
		}, true
	case "agy":
		return HarnessSurface{
			Harness:         "agy",
			ContextSource:   "manifest context_usage and AGY conversation metadata",
			CostSource:      "manifest cost_tracking priced by shared model table",
			RateLimitSource: "explicitly unavailable from AGY CLI",
			Persistence:     "manifest context_usage, cost_tracking, and agy conversation metadata",
			Degradation:     "unknown context and unpriced cost are displayed as unavailable, not Claude defaults",
		}, true
	case "opencode-cli":
		return HarnessSurface{
			Harness:         "opencode-cli",
			ContextSource:   "OpenCode monitor events and manifest context_usage",
			CostSource:      "manifest cost_tracking priced by shared model table",
			RateLimitSource: "explicitly unavailable from OpenCode server",
			Persistence:     "manifest context_usage, cost_tracking, and opencode metadata",
			Degradation:     "SSE failures fall back to tmux/manifest monitoring",
		}, true
	case "pi-cli":
		return HarnessSurface{
			Harness:         "pi-cli",
			ContextSource:   "Pi native JSONL message usage and manifest context_usage",
			CostSource:      "Pi native JSONL message usage and manifest cost_tracking",
			RateLimitSource: "explicitly unavailable from Pi",
			Persistence:     "Pi native transcript plus manifest context_usage and cost_tracking",
			Degradation:     "missing native usage is displayed as unavailable, not a Claude-specific default",
		}, true
	default:
		return HarnessSurface{}, false
	}
}

// ValidateActiveHarnessSurfaces verifies all active harnesses have explicit
// quota monitoring coverage.
func ValidateActiveHarnessSurfaces() error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no quota monitoring surface", harness)
		}
		if surface.ContextSource == "" || surface.CostSource == "" || surface.RateLimitSource == "" ||
			surface.Persistence == "" || surface.Degradation == "" {
			return fmt.Errorf("active harness %q has incomplete quota monitoring surface: %+v", harness, surface)
		}
	}
	return nil
}

// ModelFamilyCoverageFor returns quota accounting coverage for a supported
// model family.
func ModelFamilyCoverageFor(family string) (ModelFamilyCoverage, bool) {
	family = strings.ToLower(family)
	model, ok := agent.DefaultModelForFamily(family)
	if !ok {
		return ModelFamilyCoverage{}, false
	}
	price := pricing.Lookup(model.FullName)
	if price == pricing.UnknownModel {
		price = pricing.Lookup(model.Alias)
	}
	priced := price != pricing.UnknownModel
	policy := "explicitly-unpriced"
	if priced {
		policy = "priced"
	}
	return ModelFamilyCoverage{
		Family:       family,
		DefaultModel: model.FullName,
		PricePolicy:  policy,
		Priced:       priced,
		PriceSource:  price.Source,
		PriceAsOf:    price.AsOf,
	}, true
}

// ValidateModelFamilyCoverage verifies every supported model family has either
// shared pricing coverage or an explicit unpriced policy.
func ValidateModelFamilyCoverage() error {
	for _, family := range agent.ModelFamilyNames() {
		coverage, ok := ModelFamilyCoverageFor(family)
		if !ok {
			return fmt.Errorf("model family %q has no quota coverage", family)
		}
		if coverage.DefaultModel == "" || coverage.PricePolicy == "" {
			return fmt.Errorf("model family %q has incomplete quota coverage: %+v", family, coverage)
		}
	}
	return nil
}
