package steps

import "github.com/cucumber/godog"

type diskLedgerPackageGuardrailStateKey struct{}

// RegisterDiskLedgerPackageGuardrailSteps registers disk ledger package coverage steps.
func RegisterDiskLedgerPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          diskLedgerPackageGuardrailStateKey{},
		label:             "disk ledger package",
		featurePath:       "agm/test/bdd/features/disk_ledger_guardrails.feature",
		configuredPattern: `^disk ledger package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates disk ledger package coverage$`,
		colocatedPattern:  `^disk ledger package "([^"]*)" should have a co-located SPEC$`,
	})
}
