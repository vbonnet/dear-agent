package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/burndownmaint"
)

const rootMaintenanceFeaturePath = "agm/test/bdd/features/root_maintenance_command_guardrails.feature"

type rootMaintenancePackageStateKey struct{}
type burndownRouteStateKey struct{}

type burndownRouteState struct {
	harness string
	model   string
	family  string
	args    []string
}

// RegisterRootMaintenanceCommandGuardrailSteps registers maintenance coverage and route steps.
func RegisterRootMaintenanceCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          rootMaintenancePackageStateKey{},
		label:             "maintenance command package",
		featurePath:       rootMaintenanceFeaturePath,
		configuredPattern: `^maintenance command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates maintenance command package coverage$`,
		colocatedPattern:  `^maintenance command package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, burndownRouteStateKey{}, &burndownRouteState{}), nil
	})
	ctx.Step(`^burndown worker harness "([^"]*)" uses model "([^"]*)"$`, burndownWorkerHarnessUsesModel)
	ctx.Step(`^AGM builds burndown worker session arguments$`, agmBuildsBurndownWorkerSessionArguments)
	ctx.Step(`^the burndown arguments should preserve harness "([^"]*)" and model "([^"]*)"$`, burndownArgumentsShouldPreserveHarnessAndModel)
	ctx.Step(`^burndown worker model family "([^"]*)" uses model "([^"]*)"$`, burndownWorkerModelFamilyUsesModel)
	ctx.Step(`^the burndown arguments should preserve model "([^"]*)" for family "([^"]*)"$`, burndownArgumentsShouldPreserveModelForFamily)
}

func burndownWorkerHarnessUsesModel(ctx context.Context, harness, model string) error {
	state, err := getBurndownRouteState(ctx)
	if err != nil {
		return err
	}
	state.harness, state.model = harness, model
	return nil
}

func burndownWorkerModelFamilyUsesModel(ctx context.Context, family, model string) error {
	state, err := getBurndownRouteState(ctx)
	if err != nil {
		return err
	}
	state.harness, state.family, state.model = "opencode-cli", family, model
	return nil
}

func agmBuildsBurndownWorkerSessionArguments(ctx context.Context) error {
	state, err := getBurndownRouteState(ctx)
	if err != nil {
		return err
	}
	state.args = burndownmaint.BuildSessionArgs("burndown-bdd", burndownmaint.Route{
		Harness: state.harness, Model: state.model, Workspace: "oss",
	})
	return nil
}

func burndownArgumentsShouldPreserveHarnessAndModel(ctx context.Context, harness, model string) error {
	state, err := getBurndownRouteState(ctx)
	if err != nil {
		return err
	}
	if maintenanceValueAfter(state.args, "--harness") != harness || maintenanceValueAfter(state.args, "--model") != model {
		return fmt.Errorf("burndown route %v does not preserve harness %q and model %q", state.args, harness, model)
	}
	return nil
}

func burndownArgumentsShouldPreserveModelForFamily(ctx context.Context, model, family string) error {
	state, err := getBurndownRouteState(ctx)
	if err != nil {
		return err
	}
	if state.family != family || maintenanceValueAfter(state.args, "--model") != model {
		return fmt.Errorf("burndown route %v does not preserve model %q for family %q", state.args, model, family)
	}
	return nil
}

func getBurndownRouteState(ctx context.Context) (*burndownRouteState, error) {
	state, ok := ctx.Value(burndownRouteStateKey{}).(*burndownRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("burndown route state not initialized")
	}
	return state, nil
}

func maintenanceValueAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
