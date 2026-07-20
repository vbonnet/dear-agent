package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

const sandboxProviderFeaturePath = "agm/test/bdd/features/sandbox_provider_guardrails.feature"

type sandboxProviderGuardrailStateKey struct{}

type sandboxProviderCleanupStateKey struct{}

type sandboxProviderCleanupState struct {
	output string
	err    error
}

// RegisterSandboxProviderGuardrailSteps registers BDD steps for sandbox provider packages.
func RegisterSandboxProviderGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(parent context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(parent, sandboxProviderCleanupStateKey{}, &sandboxProviderCleanupState{}), nil
	})
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          sandboxProviderGuardrailStateKey{},
		label:             "sandbox provider package",
		featurePath:       sandboxProviderFeaturePath,
		configuredPattern: `^sandbox provider package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates sandbox provider package coverage$`,
		colocatedPattern:  `^sandbox provider package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Step(`^AGM runs the sandbox provider cleanup retry regressions$`, agmRunsSandboxProviderCleanupRetryRegressions)
	ctx.Step(`^failed destruction should resume at the unfinished cleanup phase$`, failedSandboxDestructionShouldResumeAtUnfinishedPhase)
}

func agmRunsSandboxProviderCleanupRetryRegressions(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	state.output, state.err = runLocalGuardrailGoTest(ctx,
		`^(TestWorktreeCleanupRetriesOnlyIncompletePhase|TestProvider_DestroyPreservesLockedWorktreeForRetry)$`,
		"./internal/sandbox", "./internal/sandbox/bubblewrap", "./internal/sandbox/gvisor",
	)
	return nil
}

func failedSandboxDestructionShouldResumeAtUnfinishedPhase(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	if state.err != nil {
		return fmt.Errorf("sandbox provider cleanup retry regressions: %w: %s", state.err, state.output)
	}
	return nil
}
