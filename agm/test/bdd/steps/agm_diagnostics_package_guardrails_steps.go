package steps

import "github.com/cucumber/godog"

type agmDiagnosticsPackageGuardrailStateKey struct{}

// RegisterAGMDiagnosticsPackageGuardrailSteps registers diagnostics package coverage steps.
func RegisterAGMDiagnosticsPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmDiagnosticsPackageGuardrailStateKey{},
		label:             "AGM diagnostics package",
		featurePath:       "agm/test/bdd/features/agm_diagnostics_package_guardrails.feature",
		configuredPattern: `^AGM diagnostics package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates diagnostics package coverage$`,
		colocatedPattern:  `^AGM diagnostics package "([^"]*)" should have a co-located SPEC$`,
	})
}
