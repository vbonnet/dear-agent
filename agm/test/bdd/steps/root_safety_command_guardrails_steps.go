package steps

import "github.com/cucumber/godog"

const rootSafetyFeaturePath = "agm/test/bdd/features/root_safety_command_guardrails.feature"

type rootSafetyPackageStateKey struct{}

// RegisterRootSafetyCommandGuardrailSteps registers safety command coverage steps.
func RegisterRootSafetyCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          rootSafetyPackageStateKey{},
		label:             "safety command package",
		featurePath:       rootSafetyFeaturePath,
		configuredPattern: `^safety command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates safety command package coverage$`,
		colocatedPattern:  `^safety command package "([^"]*)" should have a co-located SPEC$`,
	})
}
