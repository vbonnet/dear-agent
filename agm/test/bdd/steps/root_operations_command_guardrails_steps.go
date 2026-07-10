package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/vroomprompt"
)

const rootOperationsFeaturePath = "agm/test/bdd/features/root_operations_command_guardrails.feature"

type rootOperationsPackageStateKey struct{}
type vroomWorkerRouteStateKey struct{}

type vroomWorkerRouteState struct {
	harness string
	model   string
	family  string
	rule    string
}

// RegisterRootOperationsCommandGuardrailSteps registers operations coverage and route steps.
func RegisterRootOperationsCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          rootOperationsPackageStateKey{},
		label:             "operations command package",
		featurePath:       rootOperationsFeaturePath,
		configuredPattern: `^operations command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates operations command package coverage$`,
		colocatedPattern:  `^operations command package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, vroomWorkerRouteStateKey{}, &vroomWorkerRouteState{}), nil
	})
	ctx.Step(`^VROOM worker harness "([^"]*)" uses model "([^"]*)"$`, vroomWorkerHarnessUsesModel)
	ctx.Step(`^AGM renders the VROOM worker route$`, agmRendersVROOMWorkerRoute)
	ctx.Step(`^the VROOM worker rule should preserve harness "([^"]*)" and model "([^"]*)"$`, vroomWorkerRuleShouldPreserveHarnessAndModel)
	ctx.Step(`^VROOM worker model family "([^"]*)" uses model "([^"]*)"$`, vroomWorkerModelFamilyUsesModel)
	ctx.Step(`^the VROOM worker rule should preserve model "([^"]*)" for family "([^"]*)"$`, vroomWorkerRuleShouldPreserveModelForFamily)
}

func vroomWorkerHarnessUsesModel(ctx context.Context, harness, model string) error {
	state, err := getVROOMWorkerRouteState(ctx)
	if err != nil {
		return err
	}
	state.harness, state.model = harness, model
	return nil
}

func vroomWorkerModelFamilyUsesModel(ctx context.Context, family, model string) error {
	state, err := getVROOMWorkerRouteState(ctx)
	if err != nil {
		return err
	}
	state.harness, state.family, state.model = "opencode-cli", family, model
	return nil
}

func agmRendersVROOMWorkerRoute(ctx context.Context) error {
	state, err := getVROOMWorkerRouteState(ctx)
	if err != nil {
		return err
	}
	state.rule = vroomprompt.WorkerRule(vroomprompt.Route{
		Harness: state.harness, Model: state.model, Mode: "auto", Workspace: "oss",
	})
	return nil
}

func vroomWorkerRuleShouldPreserveHarnessAndModel(ctx context.Context, harness, model string) error {
	state, err := getVROOMWorkerRouteState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.rule, "harness="+harness) || !strings.Contains(state.rule, "model="+model) {
		return fmt.Errorf("VROOM worker rule %q does not preserve harness %q and model %q", state.rule, harness, model)
	}
	return nil
}

func vroomWorkerRuleShouldPreserveModelForFamily(ctx context.Context, model, family string) error {
	state, err := getVROOMWorkerRouteState(ctx)
	if err != nil {
		return err
	}
	if state.family != family || !strings.Contains(state.rule, "model="+model) {
		return fmt.Errorf("VROOM worker rule %q does not preserve model %q for family %q", state.rule, model, family)
	}
	return nil
}

func getVROOMWorkerRouteState(ctx context.Context) (*vroomWorkerRouteState, error) {
	state, ok := ctx.Value(vroomWorkerRouteStateKey{}).(*vroomWorkerRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("VROOM worker route state not initialized")
	}
	return state, nil
}
