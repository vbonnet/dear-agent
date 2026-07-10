package steps

import "github.com/cucumber/godog"

const engramGovernanceRuntimeFeaturePath = "agm/test/bdd/features/engram_governance_runtime_guardrails.feature"

type engramGovernanceRuntimeGuardrailStateKey struct{}

// RegisterEngramGovernanceRuntimeGuardrailSteps registers BDD steps for Engram governance packages.
func RegisterEngramGovernanceRuntimeGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramGovernanceRuntimeGuardrailStateKey{},
		label:             "Engram governance runtime package",
		featurePath:       engramGovernanceRuntimeFeaturePath,
		configuredPattern: `^Engram governance runtime package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram governance runtime package coverage$`,
		colocatedPattern:  `^Engram governance runtime package "([^"]*)" should have a co-located SPEC$`,
	})
}
