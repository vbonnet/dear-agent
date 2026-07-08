package steps

import "github.com/cucumber/godog"

type agmRuntimePackageGuardrailStateKey struct{}

// RegisterAGMRuntimePackageGuardrailSteps verifies that AGM runtime support
// packages keep executable SPEC traceability.
func RegisterAGMRuntimePackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmRuntimePackageGuardrailStateKey{},
		label:             "AGM runtime package",
		featurePath:       "agm/test/bdd/features/agm_runtime_package_guardrails.feature",
		configuredPattern: `^AGM runtime package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates AGM runtime package coverage$`,
		colocatedPattern:  `^AGM runtime package "([^"]*)" should have a co-located SPEC$`,
	})
}
