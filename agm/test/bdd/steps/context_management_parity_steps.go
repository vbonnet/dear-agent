package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cucumber/godog"

	agentcontext "github.com/vbonnet/dear-agent/pkg/context"
)

type contextManagementParityStateKey struct{}

type contextManagementParityState struct {
	harness   string
	family    string
	model     string
	cli       agentcontext.CLI
	detector  *agentcontext.Detector
	usage     *agentcontext.Usage
	detectErr error
	env       map[string]*string
}

type contextHarnessRoute struct {
	cli agentcontext.CLI
	env string
}

var contextManagementParityEnvironment = []string{
	"CLAUDE_CONTEXT_USAGE", "CLAUDE_TOOL_RESULT", "CLAUDE_MODEL", "CLAUDE_MESSAGE_COUNT",
	"GEMINI_CONTEXT_USAGE", "GEMINI_TOOL_RESULT", "GEMINI_MODEL", "GEMINI_MESSAGE_COUNT",
	"OPENCODE_CONTEXT_USAGE", "OPENCODE_TOOL_RESULT", "OPENCODE_MODEL", "OPENCODE_MESSAGE_COUNT",
	"CODEX_CONTEXT_USAGE", "CODEX_TOOL_RESULT", "CODEX_MODEL", "CODEX_MESSAGE_COUNT",
	"PI_CONTEXT_USAGE", "PI_TOOL_RESULT", "PI_MODEL", "PI_MESSAGE_COUNT",
	"AGY_CONTEXT_USAGE", "ANTIGRAVITY_CONTEXT_USAGE", "AGY_MODEL", "ANTIGRAVITY_MODEL",
	"AGY_MESSAGE_COUNT", "ANTIGRAVITY_MESSAGE_COUNT", "DEAR_AGENT_MODEL", "DEAR_AGENT_MESSAGE_COUNT",
}

// RegisterContextManagementParitySteps registers cross-route context usage steps.
func RegisterContextManagementParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		env := snapshotContextManagementParityEnvironment()
		clearContextManagementParityEnvironment()
		registryPath := filepath.Join(packageSpecBDDRepoRoot(), "pkg", "context", "models.yaml")
		registry, err := agentcontext.NewRegistry(registryPath)
		if err != nil {
			restoreContextManagementParityEnvironment(env)
			return ctx, fmt.Errorf("load context model registry: %w", err)
		}
		return context.WithValue(ctx, contextManagementParityStateKey{}, &contextManagementParityState{
			detector: agentcontext.NewDetector(registry),
			env:      env,
		}), nil
	})
	ctx.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		if state, ok := ctx.Value(contextManagementParityStateKey{}).(*contextManagementParityState); ok {
			restoreContextManagementParityEnvironment(state.env)
		}
		return ctx, scenarioErr
	})

	ctx.Step(`^context route harness "([^"]*)" uses model family "([^"]*)"$`, contextRouteUsesModelFamily)
	ctx.Step(`^context route harness "([^"]*)" supplies counters outside the platform integer range$`, contextRouteSuppliesOutOfRangeCounters)
	ctx.Step(`^context route harness "([^"]*)" supplies competing nested counters$`, contextRouteSuppliesCompetingNestedCounters)
	ctx.Step(`^shared context usage is detected without native counters$`, sharedContextUsageIsDetected)
	ctx.Step(`^shared context usage detection is attempted$`, sharedContextUsageDetectionIsAttempted)
	ctx.Step(`^context usage should preserve the configured model family "([^"]*)"$`, contextUsageShouldPreserveModelFamily)
	ctx.Step(`^context usage should be marked as estimated$`, contextUsageShouldBeMarkedEstimated)
	ctx.Step(`^context usage should have a positive registered window$`, contextUsageShouldHavePositiveWindow)
	ctx.Step(`^context detection should reject the out-of-range counters$`, contextDetectionShouldRejectOutOfRangeCounters)
	ctx.Step(`^context detection should select the lexically first nested counter set$`, contextDetectionShouldSelectLexicallyFirstCounters)
}

func contextRouteSuppliesOutOfRangeCounters(ctx context.Context, harness string) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	route, ok := contextHarnessRouteForName(harness)
	if !ok {
		return fmt.Errorf("unsupported context harness %q", harness)
	}
	tooLarge := "2147483648"
	if strconv.IntSize == 64 {
		tooLarge = "9223372036854775808"
	}
	state.harness, state.cli = harness, route.cli
	return os.Setenv(route.env, fmt.Sprintf(`{"used_tokens":1,"total_tokens":%s}`, tooLarge))
}

func contextRouteSuppliesCompetingNestedCounters(ctx context.Context, harness string) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	route, ok := contextHarnessRouteForName(harness)
	if !ok {
		return fmt.Errorf("unsupported context harness %q", harness)
	}
	state.harness, state.cli = harness, route.cli
	payload := `{"z_route":{"used_tokens":900,"total_tokens":1000,"model_id":"z-model"},"a_route":{"used_tokens":100,"total_tokens":1000,"model_id":"a-model"}}`
	return os.Setenv(route.env, payload)
}

