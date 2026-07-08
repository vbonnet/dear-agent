package steps

import "github.com/cucumber/godog"

type observabilityPackageGuardrailStateKey struct{}

// RegisterObservabilityPackageGuardrailSteps verifies that observability
// packages keep executable SPEC traceability.
func RegisterObservabilityPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          observabilityPackageGuardrailStateKey{},
		label:             "observability package",
		featurePath:       "agm/test/bdd/features/observability_package_guardrails.feature",
		configuredPattern: `^observability package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates observability package coverage$`,
		colocatedPattern:  `^observability package "([^"]*)" should have a co-located SPEC$`,
	})
}
