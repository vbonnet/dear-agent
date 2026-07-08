package steps

import "github.com/cucumber/godog"

const workflowCommandFeaturePath = "agm/test/bdd/features/workflow_command_guardrails.feature"

type workflowCommandGuardrailStateKey struct{}

// RegisterWorkflowCommandGuardrailSteps registers BDD steps for workflow command packages.
func RegisterWorkflowCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          workflowCommandGuardrailStateKey{},
		label:             "workflow command package",
		featurePath:       workflowCommandFeaturePath,
		configuredPattern: `^workflow command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates workflow command package coverage$`,
		colocatedPattern:  `^workflow command package "([^"]*)" should have a co-located SPEC$`,
	})
}
