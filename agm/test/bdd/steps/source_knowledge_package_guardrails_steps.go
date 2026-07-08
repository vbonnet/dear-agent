package steps

import "github.com/cucumber/godog"

type sourceKnowledgePackageGuardrailStateKey struct{}

// RegisterSourceKnowledgePackageGuardrailSteps verifies that source adapters,
// paper search, and wikibrain keep executable SPEC traceability.
func RegisterSourceKnowledgePackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          sourceKnowledgePackageGuardrailStateKey{},
		label:             "source and knowledge package",
		featurePath:       "agm/test/bdd/features/source_knowledge_package_guardrails.feature",
		configuredPattern: `^source and knowledge package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates source and knowledge package coverage$`,
		colocatedPattern:  `^source and knowledge package "([^"]*)" should have a co-located SPEC$`,
	})
}