func contextHarnessRouteForName(harness string) (contextHarnessRoute, bool) {
	route, ok := map[string]contextHarnessRoute{
		"claude-code":  {agentcontext.CLIClaude, "CLAUDE_CONTEXT_USAGE"},
		"codex-cli":    {agentcontext.CLICodex, "CODEX_CONTEXT_USAGE"},
		"pi-cli":       {agentcontext.CLIPi, "PI_CONTEXT_USAGE"},
		"agy":          {agentcontext.CLIAgy, "AGY_CONTEXT_USAGE"},
		"opencode-cli": {agentcontext.CLIOpenCode, "OPENCODE_CONTEXT_USAGE"},
	}[harness]
	return route, ok
}

func snapshotContextManagementParityEnvironment() map[string]*string {
	snapshot := make(map[string]*string, len(contextManagementParityEnvironment))
	for _, name := range contextManagementParityEnvironment {
		if value, ok := os.LookupEnv(name); ok {
			valueCopy := value
			snapshot[name] = &valueCopy
			continue
		}
		snapshot[name] = nil
	}
	return snapshot
}

func clearContextManagementParityEnvironment() {
	for _, name := range contextManagementParityEnvironment {
		_ = os.Unsetenv(name)
	}
}

func restoreContextManagementParityEnvironment(snapshot map[string]*string) {
	for _, name := range contextManagementParityEnvironment {
		value := snapshot[name]
		if value == nil {
			_ = os.Unsetenv(name)
			continue
		}
		_ = os.Setenv(name, *value)
	}
}

func contextRouteUsesModelFamily(ctx context.Context, harness, family string) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	cli, ok := map[string]agentcontext.CLI{
		"claude-code":  agentcontext.CLIClaude,
		"codex-cli":    agentcontext.CLICodex,
		"pi-cli":       agentcontext.CLIPi,
		"agy":          agentcontext.CLIAgy,
		"opencode-cli": agentcontext.CLIOpenCode,
	}[harness]
	if !ok {
		return fmt.Errorf("unsupported context harness %q", harness)
	}
	model, ok := map[string]string{
		"anthropic": "claude-sonnet-4.5",
		"openai":    "gpt-5.5",
		"gemini":    "gemini-3.5-flash",
		"glm":       "z-ai/glm-5.2",
		"deepseek":  "deepseek/deepseek-v4-pro",
		"nemotron":  "nvidia/nemotron-3-ultra",
		"qwen":      "qwen/qwen3.6-max",
	}[family]
	if !ok {
		return fmt.Errorf("unsupported context model family %q", family)
	}
	state.harness, state.family, state.model, state.cli = harness, family, model, cli
	return os.Setenv("DEAR_AGENT_MODEL", model)
}

func sharedContextUsageIsDetected(ctx context.Context) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	state.usage, err = state.detector.DetectFromSession("bdd-session", state.cli)
	return err
}

func sharedContextUsageDetectionIsAttempted(ctx context.Context) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	state.usage, state.detectErr = state.detector.DetectFromSession("bdd-session", state.cli)
	return nil
}

func contextDetectionShouldRejectOutOfRangeCounters(ctx context.Context) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	if state.detectErr == nil {
		return fmt.Errorf("context route %s accepted counters outside the platform integer range", state.harness)
	}
	if state.usage != nil {
		return fmt.Errorf("context route %s returned usage after rejecting counters", state.harness)
	}
	return nil
}

func contextDetectionShouldSelectLexicallyFirstCounters(ctx context.Context) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	if state.detectErr != nil {
		return state.detectErr
	}
	if state.usage == nil {
		return fmt.Errorf("context route %s returned no usage", state.harness)
	}
	if state.usage.UsedTokens != 100 || state.usage.TotalTokens != 1000 || state.usage.ModelID != "a-model" {
		return fmt.Errorf("context route %s selected used=%d total=%d model=%q", state.harness, state.usage.UsedTokens, state.usage.TotalTokens, state.usage.ModelID)
	}
	return nil
}

func contextUsageShouldPreserveModelFamily(ctx context.Context, family string) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	if state.usage == nil {
		return fmt.Errorf("context usage was not detected")
	}
	if family != state.family || state.usage.ModelID != state.model {
		return fmt.Errorf("context route %s/%s resolved model %q, want %q", state.harness, family, state.usage.ModelID, state.model)
	}
	return nil
}

func contextUsageShouldBeMarkedEstimated(ctx context.Context) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	if state.usage == nil || !state.usage.Estimated {
		return fmt.Errorf("context fallback was not marked estimated")
	}
	return nil
}

func contextUsageShouldHavePositiveWindow(ctx context.Context) error {
	state, err := getContextManagementParityState(ctx)
	if err != nil {
		return err
	}
	if state.usage == nil || state.usage.TotalTokens <= 0 {
		return fmt.Errorf("context route has no positive window")
	}
	return nil
}

func getContextManagementParityState(ctx context.Context) (*contextManagementParityState, error) {
	state, ok := ctx.Value(contextManagementParityStateKey{}).(*contextManagementParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("context management parity state not initialized")
	}
	return state, nil
}
