package steps

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

const (
	specAuditGoTestDeadline    = 2 * time.Minute
	specAuditGoTestTimeout     = "90s"
	specAuditGoTestOutputLimit = 1 << 20
)

type specGovernanceToolingStateKey struct{}

type specGovernanceToolingState struct {
	repoRoot string
	command  string
	output   string
	err      error
}

// RegisterSpecGovernanceToolingSteps registers focused SPEC governance tool checks.
func RegisterSpecGovernanceToolingSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		state := &specGovernanceToolingState{repoRoot: packageSpecBDDRepoRoot()}
		return context.WithValue(ctx, specGovernanceToolingStateKey{}, state), nil
	})
	ctx.Step(`^AGM runs the focused pinned SPEC inventory unit check$`, exercisePinnedSPECInventory)
	ctx.Step(`^AGM runs the focused non-verdict SPEC audit lead unit check$`, exerciseNonVerdictSPECAuditLeads)
	ctx.Step(`^AGM runs the focused reciprocal SPEC and BDD diagnostic unit check$`, exerciseReciprocalSPECBDDDiagnostics)
	ctx.Step(`^AGM runs the focused pinned finding validation unit check$`, exercisePinnedSPECFindingValidation)
	ctx.Step(`^AGM runs the focused v2 inventory and ledger unit check$`, exerciseV2InventoryAndLedger)
	ctx.Step(`^AGM runs the focused SPEC audit platform applicability unit check$`, exerciseSPECAuditPlatformApplicability)
	ctx.Step(`^AGM runs the focused bounded offline rendering unit check$`, exerciseBoundedOfflineSPECAuditRendering)
	ctx.Step(`^AGM runs the focused read-only audit boundary unit check$`, exerciseReadOnlySPECAuditBoundary)
	ctx.Step(`^AGM runs the focused skill projection safety unit check$`, exerciseSkillProjectionSafety)
	ctx.Step(`^the focused SPEC governance unit check should pass$`, specGovernanceBehaviorShouldPass)
}

func exercisePinnedSPECInventory(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically")
}

func exerciseNonVerdictSPECAuditLeads(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically")
}

func exerciseReciprocalSPECBDDDiagnostics(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryReportsFeatureFirstDiagnosticsFromPinnedObjects")
}

func exercisePinnedSPECFindingValidation(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "^(TestValidatePinsFindingsToGitResolvedInventory|TestPinnedValidationRejectsForgedEvidenceAndUnsafeVerdicts|TestValidateReportRejectsHarnessRegistrationProposedOwners|TestHarnessRegistrationCurrentOwnersRequireRetirement|TestHarnessRegistrationRootsIncludeAliasesAndFutureGroupedHarnesses|TestReportRejectsConflictingCrossFindingRequirementMappings|TestNC005PlannedBDDTransfersSupportMixedHarnessOwnerRetirement|TestOwnershipPlanSeparatesSelectedRetirementFromResidualRecords|TestSelectedRetirementMayPreserveSelectedReciprocalFeatureForResidualContract|TestPlannedBDDTransferValidationFailsClosed|TestFullRetirementRequiresEveryRecordSelected|TestExistingProposedOwnerMustBeCurrent|TestSupportingEvidenceBudgetCountsPlannedBDDTransfers|TestNewProposedOwnerDirectoryContainmentUsesPathComponents|TestNewProposedOwnerDirectoryRuleSurvivesPinnedLedgerRoundTrip|TestOwnershipPlanRequiresEveryRequirementAndPerOwnerBDDLink|TestOwnershipPlanCopiesApplicabilityAndPendingDecisionStatus|TestReviewerExclusionsResolveAndCannotSelectProposedOwner)$")
}

func exerciseV2InventoryAndLedger(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestV2InventoryAndDecisionLedger")
}

func exerciseSPECAuditPlatformApplicability(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestValidateAndRenderPlatformApplicability|TestPinnedInventoryValidationTreatsCollectorExecutionAsNonAttesting")
}

