package steps

import "github.com/cucumber/godog"

const engramAnalysisConfigurationFeaturePath = "agm/test/bdd/features/engram_analysis_configuration_guardrails.feature"

type engramAnalysisConfigurationGuardrailStateKey struct{}

// RegisterEngramAnalysisConfigurationGuardrailSteps registers BDD steps for Engram analysis packages.
func RegisterEngramAnalysisConfigurationGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramAnalysisConfigurationGuardrailStateKey{},
		label:             "Engram analysis configuration package",
		featurePath:       engramAnalysisConfigurationFeaturePath,
		configuredPattern: `^Engram analysis configuration package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram analysis configuration package coverage$`,
		colocatedPattern:  `^Engram analysis configuration package "([^"]*)" should have a co-located SPEC$`,
	})
}
