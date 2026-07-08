package steps

import (
	"github.com/cucumber/godog"
)

const engramKnowledgeFeaturePath = "agm/test/bdd/features/engram_knowledge_guardrails.feature"

type engramKnowledgeGuardrailStateKey struct{}

// RegisterEngramKnowledgeGuardrailSteps registers BDD steps for Engram knowledge packages.
func RegisterEngramKnowledgeGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramKnowledgeGuardrailStateKey{},
		label:             "engram knowledge package",
		featurePath:       engramKnowledgeFeaturePath,
		configuredPattern: `^Engram knowledge package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram knowledge package coverage$`,
		colocatedPattern:  `^Engram knowledge package "([^"]*)" should have a co-located SPEC$`,
	})
}
