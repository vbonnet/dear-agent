package steps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

type agmSupervisionRecoveryGuardrailStateKey struct{}
type sentinelTmuxIsolationStateKey struct{}
type noChecksProviderCompletenessStateKey struct{}

type sentinelTmuxIsolationState struct {
	output string
	err    error
}

type noChecksProviderCompletenessState struct {
	output string
	err    error
}

// RegisterAGMSupervisionRecoveryGuardrailSteps registers supervision package coverage steps.
func RegisterAGMSupervisionRecoveryGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmSupervisionRecoveryGuardrailStateKey{},
		label:             "AGM supervision package",
		featurePath:       "agm/test/bdd/features/agm_supervision_recovery_guardrails.feature",
		configuredPattern: `^AGM supervision package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates supervision package coverage$`,
		colocatedPattern:  `^AGM supervision package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		ctx = context.WithValue(ctx, sentinelTmuxIsolationStateKey{}, &sentinelTmuxIsolationState{})
		return context.WithValue(ctx, noChecksProviderCompletenessStateKey{}, &noChecksProviderCompletenessState{}), nil
	})
	ctx.Step(`^sentinel monitoring owns an explicit tmux socket$`, sentinelMonitoringOwnsAnExplicitTmuxSocket)
	ctx.Step(`^AGM validates sentinel tmux isolation$`, agmValidatesSentinelTmuxIsolation)
	ctx.Step(`^sentinel discovery should use only the configured socket$`, sentinelDiscoveryShouldUseOnlyTheConfiguredSocket)
	ctx.Step(`^nested AGM recovery commands should inherit the configured socket$`, nestedAGMRecoveryCommandsShouldInheritTheConfiguredSocket)
	ctx.Step(`^sentinel lifecycle tests should not inspect ambient tmux sessions$`, sentinelLifecycleTestsShouldNotInspectAmbientTmuxSessions)
	ctx.Step(`^no-check recovery can mutate a pull request branch$`, noCheckRecoveryCanMutateAPullRequestBranch)
	ctx.Step(`^AGM validates no-check provider completeness$`, agmValidatesNoCheckProviderCompleteness)
	ctx.Step(`^required-check policy should use the shared layered owner$`, requiredCheckPolicyShouldUseTheSharedLayeredOwner)
	ctx.Step(`^check-run reads should consume every provider page$`, checkRunReadsShouldConsumeEveryProviderPage)
	ctx.Step(`^policy failures should prevent trigger calls$`, policyFailuresShouldPreventTriggerCalls)
	ctx.Step(`^unreadable check runs should remain indeterminate$`, unreadableCheckRunsShouldRemainIndeterminate)
	ctx.Step(`^no-check recovery scans pull requests across bases$`, noCheckRecoveryScansPullRequestsAcrossBases)
	ctx.Step(`^each non-draft pull request should use its actual base policy$`, eachNonDraftPullRequestShouldUseItsActualBasePolicy)
	ctx.Step(`^branch selection should be an optional verified filter$`, branchSelectionShouldBeAnOptionalVerifiedFilter)
	ctx.Step(`^every non-draft base policy should preflight before check-run reads$`, everyNonDraftBasePolicyShouldPreflightBeforeCheckRunReads)
	ctx.Step(`^policy preflight should use one total deadline$`, policyPreflightShouldUseOneTotalDeadline)
	ctx.Step(`^draft pull requests should require no policy or check-run reads$`, draftPullRequestsShouldRequireNoPolicyOrCheckRunReads)
	ctx.Step(`^pull request listings should require known draft state$`, pullRequestListingsShouldRequireKnownDraftState)
	ctx.Step(`^pull request listings should honor a positive operator limit$`, pullRequestListingsShouldHonorAPositiveOperatorLimit)
	ctx.Step(`^draft output should distinguish listed from eligible pull requests$`, draftOutputShouldDistinguishListedFromEligiblePullRequests)
	ctx.Step(`^scan output should report the explicit base filter$`, scanOutputShouldReportTheExplicitBaseFilter)
	ctx.Step(`^stuck evidence should report the actual pull request base$`, stuckEvidenceShouldReportTheActualPullRequestBase)
	ctx.Step(`^retrigger should revalidate current pull request identity$`, retriggerShouldRevalidateCurrentPullRequestIdentity)
	ctx.Step(`^stale or forked retriggers should stop before mutation$`, staleOrForkedRetriggersShouldStopBeforeMutation)
	ctx.Step(`^retrigger should recheck whether CI already appeared$`, retriggerShouldRecheckWhetherCIAlreadyAppeared)
	ctx.Step(`^caller cancellation should stop later trigger calls$`, callerCancellationShouldStopLaterTriggerCalls)
	ctx.Step(`^retrigger dry-run should validate without mutation$`, retriggerDryRunShouldValidateWithoutMutation)
	ctx.Step(`^trigger documentation should preserve snapshot boundaries$`, triggerDocumentationShouldPreserveSnapshotBoundaries)
}

func noCheckRecoveryCanMutateAPullRequestBranch() error {
	spec, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "nochecks", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("read no-check recovery SPEC: %w", err)
	}
	if !strings.Contains(string(spec), "**NCK-04**") {
		return fmt.Errorf("no-check recovery SPEC does not define branch-targeted retriggering")
	}
	return nil
}

func noCheckRecoveryScansPullRequestsAcrossBases() error {
	spec, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "nochecks", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("read no-check recovery SPEC: %w", err)
	}
	for _, requirement := range []string{
		"**NCK-09**", "**NCK-10**", "**NCK-11**",
		"**NCK-12**", "**NCK-13**", "**NCK-14**", "**NCK-15**", "**NCK-16**",
		"**NCK-17**", "**NCK-18**", "**NCK-19**", "**NCK-20**", "**NCK-21**", "**NCK-22**", "**NCK-23**",
	} {
		if !strings.Contains(string(spec), requirement) {
			return fmt.Errorf("no-check recovery SPEC does not contain %s", requirement)
		}
	}
	return nil
}

func agmValidatesNoCheckProviderCompleteness(ctx context.Context) error {
	state, err := getNoChecksProviderCompletenessState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "-v", "-count=1", "-timeout=90s",
		"-run", `^Test(GHJSONContext|RequiredCheckNamesForBranch|FetchRequiredChecks|ResolveRequiredChecksByBase|ListOpenPRs|CheckRunNamesForRef|Scan_|RunPRScanNoChecks|PRScanNoChecksBranchFlag|PRScanNoChecksHelp|NoChecksScanResultJSON|PrintNoChecksScanText|ValidateRetriggerSnapshot|RetriggerCI).*$`,
		"./internal/safegit", "./agm/internal/nochecks", "./agm/cmd/agm")
	cmd.Dir = packageSpecBDDRepoRoot()
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	output, runErr := cmd.CombinedOutput()
	state.output = string(output)
	state.err = runErr
	if errors.Is(testCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("no-check provider completeness suite timed out: %w", testCtx.Err())
	}
	return nil
}

func requiredCheckPolicyShouldUseTheSharedLayeredOwner(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	for path, requirement := range map[string]string{
		filepath.Join("internal", "safegit", "SPEC.md"):         "**SAFEGIT-31**",
		filepath.Join("agm", "internal", "nochecks", "SPEC.md"): "**NCK-06**",
	} {
		spec, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), path))
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if !strings.Contains(string(spec), requirement) {
			return fmt.Errorf("%s does not contain %s", path, requirement)
		}
	}
	return nil
}

func checkRunReadsShouldConsumeEveryProviderPage(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestCheckRunNamesForRefRequestsEveryPage",
		"TestCheckRunNamesForRefDiscardsPartialOutputOnFailure",
	)
}

func unreadableCheckRunsShouldRemainIndeterminate(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestRunPRScanNoChecksReadErrorRemainsIndeterminate",
		"TestRunPRScanNoChecksJSONReadErrorRemainsStructuredAndIndeterminate",
	)
}

func policyFailuresShouldPreventTriggerCalls(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx, "TestRunPRScanNoChecksPolicyErrorStopsBeforeTrigger")
}

func eachNonDraftPullRequestShouldUseItsActualBasePolicy(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestListOpenPRsCarriesBaseAndOmitsEmptyFilter",
		"TestResolveRequiredChecksByBaseFetchesSortedDistinctBasesOnce",
		"TestScan_UsesEachPullRequestBasePolicy",
	)
}

func branchSelectionShouldBeAnOptionalVerifiedFilter(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestListOpenPRsAppliesAndVerifiesExplicitBaseFilter",
		"TestListOpenPRsRejectsRowOutsideExplicitBaseFilter",
		"TestPRScanNoChecksBranchFlagIsOptionalFilter",
	)
}

func everyNonDraftBasePolicyShouldPreflightBeforeCheckRunReads(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestResolveRequiredChecksByBaseValidatesEveryCandidateBeforeFetching",
		"TestResolveRequiredChecksByBaseRejectsMissingNonDraftBaseWithoutFetching",
		"TestResolveRequiredChecksByBaseLaterFailureReturnsUnusableOwner",
		"TestScan_RejectsUninitializedOrMissingBasePolicyBeforeRunReads",
		"TestRunPRScanNoChecksSecondBasePolicyErrorStopsBeforeTrigger",
	)
}

func draftPullRequestsShouldRequireNoPolicyOrCheckRunReads(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestResolveRequiredChecksByBaseIgnoresDraftWithMissingBase",
		"TestResolveRequiredChecksByBaseIgnoresDraftBaseAlongsideCandidate",
		"TestScan_DraftsExcluded",
	)
}

func pullRequestListingsShouldRequireKnownDraftState(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx, "TestListOpenPRsRejectsUnknownDraftState")
}

func pullRequestListingsShouldHonorAPositiveOperatorLimit(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestListOpenPRsCarriesBaseAndOmitsEmptyFilter",
		"TestRunPRScanNoChecksRejectsNonPositiveLimitBeforeProviderCalls",
	)
}

func policyPreflightShouldUseOneTotalDeadline(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestResolveRequiredChecksByBaseSharesOneDeadlineAcrossBases",
		"TestFetchRequiredChecksByBaseWithinOwnsOneTotalDeadline",
		"TestResolveRequiredChecksByBaseStopsBeforeNextFetchAfterCancellation",
	)
}

func draftOutputShouldDistinguishListedFromEligiblePullRequests(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx, "TestRunPRScanNoChecksDraftOnlyReportsListedNotScanned")
}

func scanOutputShouldReportTheExplicitBaseFilter(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx, "TestNoChecksScanResultJSONCarriesExplicitBaseEvidence")
}

func stuckEvidenceShouldReportTheActualPullRequestBase(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestNoChecksScanResultJSONCarriesExplicitBaseEvidence",
		"TestPrintNoChecksScanTextCarriesBaseEvidence",
	)
}

func retriggerShouldRevalidateCurrentPullRequestIdentity(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestValidateRetriggerSnapshot",
		"TestRetriggerCIValidSnapshotPreservesMutationOrder",
	)
}

func staleOrForkedRetriggersShouldStopBeforeMutation(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestRetriggerCIDriftStopsBeforeCommitOrRefCalls",
		"TestRetriggerCIPreflightReadFailuresStopBeforeMutation",
		"TestRunPRScanNoChecksDryRunRevalidatesAndContinuesAfterDrift",
	)
}

func retriggerShouldRecheckWhetherCIAlreadyAppeared(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestScan_CapturesAnIsolatedCloneOfTheClassifyingPolicy",
		"TestRetriggerCISelfHealedChecksStopBeforeMutation",
		"TestRunPRScanNoChecksSelfHealedCandidateSucceedsWithoutMutation",
	)
}

func callerCancellationShouldStopLaterTriggerCalls(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestGHJSONContextReturnsPreCanceledCallerBeforeExecutableLookup",
		"TestRetriggerCICallerCancellationStopsProviderSequence",
		"TestRunPRScanNoChecksCancellationStopsLaterRetriggers",
	)
}

func retriggerDryRunShouldValidateWithoutMutation(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx,
		"TestRetriggerCIDryRunRevalidatesWithoutMutation",
		"TestRunPRScanNoChecksRejectsDryRunWithoutTriggerBeforeProviderCalls",
	)
}

func triggerDocumentationShouldPreserveSnapshotBoundaries(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx, "TestPRScanNoChecksHelpDescribesSnapshotBoundary")
}

func requireNoChecksProviderCompletenessBehavior(ctx context.Context) error {
	state, err := getNoChecksProviderCompletenessState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("no-check provider completeness suite failed: %w\n%s", state.err, state.output)
	}
	return nil
}

func requireNoChecksTestOutput(ctx context.Context, tests ...string) error {
	state, err := getNoChecksProviderCompletenessState(ctx)
	if err != nil {
		return err
	}
	for _, test := range tests {
		if !strings.Contains(state.output, "--- PASS: "+test) {
			return fmt.Errorf("no-check provider completeness behavior %s did not pass:\n%s", test, state.output)
		}
	}
	return nil
}

func getNoChecksProviderCompletenessState(ctx context.Context) (*noChecksProviderCompletenessState, error) {
	state, ok := ctx.Value(noChecksProviderCompletenessStateKey{}).(*noChecksProviderCompletenessState)
	if !ok || state == nil {
		return nil, fmt.Errorf("no-check provider completeness state not initialized")
	}
	return state, nil
}

func sentinelMonitoringOwnsAnExplicitTmuxSocket() error {
	path := filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "sentinel", "daemon", "SPEC.md")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("locate sentinel daemon SPEC: %w", err)
	}
	return nil
}

func agmValidatesSentinelTmuxIsolation(ctx context.Context) error {
	state, err := getSentinelTmuxIsolationState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test",
		"./agm/internal/sentinel/tmux", "./agm/internal/sentinel/daemon",
		"-run", `^Test(NewClientWithSocketUsesOnlyConfiguredSocket|ConfiguredClientActionsUseOnlyConfiguredSocket|NewSessionMonitor(UsesOnlyConfiguredTmuxSocket|PropagatesConfiguredSocketToNestedCommands)|MonitorStability)$`,
		"-count=1", "-v")
	cmd.Dir = packageSpecBDDRepoRoot()
	output, runErr := cmd.CombinedOutput()
	state.output = string(output)
	state.err = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("sentinel tmux isolation behavior suite timed out: %w", testCtx.Err())
	}
	return nil
}

func sentinelDiscoveryShouldUseOnlyTheConfiguredSocket(ctx context.Context) error {
	if err := requireSentinelTmuxIsolationBehavior(ctx); err != nil {
		return err
	}
	spec, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "sentinel", "daemon", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("read sentinel daemon SPEC: %w", err)
	}
	if !strings.Contains(string(spec), "**SENTD-07**") {
		return fmt.Errorf("sentinel daemon SPEC does not require configured socket isolation")
	}
	return nil
}

func sentinelLifecycleTestsShouldNotInspectAmbientTmuxSessions(ctx context.Context) error {
	return requireSentinelTmuxIsolationBehavior(ctx)
}

func nestedAGMRecoveryCommandsShouldInheritTheConfiguredSocket(ctx context.Context) error {
	return requireSentinelTmuxIsolationBehavior(ctx)
}

func requireSentinelTmuxIsolationBehavior(ctx context.Context) error {
	state, err := getSentinelTmuxIsolationState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("sentinel tmux isolation behavior suite failed: %w\n%s", state.err, state.output)
	}
	for _, behavior := range []string{
		"TestNewClientWithSocketUsesOnlyConfiguredSocket",
		"TestConfiguredClientActionsUseOnlyConfiguredSocket",
		"TestNewSessionMonitorUsesOnlyConfiguredTmuxSocket",
		"TestNewSessionMonitorPropagatesConfiguredSocketToNestedCommands",
		"TestMonitorStability",
	} {
		if !strings.Contains(state.output, "--- PASS: "+behavior) {
			return fmt.Errorf("sentinel tmux isolation behavior %s did not pass:\n%s", behavior, state.output)
		}
	}
	return nil
}

func getSentinelTmuxIsolationState(ctx context.Context) (*sentinelTmuxIsolationState, error) {
	state, ok := ctx.Value(sentinelTmuxIsolationStateKey{}).(*sentinelTmuxIsolationState)
	if !ok || state == nil {
		return nil, fmt.Errorf("sentinel tmux isolation behavior state not initialized")
	}
	return state, nil
}
