package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

type sandboxOnboardingGuardrailStateKey struct{}

type sandboxOnboardingGuardrailState struct {
	output string
	err    error
}

// RegisterSandboxOnboardingGuardrailSteps registers authenticated onboarding checks.
func RegisterSandboxOnboardingGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(parent context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(parent, sandboxOnboardingGuardrailStateKey{}, &sandboxOnboardingGuardrailState{}), nil
	})
	ctx.Step(`^AGM runs the retained HOME independence regression$`, agmRunsRetainedHomeIndependenceRegression)
	ctx.Step(`^ambient HOME drift should not redirect onboarding output$`, ambientHomeDriftShouldNotRedirectOnboardingOutput)
	ctx.Step(`^AGM runs the hostile onboarding namespace regressions$`, agmRunsHostileOnboardingNamespaceRegressions)
	ctx.Step(`^retained HOME output should reject hostile namespace entries without a skipped proof$`, retainedHomeOutputShouldRejectHostileEntries)
	ctx.Step(`^AGM runs the onboarding rollback command regression$`, agmRunsOnboardingRollbackCommandRegression)
	ctx.Step(`^onboarding failure should preserve external paths and destroy the exact provider sandbox once with bounded cancellation-independent cleanup$`, onboardingFailureShouldRollBackProvider)
	ctx.Step(`^AGM runs the onboarding cleanup-error command regression$`, agmRunsOnboardingCleanupErrorCommandRegression)
	ctx.Step(`^provider cleanup failure should remain joined with onboarding failure$`, providerCleanupFailureShouldRemainJoined)
}

func agmRunsRetainedHomeIndependenceRegression(ctx context.Context) error {
	state, err := getSandboxOnboardingGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx, "./agm/internal/sandboxonboarding",
		"TestInstallIgnoresAmbientHomeDrift",
	)
	return nil
}

func ambientHomeDriftShouldNotRedirectOnboardingOutput(ctx context.Context) error {
	return requireSandboxOnboardingRegressionPasses(ctx, "retained HOME independence",
		"TestInstallIgnoresAmbientHomeDrift",
	)
}

func agmRunsHostileOnboardingNamespaceRegressions(ctx context.Context) error {
	state, err := getSandboxOnboardingGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx, "./agm/internal/sandboxonboarding",
		"TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation",
		"TestInstallRejectsFIFOOutputDirectoriesWithoutExternalMutation",
		"TestInstallRejectsStagedTemporaryReplacementAndPreservesIt",
	)
	return nil
}

func retainedHomeOutputShouldRejectHostileEntries(ctx context.Context) error {
	return requireSandboxOnboardingRegressionPasses(ctx, "hostile onboarding namespace",
		"TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation",
		"TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation/.claude",
		"TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation/projects",
		"TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation/encoded_project_directory",
		"TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation/final_CLAUDE.md",
		"TestInstallRejectsFIFOOutputDirectoriesWithoutExternalMutation",
		"TestInstallRejectsFIFOOutputDirectoriesWithoutExternalMutation/.claude",
		"TestInstallRejectsFIFOOutputDirectoriesWithoutExternalMutation/projects",
		"TestInstallRejectsFIFOOutputDirectoriesWithoutExternalMutation/encoded_project_directory",
		"TestInstallRejectsStagedTemporaryReplacementAndPreservesIt",
	)
}

func agmRunsOnboardingRollbackCommandRegression(ctx context.Context) error {
	state, err := getSandboxOnboardingGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx, "./agm/cmd/agm",
		"TestProvisionSandboxRollsBackUnsafeOnboardingWithoutExternalMutation",
	)
	return nil
}

func onboardingFailureShouldRollBackProvider(ctx context.Context) error {
	return requireSandboxOnboardingRegressionPasses(ctx, "onboarding rollback command",
		"TestProvisionSandboxRollsBackUnsafeOnboardingWithoutExternalMutation",
	)
}

func agmRunsOnboardingCleanupErrorCommandRegression(ctx context.Context) error {
	state, err := getSandboxOnboardingGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx, "./agm/cmd/agm",
		"TestProvisionSandboxJoinsOnboardingAndProviderCleanupErrors",
	)
	return nil
}

func providerCleanupFailureShouldRemainJoined(ctx context.Context) error {
	return requireSandboxOnboardingRegressionPasses(ctx, "onboarding cleanup-error command",
		"TestProvisionSandboxJoinsOnboardingAndProviderCleanupErrors",
	)
}

func requireSandboxOnboardingRegressionPasses(ctx context.Context, label string, names ...string) error {
	state, err := getSandboxOnboardingGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("%s regressions: %w: %s", label, state.err, state.output)
	}
	return requireUnskippedGoTestPasses(label, state.output, names...)
}

func getSandboxOnboardingGuardrailState(ctx context.Context) (*sandboxOnboardingGuardrailState, error) {
	state, ok := ctx.Value(sandboxOnboardingGuardrailStateKey{}).(*sandboxOnboardingGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("sandbox onboarding guardrail state not initialized")
	}
	return state, nil
}

func requireUnskippedGoTestPasses(label, output string, names ...string) error {
	passed := make(map[string]struct{}, len(names))
	for rawLine := range strings.SplitSeq(output, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if skipped, ok := strings.CutPrefix(line, "--- SKIP: "); ok {
			name, _, _ := strings.Cut(skipped, " ")
			return fmt.Errorf("%s proof skipped selected regression %s:\n%s", label, name, output)
		}
		if passing, ok := strings.CutPrefix(line, "--- PASS: "); ok {
			name, _, _ := strings.Cut(passing, " ")
			passed[name] = struct{}{}
		}
	}
	for _, name := range names {
		if _, ok := passed[name]; !ok {
			return fmt.Errorf("%s proof did not pass %s:\n%s", label, name, output)
		}
	}
	return nil
}
