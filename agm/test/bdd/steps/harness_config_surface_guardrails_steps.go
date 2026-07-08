package steps

import "github.com/cucumber/godog"

const harnessConfigSurfaceFeaturePath = "agm/test/bdd/features/harness_config_surface_guardrails.feature"

type harnessConfigSurfaceGuardrailStateKey struct{}

// RegisterHarnessConfigSurfaceGuardrailSteps registers BDD steps for harness config surface directories.
func RegisterHarnessConfigSurfaceGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          harnessConfigSurfaceGuardrailStateKey{},
		label:             "harness configuration surface",
		featurePath:       harnessConfigSurfaceFeaturePath,
		configuredPattern: `^harness configuration surface "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates harness configuration surface coverage$`,
		colocatedPattern:  `^harness configuration surface "([^"]*)" should have a co-located SPEC$`,
	})
}
