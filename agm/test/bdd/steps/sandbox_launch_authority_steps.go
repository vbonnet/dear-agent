package steps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/fsguard"
)

type sandboxLaunchAuthorityStateKey struct{}

type sandboxLaunchAuthorityState struct {
	tempDir             string
	fakeHome            string
	configuredWorkspace string
	policy              fsguard.Policy
	decisionAllowed     bool
	suiteOutput         string
	suiteErr            error
	selectionMarker     string
}

type sandboxLaunchSelectionResult struct {
	output string
	err    error
}

var (
	sandboxLaunchSelectionOnce   sync.Once
	sandboxLaunchSelectionCached sandboxLaunchSelectionResult
)

// RegisterSandboxLaunchAuthoritySteps registers the configured sandbox
// authority behavior shared by create, cold resume, FSGUARD, and disk admission.
func RegisterSandboxLaunchAuthoritySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(parent context.Context, _ *godog.Scenario) (context.Context, error) {
		tempDir, err := os.MkdirTemp("", "agm-bdd-sandbox-launch-authority-")
		if err != nil {
			return parent, fmt.Errorf("create sandbox launch authority fixture: %w", err)
		}
		fakeHome := filepath.Join(string(filepath.Separator), "agm-bdd-"+filepath.Base(tempDir), "home")
		configuredWorkspace := filepath.Join(fakeHome, ".configured-sandboxes", "stable-agm-session")
		state := &sandboxLaunchAuthorityState{
			tempDir:             tempDir,
			fakeHome:            fakeHome,
			configuredWorkspace: configuredWorkspace,
		}
		return context.WithValue(parent, sandboxLaunchAuthorityStateKey{}, state), nil
	})
	ctx.After(func(parent context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		state, err := sandboxLaunchAuthorityStateFrom(parent)
		if err != nil || state.tempDir == "" {
			return parent, nil
		}
		if removeErr := os.RemoveAll(state.tempDir); removeErr != nil {
			return parent, fmt.Errorf("remove sandbox launch authority fixture: %w", removeErr)
		}
		return parent, nil
	})

	ctx.Step(`^AGM validates sandbox launch selection for "([^"]*)" with "([^"]*)"$`, agmValidatesSandboxLaunchSelection)
	ctx.Step(`^the configured parent and stable AGM session child should reach the launch boundary$`, configuredParentAndStableSessionChildShouldReachLaunchBoundary)
	ctx.Step(`^persisted or effective launch paths outside that child should be refused$`, persistedOrEffectiveLaunchPathsOutsideChildShouldBeRefused)
	ctx.Step(`^APFS merged-directory symlinks within that child should remain valid for create and cold resume$`, apfsMergedDirectorySymlinksShouldRemainValidForCreateAndColdResume)
	ctx.Step(`^AGM validates sandbox provider output containment$`, agmValidatesSandboxProviderOutputContainment)
	ctx.Step(`^malicious provider paths should be refused before outside onboarding or permission writes and cleaned by stable AGM session ID$`, maliciousProviderPathsShouldBeRefusedBeforeWritesAndCleaned)
	ctx.Step(`^unmaterialized provider paths should be refused before host directories are created$`, unmaterializedProviderPathsShouldBeRefusedBeforeHostDirectoriesAreCreated)
	ctx.Step(`^later create failures should clean through the original provider instance with an uncanceled context$`, laterCreateFailuresShouldCleanThroughOriginalProviderInstance)
	ctx.Step(`^contained provider symlinks should preserve the exact durable cleanup boundary while supplying a physical live and onboarding path$`, containedProviderSymlinksShouldPreserveDurableOwnershipSpelling)
	ctx.Step(`^AGM validates fresh and cold-resume sandbox containment$`, agmValidatesFreshAndColdResumeSandboxContainment)
	ctx.Step(`^fresh creation should refuse an outside prepared working directory before remote or tmux mutation$`, freshCreationShouldRefuseOutsideWorkDirBeforeMutation)
	ctx.Step(`^cold resume should refuse persisted path drift or missing effective directories before tmux creation$`, coldResumeShouldRefusePathDriftOrMissingDirectoriesBeforeTmuxCreation)
	ctx.Step(`^FSGUARD uses "([^"]*)" sandbox workspace authority$`, fsGuardUsesSandboxWorkspaceAuthority)
	ctx.Step(`^FSGUARD classifies the "([^"]*)" target$`, fsGuardClassifiesTarget)
	ctx.Step(`^the sandbox workspace decision should be "([^"]*)"$`, sandboxWorkspaceDecisionShouldBe)
	ctx.Step(`^AGM validates invalid sandbox workspace authority$`, agmValidatesInvalidSandboxWorkspaceAuthority)
	ctx.Step(`^invalid sandbox workspace authority should leave no default allowance$`, invalidSandboxWorkspaceAuthorityShouldLeaveNoDefaultAllowance)
	ctx.Step(`^AGM validates writable-root symlink resolution$`, agmValidatesWritableRootSymlinkResolution)
	ctx.Step(`^dangling symlink targets should be denied while ordinary nonexistent descendants remain allowed$`, danglingSymlinksShouldBeDeniedWhileOrdinaryDescendantsRemainAllowed)
	ctx.Step(`^AGM validates exact sandbox workspace escape resistance$`, agmValidatesExactSandboxWorkspaceEscapeResistance)
	ctx.Step(`^symlink parent traversal and unsupported tilde expansion should be denied$`, symlinkParentTraversalAndUnsupportedTildeExpansionShouldBeDenied)
	ctx.Step(`^exact workspace authority should shadow generic writable parents for sibling paths$`, exactWorkspaceAuthorityShouldShadowWritableParents)
	ctx.Step(`^AGM validates private sandbox authority handoffs$`, agmValidatesPrivateSandboxAuthorityHandoffs)
	ctx.Step(`^Claude and Codex should consume exact authority without FSGUARD assignments, authority flags, or the stale inherited value in command text$`, claudeAndCodexShouldConsumeExactAuthorityWithoutCommandAuthorityEncoding)
	ctx.Step(`^AGM validates ordinary Codex handoff argument binding$`, agmValidatesOrdinaryCodexHandoffArgumentBinding)
	ctx.Step(`^changed ordinary Codex launch arguments should be refused without requiring override proofs$`, changedOrdinaryCodexLaunchArgumentsShouldBeRefused)
	ctx.Step(`^AGM validates managed private executor workspace containment$`, agmValidatesManagedPrivateExecutorWorkspaceContainment)
	ctx.Step(`^Claude and Codex should reject a workspace symlink retargeted outside the exact workspace or a retargeted workspace authority$`, privateExecutorsShouldRejectRetargetedWorkspaceSymlinkOrAuthority)
	ctx.Step(`^AGM validates configured sandbox volume selection$`, agmValidatesConfiguredSandboxVolumeSelection)
	ctx.Step(`^initial admission should use the configured volume$`, initialAdmissionShouldUseConfiguredVolume)
	ctx.Step(`^each disk probe should re-resolve the planned path and fail closed on ambiguity or symlinks$`, eachDiskProbeShouldReResolvePlannedPathAndFailClosed)
	ctx.Step(`^AGM validates the final private executor disk recheck$`, agmValidatesFinalPrivateExecutorDiskRecheck)
	ctx.Step(`^the private executor should recheck the configured volume or default reader with the caller threshold after executable preparation and before optional proof commitment or process replacement$`, privateExecutorShouldRecheckVolumeAndCallerThresholdAtFinalBoundary)
}

