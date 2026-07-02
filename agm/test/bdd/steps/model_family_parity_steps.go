package steps

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/pkg/llm/provider"
)

type modelFamilyParityState struct {
	family         string
	modelID        string
	resolvedFamily string
	resolvedModel  string
	created        provider.Provider
	capabilities   provider.Capabilities
}

type modelFamilyParityStateKey struct{}

// RegisterModelFamilyParitySteps registers BDD steps for LLM provider family parity.
func RegisterModelFamilyParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, modelFamilyParityStateKey{}, &modelFamilyParityState{}), nil
	})

	ctx.Step(`^LLM model identifier "([^"]*)" for model family "([^"]*)"$`, llmModelIdentifierForModelFamily)
	ctx.Step(`^dear-agent resolves the LLM provider family$`, dearAgentResolvesTheLLMProviderFamily)
	ctx.Step(`^the resolved provider family should be "([^"]*)"$`, resolvedProviderFamilyShouldBe)
	ctx.Step(`^the resolved provider model should be "([^"]*)"$`, resolvedProviderModelShouldBe)
	ctx.Step(`^AGM model family "([^"]*)" has a default route$`, agmModelFamilyHasDefaultRoute)
	ctx.Step(`^dear-agent resolves the default route through the LLM provider resolver$`, dearAgentResolvesTheDefaultRouteThroughTheLLMProviderResolver)
	ctx.Step(`^the resolved provider model should not be empty$`, resolvedProviderModelShouldNotBeEmpty)
	ctx.Step(`^OpenRouter API key authentication is configured$`, openRouterAPIKeyAuthenticationIsConfigured)
	ctx.Step(`^dear-agent creates provider family "([^"]*)" with model "([^"]*)"$`, dearAgentCreatesProviderFamilyWithModel)
	ctx.Step(`^the created provider should be named "([^"]*)"$`, createdProviderShouldBeNamed)
	ctx.Step(`^dear-agent reads OpenRouter provider capabilities$`, dearAgentReadsOpenRouterProviderCapabilities)
	ctx.Step(`^OpenRouter capabilities should include model "([^"]*)"$`, openRouterCapabilitiesShouldIncludeModel)
}

func llmModelIdentifierForModelFamily(ctx context.Context, modelID, family string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	family = strings.ToLower(family)
	if !agent.IsSupportedModelFamily(family) {
		return fmt.Errorf("model family %q is not supported", family)
	}
	if got := agent.ModelFamilyForName(modelID); got != family {
		return fmt.Errorf("model %q maps to family %q, want %q", modelID, got, family)
	}
	state.family = family
	state.modelID = modelID
	return nil
}

func dearAgentResolvesTheLLMProviderFamily(ctx context.Context) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	family, model, err := provider.NewResolver().Resolve(state.modelID)
	if err != nil {
		return err
	}
	state.resolvedFamily = family
	state.resolvedModel = model
	return nil
}

func resolvedProviderFamilyShouldBe(ctx context.Context, want string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	if state.resolvedFamily != want {
		return fmt.Errorf("resolved provider family = %q, want %q", state.resolvedFamily, want)
	}
	return nil
}

func resolvedProviderModelShouldBe(ctx context.Context, want string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	if state.resolvedModel != want {
		return fmt.Errorf("resolved provider model = %q, want %q", state.resolvedModel, want)
	}
	return nil
}

func agmModelFamilyHasDefaultRoute(ctx context.Context, family string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	family = strings.ToLower(family)
	model, ok := agent.DefaultModelForFamily(family)
	if !ok {
		return fmt.Errorf("model family %q has no AGM default route", family)
	}
	state.family = family
	state.modelID = model.FullName
	return nil
}

func dearAgentResolvesTheDefaultRouteThroughTheLLMProviderResolver(ctx context.Context) error {
	return dearAgentResolvesTheLLMProviderFamily(ctx)
}

func resolvedProviderModelShouldNotBeEmpty(ctx context.Context) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	if state.resolvedModel == "" {
		return fmt.Errorf("resolved provider model is empty for family %q", state.family)
	}
	return nil
}

func openRouterAPIKeyAuthenticationIsConfigured(ctx context.Context) error {
	return os.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")
}

func dearAgentCreatesProviderFamilyWithModel(ctx context.Context, family, model string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	created, err := provider.NewFactory().NewProvider(family, model)
	if err != nil {
		return err
	}
	state.created = created
	return nil
}

func createdProviderShouldBeNamed(ctx context.Context, want string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	if state.created == nil {
		return fmt.Errorf("provider was not created")
	}
	if got := state.created.Name(); got != want {
		return fmt.Errorf("provider name = %q, want %q", got, want)
	}
	return nil
}

func dearAgentReadsOpenRouterProviderCapabilities(ctx context.Context) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	created, err := provider.NewOpenRouterProvider(provider.OpenRouterConfig{})
	if err != nil {
		return err
	}
	state.capabilities = created.Capabilities()
	return nil
}

func openRouterCapabilitiesShouldIncludeModel(ctx context.Context, model string) error {
	state, err := getModelFamilyParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(state.capabilities.SupportedModels, model) {
		return fmt.Errorf("OpenRouter capabilities missing %q: %v", model, state.capabilities.SupportedModels)
	}
	return nil
}

func getModelFamilyParityState(ctx context.Context) (*modelFamilyParityState, error) {
	state, ok := ctx.Value(modelFamilyParityStateKey{}).(*modelFamilyParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("model family parity state not initialized")
	}
	return state, nil
}
