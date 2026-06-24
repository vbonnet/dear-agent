package modelrouter

import (
	"fmt"
)

// Decision records a routing decision: what tier was chosen, which model it
// mapped to, and why the tier was selected.
type Decision struct {
	Tier         Tier
	Model        string // harness-specific alias (e.g. "haiku", "sonnet")
	Reason       string // human-readable: "explicit flag", "prompt heuristic: code_gen", etc.
	ExplicitTier bool   // true when the caller passed --model-tier explicitly
}

// Route selects a model for a (harness, tier, prompt) triple.
//
//   - When explicitTier is non-empty it is trusted directly (the user passed
//     --model-tier).
//   - Otherwise the prompt is classified with the heuristic and its tier is
//     used.
//   - When tierOverrideForBead is non-empty (e.g. a bead spec that forces
//     "expensive") it takes precedence over the prompt heuristic but not over
//     an explicit CLI flag.
func Route(harness, explicitTier, tierOverrideForBead, prompt string) (*Decision, error) {
	var tier Tier
	var reason string
	isExplicit := false

	switch {
	case explicitTier != "":
		t, ok := ParseTier(explicitTier)
		if !ok {
			return nil, fmt.Errorf("modelrouter: unknown tier %q (must be cheap, mid, or expensive)", explicitTier)
		}
		tier = t
		reason = "explicit --model-tier flag"
		isExplicit = true

	case tierOverrideForBead != "":
		t, ok := ParseTier(tierOverrideForBead)
		if !ok {
			return nil, fmt.Errorf("modelrouter: unknown bead tier override %q", tierOverrideForBead)
		}
		tier = t
		reason = fmt.Sprintf("bead spec override: %s", tierOverrideForBead)

	default:
		task := ClassifyPrompt(prompt)
		tier = taskTier[task]
		reason = fmt.Sprintf("prompt heuristic: %s", task)
	}

	model, ok := ModelForTier(harness, tier)
	if !ok {
		// Harness not in TierModels: fall through without a routed model.
		// Callers should use their own harness default.
		return &Decision{
			Tier:         tier,
			Model:        "",
			Reason:       reason + " (harness has no tier mapping)",
			ExplicitTier: isExplicit,
		}, nil
	}

	return &Decision{
		Tier:         tier,
		Model:        model,
		Reason:       reason,
		ExplicitTier: isExplicit,
	}, nil
}
