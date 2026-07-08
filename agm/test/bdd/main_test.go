package bdd

import (
	"testing"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/test/bdd/steps"
)

// TestFeatures runs every Gherkin scenario under features/. There is no tag
// filter on purpose: any feature file that exists in this directory MUST run.
// A scenario whose steps are not implemented fails as "undefined" rather than
// being silently skipped, so dead/aspirational specs cannot accumulate. If you
// add a feature file, add its step definitions in the same change.
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
	// Each step group registers its own per-scenario Before hook to set up
	// state, so there is no shared environment to wire here.
	steps.RegisterDBPersistenceGuardrailSteps(ctx)
	steps.RegisterTrustProtocolSteps(ctx)
	steps.RegisterScanLoopSteps(ctx)
	steps.RegisterStallDetectionSteps(ctx)
	steps.RegisterHarnessParitySteps(ctx)
	steps.RegisterInstructionParitySteps(ctx)
	steps.RegisterHookParitySteps(ctx)
	steps.RegisterLocalDevelopmentGuardrailSteps(ctx)
	steps.RegisterModelFamilyParitySteps(ctx)
	steps.RegisterSpecCoverageSteps(ctx)
}
