package steps

import "github.com/cucumber/godog"

const sandboxProviderFeaturePath = "agm/test/bdd/features/sandbox_provider_guardrails.feature"

type sandboxProviderGuardrailStateKey struct{}

// RegisterSandboxProviderGuardrailSteps registers BDD steps for sandbox provider packages.
func RegisterSandboxProviderGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          sandboxProviderGuardrailStateKey{},
		label:             "sandbox provider package",
		featurePath:       sandboxProviderFeaturePath,
		configuredPattern: `^sandbox provider package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates sandbox provider package coverage$`,
		colocatedPattern:  `^sandbox provider package "([^"]*)" should have a co-located SPEC$`,
	})
}
