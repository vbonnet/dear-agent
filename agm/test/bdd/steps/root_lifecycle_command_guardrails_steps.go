package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/mergeloop"
)

const rootLifecycleFeaturePath = "agm/test/bdd/features/root_lifecycle_command_guardrails.feature"

type rootLifecyclePackageStateKey struct{}
type mergeRepairRouteStateKey struct{}

type mergeRepairRouteState struct {
	harness string
	model   string
	family  string
	args    []string
}

// RegisterRootLifecycleCommandGuardrailSteps registers lifecycle coverage and route steps.
func RegisterRootLifecycleCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          rootLifecyclePackageStateKey{},
		label:             "lifecycle command package",
		featurePath:       rootLifecycleFeaturePath,
		configuredPattern: `^lifecycle command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates lifecycle command package coverage$`,
		colocatedPattern:  `^lifecycle command package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, mergeRepairRouteStateKey{}, &mergeRepairRouteState{}), nil
	})
	ctx.Step(`^merge repair harness "([^"]*)" uses model "([^"]*)"$`, mergeRepairHarnessUsesModel)
	ctx.Step(`^AGM builds merge repair session arguments$`, agmBuildsMergeRepairSessionArguments)
	ctx.Step(`^the merge repair arguments should preserve harness "([^"]*)" and model "([^"]*)"$`, mergeRepairArgumentsShouldPreserveHarnessAndModel)
	ctx.Step(`^merge repair model family "([^"]*)" uses model "([^"]*)"$`, mergeRepairModelFamilyUsesModel)
	ctx.Step(`^the merge repair arguments should preserve model "([^"]*)" for family "([^"]*)"$`, mergeRepairArgumentsShouldPreserveModelForFamily)
}

func mergeRepairHarnessUsesModel(ctx context.Context, harness, model string) error {
	state, err := getMergeRepairRouteState(ctx)
	if err != nil {
		return err
	}
	state.harness = harness
	state.model = model
	return nil
}

func mergeRepairModelFamilyUsesModel(ctx context.Context, family, model string) error {
	state, err := getMergeRepairRouteState(ctx)
	if err != nil {
		return err
	}
	state.harness = "opencode-cli"
	state.family = family
	state.model = model
	return nil
}

func agmBuildsMergeRepairSessionArguments(ctx context.Context) error {
	state, err := getMergeRepairRouteState(ctx)
	if err != nil {
		return err
	}
	state.args = mergeloop.BuildSessionNewArgs(mergeloop.AgentRequest{
		SessionName: "mergeloop/pr-42",
		Prompt:      "repair the pull request",
	}, "oss", mergeloop.SpawnRoute{Harness: state.harness, Model: state.model})
	return nil
}

func mergeRepairArgumentsShouldPreserveHarnessAndModel(ctx context.Context, harness, model string) error {
	state, err := getMergeRepairRouteState(ctx)
	if err != nil {
		return err
	}
	if valueAfter(state.args, "--harness") != harness || valueAfter(state.args, "--model") != model {
		return fmt.Errorf("merge repair route %v does not preserve harness %q and model %q", state.args, harness, model)
	}
	return nil
}

func mergeRepairArgumentsShouldPreserveModelForFamily(ctx context.Context, model, family string) error {
	state, err := getMergeRepairRouteState(ctx)
	if err != nil {
		return err
	}
	if state.family != family || valueAfter(state.args, "--model") != model {
		return fmt.Errorf("merge repair route %v does not preserve model %q for family %q", state.args, model, family)
	}
	return nil
}

func getMergeRepairRouteState(ctx context.Context) (*mergeRepairRouteState, error) {
	state, ok := ctx.Value(mergeRepairRouteStateKey{}).(*mergeRepairRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("merge repair route state not initialized")
	}
	return state, nil
}

func valueAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
