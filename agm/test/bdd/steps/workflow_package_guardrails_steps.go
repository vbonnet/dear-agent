package steps

import "github.com/cucumber/godog"

const workflowPackageFeaturePath = "agm/test/bdd/features/workflow_package_guardrails.feature"

type workflowPackageGuardrailStateKey struct{}

// RegisterWorkflowPackageGuardrailSteps registers BDD steps for workflow implementation packages.
func RegisterWorkflowPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          workflowPackageGuardrailStateKey{},
		label:             "workflow package",
		featurePath:       workflowPackageFeaturePath,
		configuredPattern: `^workflow package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates workflow package coverage$`,
		colocatedPattern:  `^workflow package "([^"]*)" should have a co-located SPEC$`,
	})
}
