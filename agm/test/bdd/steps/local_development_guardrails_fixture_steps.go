package steps

import (
	"context"
	"fmt"
)

func agmRunsAffectedRunnerProcessTreeRegressions(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.treeRegression, state.treeRegressionErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./cmd/test-affected",
		"TestRunGoTestCommandTimeoutKillsProcessGroup",
		"TestListPackagesContextCancellationKillsProcessGroup",
	)
	return nil
}

func boundedAffectedRunnerCommandsShouldTerminateTheirDescendants(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.treeRegressionErr != nil {
		return fmt.Errorf("affected runner process-tree regressions: %w: %s", state.treeRegressionErr, state.treeRegression)
	}
	return nil
}

func agmRunsAffectedRunnerFixtureRegressions(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.treeRegression, state.treeRegressionErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./cmd/test-affected",
		"TestParseProcessPIDs",
		"TestAwaitProcessFixtureWaitsForCompletePIDRecord",
		"TestAwaitProcessFixtureReportsEarlyCommandExit",
		"TestAwaitProcessFixtureTimeoutCancelsAndJoinsProcessGroup",
	)
	return nil
}

func affectedRunnerFixturesShouldDistinguishSetupOutcomes(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.treeRegressionErr != nil {
		return fmt.Errorf("affected runner fixture regressions: %w: %s", state.treeRegressionErr, state.treeRegression)
	}
	return nil
}
