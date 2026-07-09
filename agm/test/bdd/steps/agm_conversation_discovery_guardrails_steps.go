package steps

import "github.com/cucumber/godog"

type agmConversationDiscoveryGuardrailStateKey struct{}

// RegisterAGMConversationDiscoveryGuardrailSteps registers conversation package coverage steps.
func RegisterAGMConversationDiscoveryGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmConversationDiscoveryGuardrailStateKey{},
		label:             "AGM conversation package",
		featurePath:       "agm/test/bdd/features/agm_conversation_discovery_guardrails.feature",
		configuredPattern: `^AGM conversation package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates conversation package coverage$`,
		colocatedPattern:  `^AGM conversation package "([^"]*)" should have a co-located SPEC$`,
	})
}