func agmValidatesSandboxLaunchSelection(ctx context.Context, lifecycle, harness string) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	if lifecycle != "create" && lifecycle != "cold-resume" {
		return fmt.Errorf("unsupported sandbox launch lifecycle %q", lifecycle)
	}
	if harness != "claude-code" && harness != "codex-cli" {
		return fmt.Errorf("unsupported private harness %q", harness)
	}
	sandboxLaunchSelectionOnce.Do(func() {
		sandboxLaunchSelectionCached.output, sandboxLaunchSelectionCached.err = runLocalGuardrailNamedGoTests(
			ctx,
			"./agm/internal/ops",
			"TestProjectPrivateLaunchAuthorityUsesStableSessionIDAcrossCreateAndColdResume",
			"TestProjectPrivateLaunchAuthorityRejectsWorkDirOutsideWorkspace",
			"TestProjectPrivateLaunchAuthorityAcceptsAPFSMergedWorkDir",
			"TestValidateSandboxResumePathsRejectsPersistedAuthorityDrift",
			"TestValidateSandboxResumePathsAcceptsAPFSMergedSymlink",
			"TestPrepareResumeLaunchRejectsClaudeTranscriptOutsideStableSessionWorkspace",
		)
	})
	state.suiteOutput = sandboxLaunchSelectionCached.output
	state.suiteErr = sandboxLaunchSelectionCached.err
	state.selectionMarker = "TestProjectPrivateLaunchAuthorityUsesStableSessionIDAcrossCreateAndColdResume/" + lifecycle + "/" + harness
	return nil
}

