package steps

import "github.com/cucumber/godog"

const engramCLISupportFeaturePath = "agm/test/bdd/features/engram_cli_support_guardrails.feature"

type engramCLISupportGuardrailStateKey struct{}

// RegisterEngramCLISupportGuardrailSteps registers BDD steps for Engram CLI support packages.
func RegisterEngramCLISupportGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramCLISupportGuardrailStateKey{},
		label:             "Engram CLI support package",
		featurePath:       engramCLISupportFeaturePath,
		configuredPattern: `^Engram CLI support package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram CLI support package coverage$`,
		colocatedPattern:  `^Engram CLI support package "([^"]*)" should have a co-located SPEC$`,
	})
}
