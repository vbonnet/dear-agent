package steps

import "github.com/cucumber/godog"

const engramReflectionStorageFeaturePath = "agm/test/bdd/features/engram_reflection_storage_guardrails.feature"

type engramReflectionStorageGuardrailStateKey struct{}

// RegisterEngramReflectionStorageGuardrailSteps registers BDD steps for Engram learning packages.
func RegisterEngramReflectionStorageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramReflectionStorageGuardrailStateKey{},
		label:             "Engram reflection storage package",
		featurePath:       engramReflectionStorageFeaturePath,
		configuredPattern: `^Engram reflection storage package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram reflection storage package coverage$`,
		colocatedPattern:  `^Engram reflection storage package "([^"]*)" should have a co-located SPEC$`,
	})
}