func configuredParentAndStableSessionChildShouldReachLaunchBoundary(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	if state.suiteErr != nil {
		return fmt.Errorf("sandbox launch selection regressions failed: %w\n%s", state.suiteErr, state.suiteOutput)
	}
	if !strings.Contains(state.suiteOutput, "=== RUN   "+state.selectionMarker) {
		return fmt.Errorf("sandbox launch selection output omitted %q:\n%s", state.selectionMarker, state.suiteOutput)
	}
	return nil
}

func persistedOrEffectiveLaunchPathsOutsideChildShouldBeRefused(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	if state.suiteErr != nil {
		return fmt.Errorf("sandbox launch selection regressions failed: %w\n%s", state.suiteErr, state.suiteOutput)
	}
	for _, testName := range []string{
		"TestProjectPrivateLaunchAuthorityRejectsWorkDirOutsideWorkspace",
		"TestValidateSandboxResumePathsRejectsPersistedAuthorityDrift",
		"TestPrepareResumeLaunchRejectsClaudeTranscriptOutsideStableSessionWorkspace",
	} {
		marker := "=== RUN   " + testName
		if !strings.Contains(state.suiteOutput, marker) {
			return fmt.Errorf("sandbox launch selection output omitted %q regression:\n%s", testName, state.suiteOutput)
		}
	}
	return nil
}

func apfsMergedDirectorySymlinksShouldRemainValidForCreateAndColdResume(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	if state.suiteErr != nil {
		return fmt.Errorf("sandbox launch selection regressions failed: %w\n%s", state.suiteErr, state.suiteOutput)
	}
	for _, testName := range []string{
		"TestProjectPrivateLaunchAuthorityAcceptsAPFSMergedWorkDir",
		"TestValidateSandboxResumePathsAcceptsAPFSMergedSymlink",
	} {
		marker := "=== RUN   " + testName
		if !strings.Contains(state.suiteOutput, marker) {
			return fmt.Errorf("sandbox launch selection output omitted %q regression:\n%s", testName, state.suiteOutput)
		}
	}
	return nil
}

func agmValidatesSandboxProviderOutputContainment(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/cmd/agm",
		"TestValidateCreatedSandboxRejectsAuthorityWidening",
		"TestValidateCreatedSandboxSeparatesPhysicalLaunchPathFromDurableProviderSpelling",
		"TestProvisionSandboxWritesOnboardingForPhysicalAPFSLaunchPath",
		"TestProvisionSandboxRejectsUnmaterializedProviderPathsBeforeOnboardingWrite",
		"TestProvisionSandboxRejectsEscapingProviderBeforeOutsideOnboardingWriteAndCleansStableSession",
		"TestCreateLifecycleRollsBackWithOriginalProviderInstanceAfterProvisioning",
	)
	return nil
}

func maliciousProviderPathsShouldBeRefusedBeforeWritesAndCleaned(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "sandbox provider output containment")
}

func unmaterializedProviderPathsShouldBeRefusedBeforeHostDirectoriesAreCreated(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "sandbox provider output containment")
}

func laterCreateFailuresShouldCleanThroughOriginalProviderInstance(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "sandbox provider output containment")
}

func containedProviderSymlinksShouldPreserveDurableOwnershipSpelling(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "sandbox provider output containment")
}

