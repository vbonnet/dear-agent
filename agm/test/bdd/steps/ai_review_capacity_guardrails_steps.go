package steps

import "github.com/cucumber/godog"

const aiReviewCapacityFeaturePath = "agm/test/bdd/features/ai_review_capacity_guardrails.feature"

type aiReviewCapacityPackageStateKey struct{}

// RegisterAIReviewCapacityGuardrailSteps binds the semantic-review capacity
// contract to its dedicated executable traceability feature.
func RegisterAIReviewCapacityGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:           aiReviewCapacityPackageStateKey{},
		label:              "AI review package",
		featurePath:        aiReviewCapacityFeaturePath,
		configuredPattern:  `^AI review package "([^"]*)" is configured$`,
		validatePattern:    `^AGM validates AI review capacity coverage$`,
		colocatedPattern:   `^AI review package "([^"]*)" should have a co-located SPEC$`,
		requirementPattern: `^AI review requirement "([^"]*)" should contain "([^"]*)"$`,
	})
}
