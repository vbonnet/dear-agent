package steps

import "github.com/cucumber/godog"

const wayfinderLifecycleFeaturePath = "agm/test/bdd/features/wayfinder_lifecycle_guardrails.feature"

type wayfinderLifecycleGuardrailStateKey struct{}

// RegisterWayfinderLifecycleGuardrailSteps registers BDD steps for Wayfinder lifecycle packages.
func RegisterWayfinderLifecycleGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          wayfinderLifecycleGuardrailStateKey{},
		label:             "Wayfinder lifecycle package",
		featurePath:       wayfinderLifecycleFeaturePath,
		configuredPattern: `^Wayfinder lifecycle package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Wayfinder lifecycle package coverage$`,
		colocatedPattern:  `^Wayfinder lifecycle package "([^"]*)" should have a co-located SPEC$`,
	})
}