func agmValidatesFreshAndColdResumeSandboxContainment(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/internal/ops",
		"TestCreateSession_SandboxedCodexRejectsPreparedCwdOutsideAuthorityBeforeRemoteOrTmuxMutation",
		"TestCreateSession_SandboxedCodexRejectsMissingPreparedCwdBeforeRemoteOrTmuxMutation",
		"TestValidateFreshSandboxCreateAuthorityRejectsMissingWorkspaceDirectory",
		"TestValidateSandboxResumePathsRejectsPersistedAuthorityDrift",
		"TestResumeSessionRejectsSandboxPathDriftBeforeTmuxCreation",
		"TestResumeSessionRejectsMissingClaudeTranscriptDirectoryBeforeTmuxCreation",
	)
	return nil
}

func freshCreationShouldRefuseOutsideWorkDirBeforeMutation(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "fresh and cold-resume sandbox containment")
}

func coldResumeShouldRefusePathDriftOrMissingDirectoriesBeforeTmuxCreation(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "fresh and cold-resume sandbox containment")
}

func fsGuardUsesSandboxWorkspaceAuthority(ctx context.Context, authority string) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.policy = fsguard.DefaultPolicy(state.fakeHome)
	switch authority {
	case "default":
		return nil
	case "configured":
		state.policy.SandboxWorkspace = state.configuredWorkspace
		return nil
	default:
		return fmt.Errorf("unsupported sandbox workspace authority %q", authority)
	}
}

func fsGuardClassifiesTarget(ctx context.Context, targetKind string) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	var target string
	switch targetKind {
	case "default descendant":
		target = filepath.Join(state.fakeHome, ".agm", "sandboxes", "legacy", "upper", "file")
	case "exact workspace":
		target = state.configuredWorkspace
	case "workspace descendant":
		target = filepath.Join(state.configuredWorkspace, "upper", "file")
	case "workspace dotfile":
		target = filepath.Join(state.configuredWorkspace, ".session-state", "file")
	case "sibling workspace":
		target = filepath.Join(filepath.Dir(state.configuredWorkspace), "sibling-session", "upper", "file")
	case "lexical prefix lookalike":
		target = state.configuredWorkspace + "-other" + string(filepath.Separator) + "file"
	case "parent traversal escape":
		target = state.configuredWorkspace + string(filepath.Separator) + ".." + string(filepath.Separator) + "sibling-session" + string(filepath.Separator) + "file"
	default:
		return fmt.Errorf("unsupported sandbox workspace target %q", targetKind)
	}
	state.decisionAllowed, _ = fsguard.NewWithPolicy(state.policy).Classify(target, state.fakeHome)
	return nil
}

func sandboxWorkspaceDecisionShouldBe(ctx context.Context, decision string) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	wantAllowed := false
	switch decision {
	case "allow":
		wantAllowed = true
	case "deny":
	default:
		return fmt.Errorf("unsupported sandbox workspace decision %q", decision)
	}
	if state.decisionAllowed != wantAllowed {
		return fmt.Errorf("sandbox workspace allowed = %v, want %v", state.decisionAllowed, wantAllowed)
	}
	return nil
}

func agmValidatesInvalidSandboxWorkspaceAuthority(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./internal/fsguard",
		"TestInvalidConfiguredSandboxWorkspaceFailsClosed",
		"TestInvalidSandboxWorkspaceDoesNotRemoveExplicitWritablePaths",
	)
	return nil
}

func invalidSandboxWorkspaceAuthorityShouldLeaveNoDefaultAllowance(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "invalid sandbox workspace")
}

func agmValidatesWritableRootSymlinkResolution(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./internal/fsguard",
		"TestDanglingSymlinkEscapeBlockedForWritableRoots",
	)
	return nil
}

func danglingSymlinksShouldBeDeniedWhileOrdinaryDescendantsRemainAllowed(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "writable-root symlink resolution")
}

func agmValidatesExactSandboxWorkspaceEscapeResistance(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./internal/fsguard",
		"TestSymlinkParentTraversalBlockedForFileAndBashHooks",
		"TestUnhandledTildeExpansionBlockedForBashHook",
		"TestExactSandboxWorkspaceShadowsGenericWritableParents",
	)
	return nil
}

func symlinkParentTraversalAndUnsupportedTildeExpansionShouldBeDenied(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "exact sandbox workspace escape resistance")
}

