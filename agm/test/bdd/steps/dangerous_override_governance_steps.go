package steps

import "github.com/cucumber/godog"

const dangerousOverrideFeaturePath = "agm/test/bdd/features/dangerous_override_governance.feature"

type dangerousOverrideGuardrailStateKey struct{}

// RegisterDangerousOverrideGovernanceSteps registers BDD steps for the shared
// dangerous-override contract. Keeping the package under the same guardrail as
// every other spec'd package is what stops a new override kind from being added
// beside the contract instead of through it.
func RegisterDangerousOverrideGovernanceSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          dangerousOverrideGuardrailStateKey{},
		label:             "dangerous override package",
		featurePath:       dangerousOverrideFeaturePath,
		configuredPattern: `^dangerous override package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates dangerous override package coverage$`,
		colocatedPattern:  `^dangerous override package "([^"]*)" should have a co-located SPEC$`,
	})
}
