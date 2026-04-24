package steps

import (
	"context"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/testenv"
)

// RegisterSetupSteps registers setup-related step definitions
func RegisterSetupSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^I have AGM installed$`, iHaveAGMInstalled)
	ctx.Step(`^I have a mock ([^ ]+) adapter configured$`, iHaveAMockAdapterConfigured)
}

func iHaveAGMInstalled(ctx context.Context) (context.Context, error) {
	// Environment already created in BeforeScenario hook
	// This step is mostly for readability in Gherkin
	return ctx, nil
}

func iHaveAMockAdapterConfigured(ctx context.Context, agent string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	// Get and set current adapter
	adapter, err := env.GetAdapter(agent)
	if err != nil {
		return ctx, err
	}

	env.CurrentAdapter = adapter
	return testenv.ContextWithEnv(ctx, env), nil
}