func exerciseBoundedOfflineSPECAuditRendering(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestRenderIsOfflineAndEscapesEvidence|TestReportInputsAndArtifactsAreBounded|TestEscapedHTMLStopsAtArtifactLimit")
}

func exerciseReadOnlySPECAuditBoundary(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryValidateRenderPreserveTargetRepositoryState")
}

func exerciseSkillProjectionSafety(ctx context.Context) error {
	return runFocusedGoTests(
		ctx,
		"./tools/skill-projections",
		"^Test",
	)
}

func runSpecAuditGoTests(ctx context.Context, pattern string) error {
	return runFocusedGoTests(ctx, "./tools/specaudit", pattern)
}

func runFocusedGoTests(ctx context.Context, packagePath, pattern string) error {
	state, err := getSpecGovernanceToolingState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, specAuditGoTestDeadline)
	defer cancel()

	command := newFocusedGoTestCommand(testCtx, state.repoRoot, packagePath, pattern)
	state.command = strings.Join(command.Args, " ")
	output := &boundedSpecAuditOutput{limit: specAuditGoTestOutputLimit}
	command.Stdout = output
	command.Stderr = output
	commandErr := command.Run()
	state.output = output.String()
	switch {
	case testCtx.Err() != nil:
		state.err = fmt.Errorf("%s did not complete within %s: %w", state.command, specAuditGoTestDeadline, testCtx.Err())
	case output.Truncated():
		state.err = fmt.Errorf("%s output exceeded %d-byte safety limit", state.command, specAuditGoTestOutputLimit)
	case commandErr != nil:
		state.err = commandErr
	case !strings.Contains(state.output, "=== RUN"):
		state.err = fmt.Errorf("%s pattern %q did not run a named regression", state.command, pattern)
	}
	return nil
}

func newSpecAuditGoTestCommand(ctx context.Context, repoRoot, pattern string) *exec.Cmd {
	return newFocusedGoTestCommand(ctx, repoRoot, "./tools/specaudit", pattern)
}

func newFocusedGoTestCommand(ctx context.Context, repoRoot, packagePath, pattern string) *exec.Cmd {
	arguments := []string{"test", "-v", "-count=1", "-timeout=" + specAuditGoTestTimeout, packagePath, "-run", pattern}
	command := exec.CommandContext(ctx, "go", arguments...)
	command.Dir = repoRoot
	command.SysProcAttr = procguard.ProcessGroupAttr()
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = time.Second
	return command
}

// boundedSpecAuditOutput caps nested go-test output while allowing the command
// to complete so its normal error remains available to the BDD failure.
type boundedSpecAuditOutput struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (output *boundedSpecAuditOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	originalLen := len(data)
	remaining := output.limit - output.buffer.Len()
	if remaining <= 0 {
		output.truncated = true
		return originalLen, nil
	}
	if len(data) > remaining {
		output.truncated = true
		data = data[:remaining]
	}
	_, err := output.buffer.Write(data)
	return originalLen, err
}

func (output *boundedSpecAuditOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

func (output *boundedSpecAuditOutput) Truncated() bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.truncated
}

func specGovernanceBehaviorShouldPass(ctx context.Context) error {
	state, err := getSpecGovernanceToolingState(ctx)
	if err != nil {
		return err
	}
	if state.command == "" {
		return fmt.Errorf("SPEC governance command was not selected")
	}
	if state.err != nil {
		return fmt.Errorf("%s failed: %w\n%s", state.command, state.err, state.output)
	}
	return nil
}

func getSpecGovernanceToolingState(ctx context.Context) (*specGovernanceToolingState, error) {
	state, ok := ctx.Value(specGovernanceToolingStateKey{}).(*specGovernanceToolingState)
	if !ok || state == nil {
		return nil, fmt.Errorf("SPEC governance tooling state not initialized")
	}
	return state, nil
}
