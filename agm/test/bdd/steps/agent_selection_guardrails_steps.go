package steps

import "github.com/cucumber/godog"

const agentSelectionFeaturePath = "agm/test/bdd/features/agent_selection_guardrails.feature"

type agentSelectionGuardrailStateKey struct{}

// RegisterAgentSelectionGuardrailSteps registers BDD steps for AGM agent selection packages.
func RegisterAgentSelectionGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agentSelectionGuardrailStateKey{},
		label:             "agent selection package",
		featurePath:       agentSelectionFeaturePath,
		configuredPattern: `^agent selection package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates agent selection package coverage$`,
		colocatedPattern:  `^agent selection package "([^"]*)" should have a co-located SPEC$`,
	})
}
