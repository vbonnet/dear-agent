package steps

import "github.com/cucumber/godog"

type agmSupervisionRecoveryGuardrailStateKey struct{}

// RegisterAGMSupervisionRecoveryGuardrailSteps registers supervision package coverage steps.
func RegisterAGMSupervisionRecoveryGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmSupervisionRecoveryGuardrailStateKey{},
		label:             "AGM supervision package",
		featurePath:       "agm/test/bdd/features/agm_supervision_recovery_guardrails.feature",
		configuredPattern: `^AGM supervision package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates supervision package coverage$`,
		colocatedPattern:  `^AGM supervision package "([^"]*)" should have a co-located SPEC$`,
	})
}
