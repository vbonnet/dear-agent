package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/override"
)

const internalFoundationFeaturePath = "agm/test/bdd/features/internal_foundation_guardrails.feature"

type internalFoundationGuardrailStateKey struct{}
type overrideParityStateKey struct{}

type overrideParityState struct {
	harness     string
	family      string
	classifier  *override.FakeJudge
	judge       *override.LLMOverrideJudge
	concrete    override.JudgeVerdict
	filler      override.JudgeVerdict
	concreteErr error
	fillerErr   error
}

// RegisterInternalFoundationGuardrailSteps registers shared package coverage
// and provider-neutral override policy steps.
func RegisterInternalFoundationGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          internalFoundationGuardrailStateKey{},
		label:             "internal foundation package",
		featurePath:       internalFoundationFeaturePath,
		configuredPattern: `^internal foundation package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates internal foundation package coverage$`,
		colocatedPattern:  `^internal foundation package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, overrideParityStateKey{}, &overrideParityState{}), nil
	})

	ctx.Step(`^harness "([^"]*)" and model family "([^"]*)" use the shared override policy$`,
		func(ctx context.Context, harness, family string) error {
			state, err := getOverrideParityState(ctx)
			if err != nil {
				return err
			}
			state.harness = harness
			state.family = family
			state.classifier = &override.FakeJudge{Verdict: override.JudgeVerdict{
				Allow: true, Explanation: "specific high-risk exception",
			}}
			state.judge = override.NewLLMOverrideJudgeWith(state.classifier)
			return nil
		})

	ctx.Step(`^the high-risk override policy evaluates concrete and filler reasons$`,
		func(ctx context.Context) error {
			state, err := getOverrideParityState(ctx)
			if err != nil {
				return err
			}
			if state.judge == nil {
				return fmt.Errorf("shared override policy is not configured")
			}
			req := override.JudgeRequest{
				Tool: "agent-worker", Flag: "--force", Risk: override.RiskP0,
				Gate:   "high-risk operation for " + state.harness + "/" + state.family,
				Reason: "the protected operation is required to recover verified state",
			}
			state.concrete, state.concreteErr = state.judge.Evaluate(ctx, req)
			req.Reason = "force"
			state.filler, state.fillerErr = state.judge.Evaluate(ctx, req)
			return nil
		})

	ctx.Step(`^the override judge identity should be provider neutral$`,
		func(ctx context.Context) error {
			state, err := getOverrideParityState(ctx)
			if err != nil {
				return err
			}
			name := state.judge.Name()
			if name != "llm-override" {
				return fmt.Errorf("override judge name = %q, want llm-override", name)
			}
			lowerName := strings.ToLower(name)
			if strings.Contains(lowerName, strings.ToLower(state.harness)) ||
				strings.Contains(lowerName, strings.ToLower(state.family)) {
				return fmt.Errorf("override judge identity %q encodes route %s/%s", name, state.harness, state.family)
			}
			return nil
		})

	ctx.Step(`^the deterministic override floor should remain mandatory$`,
		func(ctx context.Context) error {
			state, err := getOverrideParityState(ctx)
			if err != nil {
				return err
			}
			if state.concreteErr != nil {
				return fmt.Errorf("concrete reason evaluation: %w", state.concreteErr)
			}
			if !state.concrete.Allow {
				return fmt.Errorf("concrete reason rejected")
			}
			if state.fillerErr != nil {
				return fmt.Errorf("filler evaluation returned error: %w", state.fillerErr)
			}
			if state.filler.Allow {
				return fmt.Errorf("deterministic floor allowed filler reason")
			}
			if len(state.classifier.Calls) != 1 {
				return fmt.Errorf("model classifier calls = %d, want 1", len(state.classifier.Calls))
			}
			return nil
		})
}

func getOverrideParityState(ctx context.Context) (*overrideParityState, error) {
	state, ok := ctx.Value(overrideParityStateKey{}).(*overrideParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("override parity state not initialized")
	}
	return state, nil
}
