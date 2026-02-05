package bdd

import (
	"context"
	"testing"

	"github.com/cucumber/godog"

	"github.com/vbonnet/ai-tools/claude-session-manager/test/bdd/internal/testenv"
	"github.com/vbonnet/ai-tools/claude-session-manager/test/bdd/steps"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	var env *testenv.Environment
	var t *testing.T

	// Before each scenario: Setup environment
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		// Note: testing.T is not directly accessible in hooks
		// For now, we create environment without t
		// In production, we'd extract t from context or use a different approach
		env = testenv.NewEnvironment(t)
		return testenv.ContextWithEnv(ctx, env), nil
	})

	// After each scenario: Cleanup
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if env != nil {
			env.Cleanup()
		}
		return ctx, nil
	})

	// Register step definitions
	steps.RegisterSetupSteps(ctx)
	steps.RegisterSessionSteps(ctx)
	steps.RegisterConversationSteps(ctx)
	steps.RegisterAgentInterfaceSteps(ctx)
	steps.RegisterErrorHandlingSteps(ctx)
	steps.RegisterAssociationSteps(ctx)
}
