package steps

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

func registerAuthProjectionGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^AGM validates inherited test authentication isolation$`, agmValidatesInheritedAuthProjection)
	ctx.Step(`^approved credential leaves should be exact refreshable links$`, approvedCredentialLeavesAreExactLinks)
	ctx.Step(`^approved configuration leaves should be bounded private snapshots$`, approvedConfigurationLeavesAreBoundedSnapshots)
	ctx.Step(`^missing approved sources should not create provider namespaces$`, missingAuthProjectionSourcesAreSkipped)
	ctx.Step(`^non-allowlisted host provider state should not be projected$`, nonAllowlistedHostProviderStateIsNotProjected)
	ctx.Step(`^changed or unsafe projection inputs should fail before destination mutation$`, changedOrUnsafeAuthProjectionInputsFailBeforeMutation)
	ctx.Step(`^failed projection should remove only nodes created by that attempt$`, failedAuthProjectionRemovesOnlyCreatedNodes)
	ctx.Step(`^synthetic provider onboarding and Codex trust writes should leave host sentinels unchanged$`, selectedHomeMutationsLeaveHostSentinelsUnchanged)
}

func agmValidatesInheritedAuthProjection(ctx context.Context) error {
	state, err := getTestSupportRouteState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(
		testCtx,
		"go", "test",
		"./agm/internal/testcontext",
		"-run", `^TestForwardAuth(Projection|RoutesSelectedHome)`,
		"-count=1",
		"-timeout=90s",
		"-v",
	)
	command.Dir = packageSpecBDDRepoRoot()
	command.SysProcAttr = procguard.ProcessGroupAttr()
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = time.Second
	output, runErr := command.CombinedOutput()
	state.authProjectionOutput = string(output)
	state.authProjectionErr = runErr
	return nil
}

func approvedCredentialLeavesAreExactLinks(ctx context.Context) error {
	return requireAuthProjectionTests(ctx, "TestForwardAuthProjectionExactAllowlist")
}

func approvedConfigurationLeavesAreBoundedSnapshots(ctx context.Context) error {
	return requireAuthProjectionTests(
		ctx,
		"TestForwardAuthProjectionExactAllowlist",
		"TestForwardAuthProjectionEnforcesSnapshotBound",
	)
}

func missingAuthProjectionSourcesAreSkipped(ctx context.Context) error {
	return requireAuthProjectionTests(ctx, "TestForwardAuthProjectionSkipsMissingSources")
}

func nonAllowlistedHostProviderStateIsNotProjected(ctx context.Context) error {
	return requireAuthProjectionTests(ctx, "TestForwardAuthProjectionExactAllowlist")
}

func changedOrUnsafeAuthProjectionInputsFailBeforeMutation(ctx context.Context) error {
	return requireAuthProjectionTests(
		ctx,
		"TestForwardAuthProjectionRejectsChangedSnapshotSource",
		"TestForwardAuthProjectionRejectsUnsafeSourcesBeforeMutation",
		"TestForwardAuthProjectionRejectsUnsafeDestinationsBeforeMutation",
		"TestForwardAuthProjectionRejectsUnsafeRoots",
		"TestForwardAuthProjectionRejectsWrongOwnerMetadata",
		"TestForwardAuthProjectionRejectsAmbientOnboardingActivation",
		"TestForwardAuthProjectionKeepsWritesInsideOpenedSelectedHome",
		"TestForwardAuthProjectionSerializesSelectedHomeTransactions",
	)
}

func failedAuthProjectionRemovesOnlyCreatedNodes(ctx context.Context) error {
	return requireAuthProjectionTests(
		ctx,
		"TestForwardAuthProjectionRollback",
		"TestForwardAuthProjectionRollbackPreservesPreexistingDirectory",
		"TestForwardAuthProjectionRollbackPreservesChangedNodes",
		"TestForwardAuthProjectionReauthenticatesCredentialBeforeLink",
		"TestForwardAuthProjectionCleansUntrackedStagedNodes",
	)
}

func selectedHomeMutationsLeaveHostSentinelsUnchanged(ctx context.Context) error {
	return requireAuthProjectionTests(ctx, "TestForwardAuthRoutesSelectedHomeMutations")
}

func requireAuthProjectionTests(ctx context.Context, tests ...string) error {
	state, err := getTestSupportRouteState(ctx)
	if err != nil {
		return err
	}
	if state.authProjectionErr != nil {
		return fmt.Errorf(
			"inherited authentication projection tests failed: %w\n%s",
			state.authProjectionErr,
			state.authProjectionOutput,
		)
	}
	for _, test := range tests {
		if !strings.Contains(state.authProjectionOutput, "--- PASS: "+test) {
			return fmt.Errorf(
				"inherited authentication projection test %s did not pass:\n%s",
				test,
				state.authProjectionOutput,
			)
		}
	}
	return nil
}
