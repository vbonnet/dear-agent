package steps

import "github.com/cucumber/godog"

const pluginSkillPackageFeaturePath = "agm/test/bdd/features/plugin_skill_package_guardrails.feature"

type pluginSkillPackageGuardrailStateKey struct{}

// RegisterPluginSkillPackageGuardrailSteps registers BDD steps for plugin and skill packages.
func RegisterPluginSkillPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          pluginSkillPackageGuardrailStateKey{},
		label:             "plugin and skill package",
		featurePath:       pluginSkillPackageFeaturePath,
		configuredPattern: `^plugin and skill package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates plugin and skill package coverage$`,
		colocatedPattern:  `^plugin and skill package "([^"]*)" should have a co-located SPEC$`,
	})
}
