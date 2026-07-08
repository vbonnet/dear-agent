package steps

import "github.com/cucumber/godog"

const agmControlSurfaceFeaturePath = "agm/test/bdd/features/agm_control_surface_guardrails.feature"

type agmControlSurfaceGuardrailStateKey struct{}

// RegisterAGMControlSurfaceGuardrailSteps registers BDD steps for AGM control-plane packages.
func RegisterAGMControlSurfaceGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmControlSurfaceGuardrailStateKey{},
		label:             "AGM control surface package",
		featurePath:       agmControlSurfaceFeaturePath,
		configuredPattern: `^AGM control surface package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates control surface package coverage$`,
		colocatedPattern:  `^AGM control surface package "([^"]*)" should have a co-located SPEC$`,
	})
}
