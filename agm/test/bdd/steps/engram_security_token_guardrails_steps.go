package steps

import "github.com/cucumber/godog"

const engramTrustBudgetFeaturePath = "agm/test/bdd/features/engram_security_token_guardrails.feature"

type engramSecurityTokenGuardrailStateKey struct{}

// RegisterEngramSecurityTokenGuardrailSteps registers BDD steps for Engram security packages.
func RegisterEngramSecurityTokenGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramSecurityTokenGuardrailStateKey{},
		label:             "Engram security token package",
		featurePath:       engramTrustBudgetFeaturePath,
		configuredPattern: `^Engram security token package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram security token package coverage$`,
		colocatedPattern:  `^Engram security token package "([^"]*)" should have a co-located SPEC$`,
	})
}
