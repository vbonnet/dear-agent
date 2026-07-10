package steps

import "github.com/cucumber/godog"

const engramCoreContextFeaturePath = "agm/test/bdd/features/engram_core_context_guardrails.feature"

type engramCoreContextGuardrailStateKey struct{}

// RegisterEngramCoreContextGuardrailSteps registers BDD steps for Engram core context packages.
func RegisterEngramCoreContextGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramCoreContextGuardrailStateKey{},
		label:             "Engram core context package",
		featurePath:       engramCoreContextFeaturePath,
		configuredPattern: `^Engram core context package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram core context package coverage$`,
		colocatedPattern:  `^Engram core context package "([^"]*)" should have a co-located SPEC$`,
	})
}
