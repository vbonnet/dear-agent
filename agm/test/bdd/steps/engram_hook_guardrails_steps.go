package steps

import (
	"github.com/cucumber/godog"
)

const engramHookFeaturePath = "agm/test/bdd/features/engram_hook_guardrails.feature"

type engramHookGuardrailStateKey struct{}

// RegisterEngramHookGuardrailSteps registers BDD steps for Engram hook packages.
func RegisterEngramHookGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramHookGuardrailStateKey{},
		label:             "engram hook package",
		featurePath:       engramHookFeaturePath,
		configuredPattern: `^Engram hook package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram hook package coverage$`,
		colocatedPattern:  `^Engram hook package "([^"]*)" should have a co-located SPEC$`,
	})
}
