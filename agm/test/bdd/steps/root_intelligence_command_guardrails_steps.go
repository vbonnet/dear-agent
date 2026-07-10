package steps

import "github.com/cucumber/godog"

const rootIntelligenceFeaturePath = "agm/test/bdd/features/root_intelligence_command_guardrails.feature"

type rootIntelligencePackageStateKey struct{}

// RegisterRootIntelligenceCommandGuardrailSteps registers intelligence command coverage steps.
func RegisterRootIntelligenceCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          rootIntelligencePackageStateKey{},
		label:             "intelligence command package",
		featurePath:       rootIntelligenceFeaturePath,
		configuredPattern: `^intelligence command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates intelligence command package coverage$`,
		colocatedPattern:  `^intelligence command package "([^"]*)" should have a co-located SPEC$`,
	})
}
