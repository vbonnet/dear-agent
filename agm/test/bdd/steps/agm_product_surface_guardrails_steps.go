package steps

import "github.com/cucumber/godog"

type agmProductSurfaceGuardrailStateKey struct{}

// RegisterAGMProductSurfaceGuardrailSteps registers AGM product surface coverage steps.
func RegisterAGMProductSurfaceGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmProductSurfaceGuardrailStateKey{},
		label:             "AGM product surface",
		featurePath:       "agm/test/bdd/features/agm_product_surface_guardrails.feature",
		configuredPattern: `^AGM product surface "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates product surface coverage$`,
		colocatedPattern:  `^AGM product surface "([^"]*)" should have a co-located SPEC$`,
	})
}
