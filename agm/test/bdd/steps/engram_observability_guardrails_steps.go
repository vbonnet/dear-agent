package steps

import "github.com/cucumber/godog"

const engramObservabilityFeaturePath = "agm/test/bdd/features/engram_observability_guardrails.feature"

type engramObservabilityGuardrailStateKey struct{}

// RegisterEngramObservabilityGuardrailSteps registers BDD steps for Engram observability packages.
func RegisterEngramObservabilityGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramObservabilityGuardrailStateKey{},
		label:             "Engram observability package",
		featurePath:       engramObservabilityFeaturePath,
		configuredPattern: `^Engram observability package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram observability package coverage$`,
		colocatedPattern:  `^Engram observability package "([^"]*)" should have a co-located SPEC$`,
	})
}
