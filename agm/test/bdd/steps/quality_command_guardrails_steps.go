package steps

import "github.com/cucumber/godog"

const qualityCommandFeaturePath = "agm/test/bdd/features/quality_command_guardrails.feature"

type qualityCommandGuardrailStateKey struct{}

// RegisterQualityCommandGuardrailSteps registers BDD steps for repo quality command packages.
func RegisterQualityCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          qualityCommandGuardrailStateKey{},
		label:             "quality command package",
		featurePath:       qualityCommandFeaturePath,
		configuredPattern: `^quality command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates quality command package coverage$`,
		colocatedPattern:  `^quality command package "([^"]*)" should have a co-located SPEC$`,
	})
}
