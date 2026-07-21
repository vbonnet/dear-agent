package steps

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
	engrammigrate "github.com/vbonnet/dear-agent/pkg/engram/migrate"
)

const evaluationControlParityFeaturePath = "agm/test/bdd/features/evaluation_control_parity.feature"

type evaluationControlPackageStateKey struct{}
type evaluationControlRouteStateKey struct{}

type evaluationControlRouteState struct {
	harness  string
	family   string
	model    string
	result   benchmarks.TaskResult
	migrated string
}

// RegisterEvaluationControlParitySteps verifies benchmark, migration, and HITL
// package traceability and route-neutral execution inputs.
func RegisterEvaluationControlParitySteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          evaluationControlPackageStateKey{},
		label:             "evaluation control package",
		featurePath:       evaluationControlParityFeaturePath,
		configuredPattern: `^evaluation control package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates evaluation control package coverage$`,
		colocatedPattern:  `^evaluation control package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, evaluationControlRouteStateKey{}, &evaluationControlRouteState{}), nil
	})
	ctx.Step(`^evaluation harness "([^"]*)" uses model family "([^"]*)"$`, evaluationHarnessUsesModelFamily)
	ctx.Step(`^a shared benchmark task and Engram migration are evaluated$`, sharedBenchmarkAndMigrationAreEvaluated)
	ctx.Step(`^the benchmark should preserve the selected model family "([^"]*)"$`, benchmarkShouldPreserveModelFamily)
	ctx.Step(`^the migration should remain harness neutral$`, migrationShouldRemainHarnessNeutral)
}

func evaluationHarnessUsesModelFamily(ctx context.Context, harness, family string) error {
	state, err := getEvaluationControlRouteState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}, harness) {
		return fmt.Errorf("unsupported evaluation harness %q", harness)
	}
	models := map[string]string{
		"anthropic": "claude-sonnet-4.5", "openai": "gpt-5.5", "gemini": "gemini-3.5-flash",
		"glm": "z-ai/glm-5.2", "deepseek": "deepseek/deepseek-v4-pro",
		"nemotron": "nvidia/nemotron-3-ultra", "qwen": "qwen/qwen3.6-max",
	}
	model, ok := models[family]
	if !ok {
		return fmt.Errorf("unsupported evaluation model family %q", family)
	}
	state.harness, state.family, state.model = harness, family, model
	return nil
}

func sharedBenchmarkAndMigrationAreEvaluated(ctx context.Context) error {
	state, err := getEvaluationControlRouteState(ctx)
	if err != nil {
		return err
	}
	state.result, err = (benchmarks.StubExecutor{}).Execute(ctx, benchmarks.TaskSpec{
		ID: "bdd-task", Suite: benchmarks.SuiteVibeBench, Prompt: "validate route",
	}, benchmarks.ModeDearAgent, state.model)
	if err != nil {
		return err
	}
	state.migrated, err = engrammigrate.InsertTierMarkers("# Overview\n\nRoute-neutral migration content.")
	return err
}

func benchmarkShouldPreserveModelFamily(ctx context.Context, family string) error {
	state, err := getEvaluationControlRouteState(ctx)
	if err != nil {
		return err
	}
	if state.family != family || !strings.Contains(state.result.Error, state.model) {
		return fmt.Errorf("benchmark route %s/%s did not preserve model %q", state.harness, family, state.model)
	}
	return nil
}

func migrationShouldRemainHarnessNeutral(ctx context.Context) error {
	state, err := getEvaluationControlRouteState(ctx)
	if err != nil {
		return err
	}
	for _, marker := range []string{"[!T0]", "[!T1]", "[!T2]"} {
		if !strings.Contains(state.migrated, marker) {
			return fmt.Errorf("migration for %s/%s omitted %s", state.harness, state.family, marker)
		}
	}
	if strings.Contains(strings.ToLower(state.migrated), state.harness) {
		return fmt.Errorf("migration embedded harness %s", state.harness)
	}
	return nil
}

func getEvaluationControlRouteState(ctx context.Context) (*evaluationControlRouteState, error) {
	state, ok := ctx.Value(evaluationControlRouteStateKey{}).(*evaluationControlRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("evaluation control route state not initialized")
	}
	return state, nil
}
