package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

// registerSafePRRegressionGuardrailSteps keeps the executable safe-pr
// regression bridge cohesive instead of growing the general local-development
// guardrail step file.
func registerSafePRRegressionGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^AGM runs the safe-pr abrupt-parent regression$`, agmRunsSafePRAbruptParentRegression)
	ctx.Step(`^the child should retain transaction ownership until it exits$`, childShouldRetainTransactionOwnershipUntilExit)
	ctx.Step(`^AGM runs the safe-pr final transaction audit regression$`, agmRunsSafePRFinalTransactionAuditRegression)
	ctx.Step(`^each safe-pr transaction should have one accurate audit record$`, eachSafePRTransactionShouldHaveOneAccurateAuditRecord)
	ctx.Step(`^AGM runs the safe-pr no-merge subprocess regression$`, agmRunsSafePRNoMergeSubprocessRegression)
	ctx.Step(`^safe-pr creation should not invoke a merge subprocess$`, safePRCreationShouldNotInvokeMergeSubprocess)
}

func agmRunsSafePRAbruptParentRegression(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.childRegression, state.childRegressionErr = runLocalGuardrailGoTest(ctx,
		`^TestWorktreeTransactionLockOutlivesKilledParentFor(ProtectedChild|GitHelper)$`,
		"./internal/safepr",
	)
	return nil
}

func childShouldRetainTransactionOwnershipUntilExit(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.childRegressionErr != nil {
		return fmt.Errorf("safe-pr abrupt-parent regression: %w: %s", state.childRegressionErr, state.childRegression)
	}
	return nil
}

func agmRunsSafePRFinalTransactionAuditRegression(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.auditRegression, state.auditRegressionErr = runLocalGuardrailGoTest(ctx,
		`^TestRun_CreateAuditsFinalTransactionOutcome$`,
		"./cmd/safe-pr",
	)
	return nil
}

func eachSafePRTransactionShouldHaveOneAccurateAuditRecord(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.auditRegressionErr != nil {
		return fmt.Errorf("safe-pr final transaction audit regression: %w: %s", state.auditRegressionErr, state.auditRegression)
	}
	return nil
}

func agmRunsSafePRNoMergeSubprocessRegression(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.noMergeRegression, state.noMergeRegressionErr = runLocalGuardrailNamedGoTests(ctx,
		"./cmd/safe-pr",
		"TestExecGh_CreateNeverMutatesMergeState",
	)
	return nil
}

func safePRCreationShouldNotInvokeMergeSubprocess(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.noMergeRegressionErr != nil {
		return fmt.Errorf("safe-pr no-merge subprocess regression: %w: %s", state.noMergeRegressionErr, state.noMergeRegression)
	}
	return nil
}