func exactWorkspaceAuthorityShouldShadowWritableParents(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "exact sandbox workspace escape resistance")
}

func agmValidatesPrivateSandboxAuthorityHandoffs(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/internal/harnessexec",
		"TestValidatePrivateFilesystemAuthorityRequiresImmediateChild",
		"TestPrivateFilesystemAuthorityUsesHandoffWithoutCommandLeakage",
	)
	return nil
}

func claudeAndCodexShouldConsumeExactAuthorityWithoutCommandAuthorityEncoding(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "private sandbox authority handoff")
}

func agmValidatesOrdinaryCodexHandoffArgumentBinding(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/internal/harnessexec",
		"TestOrdinaryCodexHandoffRejectsChangedLaunchArguments",
	)
	return nil
}

func changedOrdinaryCodexLaunchArgumentsShouldBeRefused(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "ordinary Codex handoff argument binding")
}

func agmValidatesManagedPrivateExecutorWorkspaceContainment(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/internal/harnessexec",
		"TestPrivateExecutorsRejectRetargetedSandboxWorkDir",
		"TestResolvePrivateSandboxWorkDirRejectsRetargetedWorkspaceAuthority",
	)
	return nil
}

func privateExecutorsShouldRejectRetargetedWorkspaceSymlinkOrAuthority(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "managed private executor workspace containment")
}

func agmValidatesConfiguredSandboxVolumeSelection(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	cmdOutput, cmdErr := runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/cmd/agm",
		"TestEnforceCircuitBreakersUsesConfiguredSandboxVolume",
	)
	circuitOutput, circuitErr := runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/internal/circuitbreaker",
		"TestNewDiskReaderUsesPlannedSandboxRoot",
		"TestNewDiskReaderUsesDeepestExistingAncestorForENOENT",
		"TestNewDiskReaderFailsClosedForNonENOENT",
		"TestNewDiskReaderFailsClosedForSymlinkCandidates",
		"TestNewDiskReaderRevalidatesPlannedPathOnEveryProbe",
		"TestNewDiskReaderRejectsAmbiguousPaths",
		"TestNewDiskReaderFailsClosedForExistingPathThroughSymlinkAncestor",
		"TestCheckDiskHeadroomRunsOnlyDiskGate",
	)
	state.suiteOutput = strings.Join([]string{cmdOutput, circuitOutput}, "\n")
	state.suiteErr = errors.Join(cmdErr, circuitErr)
	return nil
}

func initialAdmissionShouldUseConfiguredVolume(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "configured sandbox volume selection")
}

func eachDiskProbeShouldReResolvePlannedPathAndFailClosed(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "configured sandbox volume selection")
}

func agmValidatesFinalPrivateExecutorDiskRecheck(ctx context.Context) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	state.suiteOutput, state.suiteErr = runLocalGuardrailNamedGoTests(
		ctx,
		"./agm/internal/harnessexec",
		"TestDefaultPrivateDiskAdmissionDoesNotSkipEmptyRoot",
		"TestCarriedDiskThresholdOverridesStaleExecutorEnvironment",
		"TestHandoffDiskThresholdRequiresExplicitFinitePresence",
		"TestPrivateExecutorsRecheckSandboxDiskWithoutOverrideProofs",
	)
	return nil
}

func privateExecutorShouldRecheckVolumeAndCallerThresholdAtFinalBoundary(ctx context.Context) error {
	return requireSandboxLaunchAuthoritySuite(ctx, "private executor sandbox-volume recheck")
}

func requireSandboxLaunchAuthoritySuite(ctx context.Context, label string) error {
	state, err := sandboxLaunchAuthorityStateFrom(ctx)
	if err != nil {
		return err
	}
	if state.suiteErr != nil {
		return fmt.Errorf("%s regressions failed: %w\n%s", label, state.suiteErr, state.suiteOutput)
	}
	return nil
}

func sandboxLaunchAuthorityStateFrom(ctx context.Context) (*sandboxLaunchAuthorityState, error) {
	state, ok := ctx.Value(sandboxLaunchAuthorityStateKey{}).(*sandboxLaunchAuthorityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("sandbox launch authority state not initialized")
	}
	return state, nil
}
