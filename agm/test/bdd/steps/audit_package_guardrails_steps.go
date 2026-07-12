package steps

import "github.com/cucumber/godog"

const auditPackageFeaturePath = "agm/test/bdd/features/audit_package_guardrails.feature"

type auditPackageGuardrailStateKey struct{}

// RegisterAuditPackageGuardrailSteps registers BDD steps for audit support packages.
func RegisterAuditPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:           auditPackageGuardrailStateKey{},
		label:              "audit package",
		featurePath:        auditPackageFeaturePath,
		configuredPattern:  `^audit package "([^"]*)" is configured$`,
		validatePattern:    `^AGM validates audit package coverage$`,
		colocatedPattern:   `^audit package "([^"]*)" should have a co-located SPEC$`,
		requirementPattern: `^audit package SPEC should declare requirement "([^"]*)" containing "([^"]*)"$`,
	})
}
