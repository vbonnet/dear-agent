package steps

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/pkg/promptcache"
	"github.com/vbonnet/dear-agent/pkg/stophook"
	"github.com/vbonnet/dear-agent/pkg/synchub"
)

const agentUtilityParityFeaturePath = "agm/test/bdd/features/agent_utility_parity.feature"

type agentUtilityPackageGuardrailStateKey struct{}
type agentUtilityRouteStateKey struct{}

type agentUtilityRouteState struct {
	harness string
	family  promptcache.ModelFamily
	policy  promptcache.FamilyPolicy
	input   *stophook.Input
	hubOK   bool
}

// RegisterAgentUtilityParitySteps verifies package traceability and executes
// the shared utility contracts on every supported route.
func RegisterAgentUtilityParitySteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agentUtilityPackageGuardrailStateKey{},
		label:             "agent utility package",
		featurePath:       agentUtilityParityFeaturePath,
		configuredPattern: `^agent utility package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates agent utility package coverage$`,
		colocatedPattern:  `^agent utility package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, agentUtilityRouteStateKey{}, &agentUtilityRouteState{}), nil
	})
	ctx.Step(`^agent utility harness "([^"]*)" uses model family "([^"]*)"$`, agentUtilityHarnessUsesModelFamily)
	ctx.Step(`^shared agent utility contracts are resolved$`, sharedAgentUtilityContractsAreResolved)
	ctx.Step(`^the cache policy should preserve model family "([^"]*)"$`, cachePolicyShouldPreserveModelFamily)
	ctx.Step(`^the hook input should preserve harness "([^"]*)"$`, hookInputShouldPreserveHarness)
	ctx.Step(`^the synchronization session should remain route neutral$`, synchronizationSessionShouldRemainRouteNeutral)
}

func agentUtilityHarnessUsesModelFamily(ctx context.Context, harness, family string) error {
	state, err := getAgentUtilityRouteState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}, harness) {
		return fmt.Errorf("unsupported agent utility harness %q", harness)
	}
	families := map[string]promptcache.ModelFamily{
		"anthropic": promptcache.FamilyAnthropic,
		"openai":    promptcache.FamilyOpenAI,
		"gemini":    promptcache.FamilyGemini,
		"glm":       promptcache.FamilyGLM,
		"deepseek":  promptcache.FamilyDeepSeek,
		"nemotron":  promptcache.FamilyNemotron,
		"qwen":      promptcache.FamilyQwen,
	}
	modelFamily, ok := families[family]
	if !ok {
		return fmt.Errorf("unsupported agent utility model family %q", family)
	}
	state.harness, state.family = harness, modelFamily
	return nil
}

func sharedAgentUtilityContractsAreResolved(ctx context.Context) error {
	state, err := getAgentUtilityRouteState(ctx)
	if err != nil {
		return err
	}
	state.policy, err = promptcache.PolicyForFamily(state.family, promptcache.TierPersistent)
	if err != nil {
		return err
	}
	payload := fmt.Sprintf(`{"harness":%q,"session_id":"bdd-session","cwd":"/tmp"}`, state.harness)
	state.input, err = stophook.ReadInput(strings.NewReader(payload))
	if err != nil {
		return err
	}
	hub, err := synchub.New(synchub.Options{SessionID: "bdd-session"})
	if err != nil {
		return err
	}
	state.hubOK = hub.Close() == nil
	return nil
}

func cachePolicyShouldPreserveModelFamily(ctx context.Context, family string) error {
	state, err := getAgentUtilityRouteState(ctx)
	if err != nil {
		return err
	}
	if string(state.policy.Family) != family {
		return fmt.Errorf("cache policy family = %q, want %q", state.policy.Family, family)
	}
	if state.family == promptcache.FamilyAnthropic {
		if state.policy.ProviderDefault || state.policy.Control == nil {
			return fmt.Errorf("anthropic cache policy does not carry explicit control")
		}
	} else if !state.policy.ProviderDefault || state.policy.Control != nil {
		return fmt.Errorf("family %s inherited an Anthropic cache control", family)
	}
	return nil
}

func hookInputShouldPreserveHarness(ctx context.Context, harness string) error {
	state, err := getAgentUtilityRouteState(ctx)
	if err != nil {
		return err
	}
	if state.input == nil || state.input.Harness != harness || state.input.SessionID != "bdd-session" {
		return fmt.Errorf("normalized hook input = %+v, want harness %s", state.input, harness)
	}
	return nil
}

func synchronizationSessionShouldRemainRouteNeutral(ctx context.Context) error {
	state, err := getAgentUtilityRouteState(ctx)
	if err != nil {
		return err
	}
	if !state.hubOK {
		return fmt.Errorf("synchronization session failed for %s/%s", state.harness, state.family)
	}
	return nil
}

func getAgentUtilityRouteState(ctx context.Context) (*agentUtilityRouteState, error) {
	state, ok := ctx.Value(agentUtilityRouteStateKey{}).(*agentUtilityRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("agent utility route state not initialized")
	}
	return state, nil
}
