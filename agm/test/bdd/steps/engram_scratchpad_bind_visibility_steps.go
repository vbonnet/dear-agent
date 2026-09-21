package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

type engramScratchpadBindVisibilityStateKey struct{}

type engramScratchpadBindVisibilityState struct {
	output string
	err    error
}

// RegisterEngramScratchpadBindVisibilitySteps registers executable coverage
// for the scratchpad bind-identity construction boundary.
func RegisterEngramScratchpadBindVisibilitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(parent context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(parent, engramScratchpadBindVisibilityStateKey{}, &engramScratchpadBindVisibilityState{}), nil
	})
	ctx.Step(`^AGM runs the deterministic scratchpad bind visibility regressions$`, agmRunsScratchpadBindVisibilityRegressions)
	ctx.Step(`^scratchpad construction should require unchanged bind contents and roll back failed creation$`, scratchpadConstructionShouldRequireVisibleBindAndRollback)
}

func agmRunsScratchpadBindVisibilityRegressions(ctx context.Context) error {
	state, err := getEngramScratchpadBindVisibilityState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx, "./engram/internal/scratchpad",
		"TestNewMountProbePayloadIsUnique",
		"TestNewSandboxAcceptsVisibleBindUnderNonDefaultTempRoot",
		"TestNewSandboxLaunchCancellationRollsBackPreassignedContainer",
		"TestNewSandboxRejectsUnverifiedBindAndRollsBack",
		"TestRollbackSandboxCreationIgnoresCanceledCreationContext",
		"TestRollbackSandboxCreationStopsAtCleanupLimit",
	)
	return nil
}

func scratchpadConstructionShouldRequireVisibleBindAndRollback(ctx context.Context) error {
	state, err := getEngramScratchpadBindVisibilityState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("scratchpad bind visibility regressions: %w\n%s", state.err, state.output)
	}
	return nil
}

func getEngramScratchpadBindVisibilityState(ctx context.Context) (*engramScratchpadBindVisibilityState, error) {
	state, ok := ctx.Value(engramScratchpadBindVisibilityStateKey{}).(*engramScratchpadBindVisibilityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("engram scratchpad bind visibility state not initialized")
	}
	return state, nil
}
