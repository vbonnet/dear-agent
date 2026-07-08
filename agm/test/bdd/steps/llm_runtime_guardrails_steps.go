package steps

import "github.com/cucumber/godog"

const llmRuntimeFeaturePath = "agm/test/bdd/features/llm_runtime_guardrails.feature"

type llmRuntimeGuardrailStateKey struct{}

// RegisterLLMRuntimeGuardrailSteps registers BDD steps for LLM runtime packages.
func RegisterLLMRuntimeGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          llmRuntimeGuardrailStateKey{},
		label:             "LLM runtime package",
		featurePath:       llmRuntimeFeaturePath,
		configuredPattern: `^LLM runtime package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates LLM runtime package coverage$`,
		colocatedPattern:  `^LLM runtime package "([^"]*)" should have a co-located SPEC$`,
	})
}
