package steps

import "github.com/cucumber/godog"

type apiGatewayPackageGuardrailStateKey struct{}

// RegisterAPIGatewayPackageGuardrailSteps verifies that API and gateway
// packages keep executable SPEC traceability.
func RegisterAPIGatewayPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          apiGatewayPackageGuardrailStateKey{},
		label:             "API and gateway package",
		featurePath:       "agm/test/bdd/features/api_gateway_package_guardrails.feature",
		configuredPattern: `^API and gateway package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates API and gateway package coverage$`,
		colocatedPattern:  `^API and gateway package "([^"]*)" should have a co-located SPEC$`,
	})
}
