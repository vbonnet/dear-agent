package steps

import "github.com/cucumber/godog"

const mcpCommandFeaturePath = "agm/test/bdd/features/mcp_command_guardrails.feature"

type mcpCommandGuardrailStateKey struct{}

// RegisterMCPCommandGuardrailSteps registers BDD steps for MCP command packages.
func RegisterMCPCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          mcpCommandGuardrailStateKey{},
		label:             "MCP command package",
		featurePath:       mcpCommandFeaturePath,
		configuredPattern: `^MCP command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates MCP command package coverage$`,
		colocatedPattern:  `^MCP command package "([^"]*)" should have a co-located SPEC$`,
	})
}
