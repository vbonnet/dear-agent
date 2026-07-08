package steps

import "github.com/cucumber/godog"

const quotaMonitoringFeaturePath = "agm/test/bdd/features/quota_monitoring_guardrails.feature"

type quotaMonitoringGuardrailStateKey struct{}

// RegisterQuotaMonitoringGuardrailSteps registers BDD steps for quota monitoring packages.
func RegisterQuotaMonitoringGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          quotaMonitoringGuardrailStateKey{},
		label:             "quota monitoring package",
		featurePath:       quotaMonitoringFeaturePath,
		configuredPattern: `^quota monitoring package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates quota monitoring package coverage$`,
		colocatedPattern:  `^quota monitoring package "([^"]*)" should have a co-located SPEC$`,
	})
}
