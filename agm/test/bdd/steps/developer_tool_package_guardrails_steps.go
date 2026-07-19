package steps

import (
	"context"
	"fmt"
	"slices"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

const developerToolFeaturePath = "agm/test/bdd/features/developer_tool_package_guardrails.feature"

type developerToolPackageStateKey struct{}
type developerToolParityStateKey struct{}

type developerToolParityState struct {
	harness string
	family  string
}

// RegisterDeveloperToolPackageGuardrailSteps registers developer-tool coverage and parity steps.
func RegisterDeveloperToolPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          developerToolPackageStateKey{},
		label:             "developer tool package",
		featurePath:       developerToolFeaturePath,
		configuredPattern: `^developer tool package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates developer tool package coverage$`,
		colocatedPattern:  `^developer tool package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, developerToolParityStateKey{}, &developerToolParityState{}), nil
	})
	ctx.Step(`^developer tooling is invoked by harness "([^"]*)" with model family "([^"]*)"$`, developerToolingIsInvoked)
	ctx.Step(`^AGM validates the developer tooling parity route$`, agmValidatesDeveloperToolingParityRoute)
	ctx.Step(`^the developer tooling contract should remain provider neutral$`, developerToolingContractRemainsProviderNeutral)
}

func developerToolingIsInvoked(ctx context.Context, harness, family string) error {
	state, err := getDeveloperToolParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = harness
	state.family = family
	return nil
}

func agmValidatesDeveloperToolingParityRoute(ctx context.Context) error {
	state, err := getDeveloperToolParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(agent.ActiveHarnesses(), state.harness) {
		return fmt.Errorf("developer tooling harness %q is not active", state.harness)
	}
	if !agent.IsSupportedModelFamily(state.family) {
		return fmt.Errorf("developer tooling model family %q is not supported", state.family)
	}
	return nil
}

func developerToolingContractRemainsProviderNeutral(ctx context.Context) error {
	state, err := getDeveloperToolParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("developer tooling parity route is incomplete")
	}
	return nil
}

func getDeveloperToolParityState(ctx context.Context) (*developerToolParityState, error) {
	state, ok := ctx.Value(developerToolParityStateKey{}).(*developerToolParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("developer tooling parity state not initialized")
	}
	return state, nil
}
