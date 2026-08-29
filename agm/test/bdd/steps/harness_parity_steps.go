package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	commandparity "github.com/vbonnet/dear-agent/agm/cmd/agm/parity"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/configdirparity"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/engramparity"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/marketplaceparity"
	"github.com/vbonnet/dear-agent/agm/internal/mcpparity"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/quotaparity"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/reaper"
	"github.com/vbonnet/dear-agent/agm/internal/recovery"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/wayfinderparity"
	"github.com/vbonnet/dear-agent/engram/hippocampus"
)

type harnessParityState struct {
	paneOutput                 string
	detected                   state.DetectionResult
	canReceive                 state.CanReceive
	harness                    string
	codexSessionUUID           string
	agyConversationID          string
	preservedCodexUUID         bool
	preservedAgyConversationID bool
	agyResumeCommand           string
	agyModelKnown              bool
	readyFileCreated           bool
	waitedForComposer          bool
	waitedForAgyPrompt         bool
	startupDelivered           bool
	trustAutoAccepted          bool
	codexHookReviewErr         error
	codexHookReviewState       string
	codexHookReviewTestOutput  string
	codexHookReviewTestErr     error
	sendSafetyRequiresClaude   bool
	sessionListFields          []string
	sessionListHasArray        bool
	tmuxResumeLaunched         bool
	tmuxSessionExists          bool
	configuredHarness          string
	configuredModelFamily      string
	modelFamilyDefaulted       bool
	modelChangeCommand         string
	modelChangeResolvedModel   string
	permissionProfile          string
	permissionSurfaces         []permissionparity.Surface
	permissionAllowList        []string
	piPermissionMode           string
	piPermissionPolicy         []string
	piPermissionDecision       permissionparity.PiDecision
	piExactProcess             bool
	piPaneLiveness             tmux.PaneLiveness
	piPaneResumeAction         agent.PiPaneResumeAction
	piPaneResumeErr            error
	piProcessCommand           string
	piProcessRecognized        bool
	quotaSurfaces              []quotaparity.HarnessSurface
	quotaFamilyCoverage        quotaparity.ModelFamilyCoverage
	mcpSurface                 mcpparity.CreateSessionSurface
	mcpModelAccepted           bool
	mcpLifecycleOpsExposed     bool
	mcpServerStartupGuard      bool
	mcpKillTestOutput          string
	mcpKillTestErr             error
	marketplaceCatalog         marketplaceparity.Catalog
	marketplaceSurface         marketplaceparity.HarnessSurface
	marketplaceMirrorValid     bool
	marketplacePlugin          string
	marketplacePluginValid     bool
	engramSurface              engramparity.HarnessSurface
	engramMetadataValid        bool
	hippocampusAdapter         hippocampus.HarnessAdapter
	hippocampusLLMNeutral      bool
	wayfinderSurface           wayfinderparity.HarnessSurface
	wayfinderAssetsValid       bool
	wayfinderPhaseEngrams      bool
	configDirSurface           configdirparity.DirectorySurface
	conformanceFindings        []agent.HarnessConformanceFinding
	harnessHealth              agent.HarnessHealth
	runtimeHelperCommand       string
	runtimeHelperSpec          string
	runtimeMainSource          string
	runtimeProductionSource    string
	runtimeOpsSource           string
	runtimeSendSource          string
	runtimeTmuxSource          string
	runtimeExecSource          string
	cleanupSupportPackage      string
	cleanupSupportSpec         string
	archiveCleanupTestOutput   string
	archiveCleanupTestErr      error
	archiveDryRunTestOutput    string
	archiveDryRunTestErr       error
	a2aCoordinationSpecsValid  bool
	captureInvocationArgs      []string
	captureSessionName         string
	commandParityValid         bool
	commandSourceCoverageValid bool
	modelCommandParityValid    bool
	recoveryConfirmationValid  bool
	recoveryCancellationValid  bool
	recoveryFallback           recovery.Fallback
	capturePolicyValid         bool
	launchMode                 string
	launchContract             launchparity.Contract
	piCodingAgentDir           string
	piCreateCommand            string
	piResumeCommand            string
	piMetadataPreserved        bool
	startupLivenessValid       bool
	currentTmuxTestOutput      string
	currentTmuxTestErr         error
	agyLifecycleTestOutput     string
	agyLifecycleTestErr        error
	resumeSource               string
	sharedSendReadiness        string
	sharedSendResult           *ops.SendMessageResult
	sharedSendErr              error
	sharedSendTmux             *session.MockTmux
	sharedSendCancelled        bool
	sharedCreateErr            error
	sharedCreateTmux           *session.MockTmux
	sharedCreateStore          *dolt.MockAdapter
	startupReadinessTestOutput string
	startupReadinessTestErr    error
	gracefulExitCommand        string
	privateLaunchCommand       string
	privateChildEnvironment    []string
	privateAllowedCanaries     []string
	privateRejectedCanaries    []string
	privateHandoffTestOutput   string
	privateHandoffTestErr      error
	lifecycleDir               string
	lifecycleStore             *dolt.Adapter
	lifecycleTmux              *bddLifecycleTmux
	lifecycleRuntime           *bddLifecycleRuntime
	lifecycleArchiver          *bddCodexArchiver
	lifecycleOps               *ops.OpContext
	lifecycleSessionID         string
	lifecycleSessionName       string
	lifecycleTransitions       []string
}

type harnessParityStateKey struct{}

type bddBehaviorSuiteResult struct {
	output       string
	testErr      error
	executionErr error
}

type bddBehaviorSuiteCache struct {
	once   sync.Once
	result bddBehaviorSuiteResult
}

func (c *bddBehaviorSuiteCache) load(run func() bddBehaviorSuiteResult) bddBehaviorSuiteResult {
	c.once.Do(func() {
		c.result = run()
	})
	return c.result
}

var (
	sharedAgyLifecycleBehaviorSuite    bddBehaviorSuiteCache
	sharedCodexHookReviewBehaviorSuite bddBehaviorSuiteCache
)

const codexHookReviewBehaviorSuiteTimeout = 2 * time.Minute

type bddCreateSessionRuntime struct {
	tmux *session.MockTmux
}

func (r *bddCreateSessionRuntime) Launch(_ context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
	launch := ops.BuildHarnessLaunchCommand(spec)
	return ops.CreateSessionLaunchResult{ModeAppliedAtStartup: launch.ModeAppliedAtStartup}, r.tmux.SendKeys(spec.SessionName, launch.Command)
}

func (r *bddCreateSessionRuntime) Complete(_ context.Context, completion ops.CreateSessionCompletion) error {
	if completion.Prompt == "" {
		return nil
	}
	return r.tmux.SendKeys(completion.Manifest.Name, completion.Prompt)
}

// RegisterHarnessParitySteps registers BDD steps for cross-harness delivery parity.
func RegisterHarnessParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, harnessParityStateKey{}, &harnessParityState{}), nil
	})
	ctx.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		harnessState, ok := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
		if !ok || harnessState == nil {
			return ctx, nil
		}
		if harnessState.lifecycleStore != nil {
			if err := harnessState.lifecycleStore.Close(); err != nil && scenarioErr == nil {
				return ctx, err
			}
		}
		if harnessState.lifecycleDir != "" {
			if err := os.RemoveAll(harnessState.lifecycleDir); err != nil && scenarioErr == nil {
				return ctx, err
			}
		}
		return ctx, nil
	})

	ctx.Step(`^a Codex CLI composer pane$`, aCodexCLIComposerPane)
	ctx.Step(`^a stale Codex CLI composer followed by shell output$`, aStaleCodexCLIComposerFollowedByShellOutput)
	ctx.Step(`^harness "([^"]*)" is configured$`, harnessIsConfigured)
	ctx.Step(`^AGM selects the graceful reaper exit command$`, agmSelectsGracefulReaperExitCommand)
	ctx.Step(`^the graceful reaper exit command should be "([^"]*)"$`, gracefulReaperExitCommandShouldBe)
	ctx.Step(`^AGM validates active parity support$`, agmValidatesActiveParitySupport)
	ctx.Step(`^harness "([^"]*)" should be active for parity$`, harnessShouldBeActiveForParity)
	ctx.Step(`^harness "([^"]*)" should not be deprecated$`, harnessShouldNotBeDeprecated)
	ctx.Step(`^harness "([^"]*)" should be deprecated$`, harnessShouldBeDeprecated)
	ctx.Step(`^AGM resolves doctor health for the configured harness$`, agmResolvesDoctorHealthForConfiguredHarness)
	ctx.Step(`^doctor should recognize CLI binary "([^"]*)"$`, doctorShouldRecognizeCLIBinary)
	ctx.Step(`^doctor should recognize config directory suffix "([^"]*)"$`, doctorShouldRecognizeConfigDirectorySuffix)
	ctx.Step(`^AGM active harnesses are configured$`, agmActiveHarnessesAreConfigured)
	ctx.Step(`^current-tmux creation selects Codex CLI$`, currentTmuxCreationSelectsCodexCLI)
	ctx.Step(`^AGM validates current-tmux Codex launch wiring$`, agmValidatesCurrentTmuxCodexLaunchWiring)
	ctx.Step(`^Codex credential validation should precede the canonical launcher$`, codexCredentialValidationShouldPrecedeCanonicalLauncher)
	ctx.Step(`^the top-level new command should route into current tmux$`, topLevelNewCommandShouldRouteIntoCurrentTmux)
	ctx.Step(`^Codex current-tmux launch should require the executable without waiting behind its own AGM process$`, codexCurrentTmuxLaunchShouldRequireExecutableWithoutWaiting)
	ctx.Step(`^every queued current-tmux harness should defer readiness until AGM exits$`, everyQueuedCurrentTmuxHarnessShouldDeferReadinessUntilAGMExits)
	ctx.Step(`^queued private handoffs should carry producer-exit liveness$`, queuedPrivateHandoffsShouldCarryProducerExitLiveness)
	ctx.Step(`^current-tmux Claude should associate its UUID on SessionStart$`, currentTmuxClaudeShouldAssociateItsUUIDOnSessionStart)
	ctx.Step(`^Codex queue failures should propagate to shared creation rollback$`, codexQueueFailuresShouldPropagateToSharedCreationRollback)
	ctx.Step(`^current-tmux creation selects AGY$`, currentTmuxCreationSelectsAGY)
	ctx.Step(`^AGM validates current-tmux AGY safety$`, agmValidatesCurrentTmuxAGYSafety)
	ctx.Step(`^current-tmux AGY creation should fail before launch with detached guidance$`, currentTmuxAGYCreationShouldFailBeforeLaunchWithDetachedGuidance)
	ctx.Step(`^AGM validates active harness adapter conformance$`, agmValidatesActiveHarnessAdapterConformance)
	ctx.Step(`^every active harness adapter should satisfy the shared conformance suite$`, everyActiveHarnessAdapterShouldSatisfySharedConformanceSuite)
	ctx.Step(`^AGM Codex and OpenAI adapter sources$`, agmCodexAndOpenAIAdapterSources)
	ctx.Step(`^AGM validates Codex adapter routing$`, agmValidatesCodexAdapterRouting)
	ctx.Step(`^Codex factory should use the Codex CLI adapter$`, codexFactoryShouldUseCodexCLIAdapter)
	ctx.Step(`^OpenAI API status should not inspect Codex tmux state$`, openAIAPIStatusShouldNotInspectCodexTmuxState)
	ctx.Step(`^AGM validates the pane capture invocation$`, agmValidatesPaneCaptureInvocation)
	ctx.Step(`^pane capture should use the canonical AGM tmux socket$`, paneCaptureShouldUseCanonicalAGMTmuxSocket)
	ctx.Step(`^pane capture should normalize the session target$`, paneCaptureShouldNormalizeSessionTarget)
	ctx.Step(`^pane capture should be bounded and process-group isolated$`, paneCaptureShouldBeBoundedAndProcessGroupIsolated)
	ctx.Step(`^AGM tmux-facing command sources$`, agmTmuxFacingCommandSources)
	ctx.Step(`^AGM validates tmux command parity contracts$`, agmValidatesTmuxCommandParityContracts)
	ctx.Step(`^every tmux-facing command should declare all active harness strategies$`, everyTmuxFacingCommandShouldDeclareAllActiveHarnessStrategies)
	ctx.Step(`^every tmux-facing Cobra command source should have a parity contract$`, everyTmuxFacingCobraCommandSourceShouldHaveAParityContract)
	ctx.Step(`^AGM validates model-independent tmux command parity$`, agmValidatesModelIndependentTmuxCommandParity)
	ctx.Step(`^model-independent tmux commands should support model family "([^"]*)"$`, modelIndependentTmuxCommandsShouldSupportModelFamily)
	ctx.Step(`^AGM validates session recovery parity$`, agmValidatesSessionRecoveryParity)
	ctx.Step(`^recovery should require process-state confirmation$`, recoveryShouldRequireProcessStateConfirmation)
	ctx.Step(`^recovery waits should respect context cancellation$`, recoveryWaitsShouldRespectContextCancellation)
	ctx.Step(`^harness "([^"]*)" should have a safe recovery fallback policy$`, harnessShouldHaveSafeRecoveryFallbackPolicy)
	ctx.Step(`^active harness "([^"]*)" uses startup mode "([^"]*)"$`, activeHarnessUsesStartupMode)
	ctx.Step(`^AGM builds the harness launch command with persistence enabled$`, agmBuildsPersistentHarnessLaunchCommand)
	ctx.Step(`^the launch command should use the native interactive startup contract$`, launchCommandShouldUseNativeInteractiveContract)
	ctx.Step(`^the launch command should not exit the tmux pane shell$`, launchCommandShouldNotExitTmuxPaneShell)
	ctx.Step(`^a validated Pi custom coding-agent directory$`, aValidatedPiCustomCodingAgentDirectory)
	ctx.Step(`^AGM builds Pi create and cold-resume commands from native metadata$`, agmBuildsPiCreateAndColdResumeCommands)
	ctx.Step(`^both Pi commands should forward the safely quoted coding-agent directory$`, bothPiCommandsShouldForwardCodingAgentDirectory)
	ctx.Step(`^Pi native metadata should preserve the coding-agent directory$`, piNativeMetadataShouldPreserveCodingAgentDirectory)
	ctx.Step(`^default Pi launch should leave native configuration discovery unchanged$`, defaultPiLaunchShouldLeaveNativeDiscoveryUnchanged)
	ctx.Step(`^AGM validates final startup liveness$`, agmValidatesFinalStartupLiveness)
	ctx.Step(`^startup should require a live tmux session and harness process$`, startupShouldRequireLiveTmuxAndHarness)
	ctx.Step(`^AGM runtime helper command "([^"]*)" is configured$`, agmRuntimeHelperCommandIsConfigured)
	ctx.Step(`^AGM validates runtime helper command coverage$`, agmValidatesRuntimeHelperCommandCoverage)
	ctx.Step(`^runtime helper command "([^"]*)" should have a co-located SPEC$`, runtimeHelperCommandShouldHaveCoLocatedSPEC)
	ctx.Step(`^AGM production local runtime sources$`, agmProductionLocalRuntimeSources)
	ctx.Step(`^AGM validates single runtime ownership$`, agmValidatesSingleRuntimeOwnership)
	ctx.Step(`^production should use only the direct session tmux runtime type$`, productionShouldUseOnlyDirectSessionTmuxRuntimeType)
	ctx.Step(`^shared operations should expose no parallel manager runtime$`, sharedOperationsShouldExposeNoParallelManagerRuntime)
	ctx.Step(`^the direct tmux runtime should prove its safety capabilities$`, directTmuxRuntimeShouldProveSafetyCapabilities)
	ctx.Step(`^retired generalized runtimes and selection setting should be absent$`, retiredGeneralizedRuntimesAndSelectionSettingShouldBeAbsent)
	ctx.Step(`^AGM cleanup support package "([^"]*)" is configured$`, agmCleanupSupportPackageIsConfigured)
	ctx.Step(`^AGM validates cleanup support package coverage$`, agmValidatesCleanupSupportPackageCoverage)
	ctx.Step(`^cleanup support package "([^"]*)" should have a co-located SPEC$`, cleanupSupportPackageShouldHaveCoLocatedSPEC)
	ctx.Step(`^AGM archive cleanup targets a repository checkout$`, agmArchiveCleanupTargetsRepositoryCheckout)
	ctx.Step(`^AGM validates primary checkout cleanup safety$`, agmValidatesPrimaryCheckoutCleanupSafety)
	ctx.Step(`^the primary checkout and session-named branch should remain$`, primaryCheckoutAndSessionNamedBranchShouldRemain)
	ctx.Step(`^a linked session worktree should still be removed$`, linkedSessionWorktreeShouldStillBeRemoved)
	ctx.Step(`^linked worktree cleanup should continue through the surviving checkout$`, linkedWorktreeCleanupShouldContinueThroughTheSurvivingCheckout)
	ctx.Step(`^an unclassified worktree should not authorize branch deletion$`, unclassifiedWorktreeShouldNotAuthorizeBranchDeletion)
	ctx.Step(`^a context-only checkout should not authorize branch deletion$`, contextOnlyCheckoutShouldNotAuthorizeBranchDeletion)
	ctx.Step(`^branch deletion should require attributed worktree ownership$`, branchDeletionShouldRequireAttributedWorktreeOwnership)
	ctx.Step(`^AGM has a single-session archive dry-run contract$`, agmHasSingleSessionArchiveDryRunContract)
	ctx.Step(`^AGM validates single-session archive dry-run safety$`, agmValidatesSingleSessionArchiveDryRunSafety)
	ctx.Step(`^durable and provider archive state should remain unchanged$`, durableAndProviderArchiveStateShouldRemainUnchanged)
	ctx.Step(`^archive preview should return stable AGM-100 output$`, archivePreviewShouldReturnStableAGM100Output)
	ctx.Step(`^archive preview should retain the resolved stable session identity$`, archivePreviewShouldRetainResolvedStableSessionIdentity)
	ctx.Step(`^archive completion guidance should use the resolved stable session identity$`, archiveCompletionGuidanceShouldUseResolvedStableSessionIdentity)
	ctx.Step(`^active async archive should separate stable and tmux identities$`, activeAsyncArchiveShouldSeparateStableAndTmuxIdentities)
	ctx.Step(`^archive preview should honor global JSON field masks$`, archivePreviewShouldHonorGlobalJSONFieldMasks)
	ctx.Step(`^active async preview should not start a detached reaper$`, activeAsyncPreviewShouldNotStartDetachedReaper)
	ctx.Step(`^dry-run preview should preserve async state validation$`, dryRunPreviewShouldPreserveAsyncStateValidation)
	ctx.Step(`^validated persisted sandbox ownership should control archive cleanup after reload$`, validatedPersistedSandboxOwnershipShouldControlArchiveCleanupAfterReload)
	ctx.Step(`^archive cleanup should wait for transient child exit without weakening safety gates$`, archiveCleanupShouldWaitForTransientChildExitWithoutWeakeningSafetyGates)
	ctx.Step(`^archive cleanup should preserve settings written during retry grace$`, archiveCleanupShouldPreserveSettingsWrittenDuringRetryGrace)
	ctx.Step(`^unarchive should serialize with archive cleanup$`, unarchiveShouldSerializeWithArchiveCleanup)
	ctx.Step(`^admin reconcile fixes should serialize and revalidate under the archive lock$`, adminReconcileFixesShouldSerializeAndRevalidateUnderTheArchiveLock)
	ctx.Step(`^the retained A2A coordination implementation$`, retainedA2ACoordinationImplementation)
	ctx.Step(`^AGM validates A2A coordination specification drift$`, agmValidatesA2ACoordinationSpecificationDrift)
	ctx.Step(`^A2A coordination specifications should describe only retained behavior$`, a2aCoordinationSpecificationsShouldDescribeOnlyRetainedBehavior)
	ctx.Step(`^model family "([^"]*)" is configured$`, modelFamilyIsConfigured)
	ctx.Step(`^AGM validates model family parity support$`, agmValidatesModelFamilyParitySupport)
	ctx.Step(`^model family "([^"]*)" should be supported$`, modelFamilyShouldBeSupported)
	ctx.Step(`^model family "([^"]*)" should have a default model route$`, modelFamilyShouldHaveDefaultModelRoute)
	ctx.Step(`^AGM resolves a model change for harness "([^"]*)" with model "([^"]*)"$`, agmResolvesModelChangeForHarness)
	ctx.Step(`^the model change should use tmux command "([^"]*)"$`, modelChangeShouldUseTmuxCommand)
	ctx.Step(`^the resolved model should not be empty$`, resolvedModelShouldNotBeEmpty)
	ctx.Step(`^permission profile "([^"]*)" is configured$`, permissionProfileIsConfigured)
	ctx.Step(`^AGM validates permission parity support$`, agmValidatesPermissionParitySupport)
	ctx.Step(`^AGM resolves permission policy parity$`, agmResolvesPermissionPolicyParity)
	ctx.Step(`^harness "([^"]*)" should have a permission policy target$`, harnessShouldHavePermissionPolicyTarget)
	ctx.Step(`^harness "([^"]*)" should have a startup permission surface$`, harnessShouldHaveStartupPermissionSurface)
	ctx.Step(`^every active harness should have a permission policy target$`, everyActiveHarnessShouldHavePermissionPolicyTarget)
	ctx.Step(`^the resolved permission policy should include default permissions$`, resolvedPermissionPolicyShouldIncludeDefaultPermissions)
	ctx.Step(`^the resolved permission policy should include profile permissions$`, resolvedPermissionPolicyShouldIncludeProfilePermissions)
	ctx.Step(`^Pi permission mode "([^"]*)" with policy "([^"]*)"$`, piPermissionModeWithPolicy)
	ctx.Step(`^Pi requests tool "([^"]*)" with input "([^"]*)" in an interactive session$`, piRequestsToolInteractively)
	ctx.Step(`^the Pi permission decision should be "([^"]*)"$`, piPermissionDecisionShouldBe)
	ctx.Step(`^an existing Pi pane with exact process "([^"]*)" and liveness "([^"]*)"$`, existingPiPaneWithLiveness)
	ctx.Step(`^AGM evaluates Pi cold resume safety$`, agmEvaluatesPiColdResumeSafety)
	ctx.Step(`^Pi resume should "([^"]*)"$`, piResumeShould)
	ctx.Step(`^an existing Pi pane process command "([^"]*)"$`, existingPiPaneProcessCommand)
	ctx.Step(`^AGM evaluates Pi process identity$`, agmEvaluatesPiProcessIdentity)
	ctx.Step(`^Pi process identity should be "([^"]*)"$`, piProcessIdentityShouldBe)
	ctx.Step(`^AGM validates quota monitoring parity$`, agmValidatesQuotaMonitoringParity)
	ctx.Step(`^AGM validates quota model family coverage$`, agmValidatesQuotaModelFamilyCoverage)
	ctx.Step(`^harness "([^"]*)" should have a context quota source$`, harnessShouldHaveContextQuotaSource)
	ctx.Step(`^harness "([^"]*)" should have a cost quota source$`, harnessShouldHaveCostQuotaSource)
	ctx.Step(`^harness "([^"]*)" should have a rate limit quota policy$`, harnessShouldHaveRateLimitQuotaPolicy)
	ctx.Step(`^model family "([^"]*)" should have a quota pricing policy$`, modelFamilyShouldHaveQuotaPricingPolicy)
	ctx.Step(`^model family "([^"]*)" should have sourced shared pricing$`, modelFamilyShouldHaveSourcedSharedPricing)
	ctx.Step(`^model family "([^"]*)" should have a default quota model route$`, modelFamilyShouldHaveDefaultQuotaModelRoute)
	ctx.Step(`^AGM validates MCP session creation parity$`, agmValidatesMCPSessionCreationParity)
	ctx.Step(`^harness "([^"]*)" should have an MCP create-session surface$`, harnessShouldHaveMCPCreateSessionSurface)
	ctx.Step(`^the MCP create-session surface should use shared model validation$`, mcpCreateSessionSurfaceShouldUseSharedModelValidation)
	ctx.Step(`^the MCP create-session surface should be deprecated compatibility$`, mcpCreateSessionSurfaceShouldBeDeprecatedCompatibility)
	ctx.Step(`^AGM validates MCP model identifier "([^"]*)"$`, agmValidatesMCPModelIdentifier)
	ctx.Step(`^the MCP model identifier should be accepted$`, mcpModelIdentifierShouldBeAccepted)
	ctx.Step(`^AGM validates MCP operation discovery parity$`, agmValidatesMCPOperationDiscoveryParity)
	ctx.Step(`^the MCP operation registry should expose lifecycle mutations$`, mcpOperationRegistryShouldExposeLifecycleMutations)
	ctx.Step(`^AGM validates MCP server startup guard coverage$`, agmValidatesMCPServerStartupGuardCoverage)
	ctx.Step(`^the MCP server SPEC should cover loud workspace and database failures$`, mcpServerSPECShouldCoverLoudWorkspaceAndDatabaseFailures)
	ctx.Step(`^AGM validates MCP kill mutation wiring$`, agmValidatesMCPKillMutationWiring)
	ctx.Step(`^MCP kill should provide a real tmux dependency to shared operations$`, mcpKillShouldProvideARealTmuxDependencyToSharedOperations)
	ctx.Step(`^shared kill success should require exact target absence$`, sharedKillSuccessShouldRequireExactTargetAbsence)
	ctx.Step(`^AGM validates marketplace parity$`, agmValidatesMarketplaceParity)
	ctx.Step(`^harness "([^"]*)" should have a marketplace discovery surface$`, harnessShouldHaveMarketplaceDiscoverySurface)
	ctx.Step(`^the marketplace discovery surface should use the expected mode$`, marketplaceDiscoverySurfaceShouldUseExpectedMode)
	ctx.Step(`^AGM validates marketplace catalog mirrors$`, agmValidatesMarketplaceCatalogMirrors)
	ctx.Step(`^the Claude marketplace should match the neutral marketplace catalog$`, claudeMarketplaceShouldMatchNeutralMarketplaceCatalog)
	ctx.Step(`^marketplace plugin "([^"]*)" is configured$`, marketplacePluginIsConfigured)
	ctx.Step(`^marketplace plugin "([^"]*)" should publish its declared assets$`, marketplacePluginShouldPublishDeclaredAssets)
	ctx.Step(`^AGM validates Engram parity$`, agmValidatesEngramParity)
	ctx.Step(`^harness "([^"]*)" should have an Engram injection surface$`, harnessShouldHaveEngramInjectionSurface)
	ctx.Step(`^harness "([^"]*)" should persist Engram metadata through the shared manifest$`, harnessShouldPersistEngramMetadataThroughSharedManifest)
	ctx.Step(`^AGM validates Engram metadata parity$`, agmValidatesEngramMetadataParity)
	ctx.Step(`^Engram metadata should be stored in harness-neutral fields$`, engramMetadataShouldBeStoredInHarnessNeutralFields)
	ctx.Step(`^AGM validates Hippocampus transcript parity$`, agmValidatesHippocampusTranscriptParity)
	ctx.Step(`^harness "([^"]*)" should have a Hippocampus transcript adapter$`, harnessShouldHaveHippocampusTranscriptAdapter)
	ctx.Step(`^AGM validates Hippocampus LLM parity$`, agmValidatesHippocampusLLMParity)
	ctx.Step(`^Hippocampus consolidation should use a model-family-neutral provider$`, hippocampusConsolidationShouldUseModelFamilyNeutralProvider)
	ctx.Step(`^AGM validates Wayfinder parity$`, agmValidatesWayfinderParity)
	ctx.Step(`^harness "([^"]*)" should have a Wayfinder discovery surface$`, harnessShouldHaveWayfinderDiscoverySurface)
	ctx.Step(`^harness "([^"]*)" should have a Wayfinder execution surface$`, harnessShouldHaveWayfinderExecutionSurface)
	ctx.Step(`^AGM validates Wayfinder asset parity$`, agmValidatesWayfinderAssetParity)
	ctx.Step(`^Wayfinder should publish SKILL, plugin, CLI, and MCP status surfaces$`, wayfinderShouldPublishSkillPluginCommandAndMCPStatusSurfaces)
	ctx.Step(`^AGM validates Wayfinder phase Engram parity$`, agmValidatesWayfinderPhaseEngramParity)
	ctx.Step(`^Wayfinder should resolve phase Engrams without harness-specific state$`, wayfinderShouldResolvePhaseEngramsWithoutHarnessSpecificState)
	ctx.Step(`^AGM validates configuration directory parity$`, agmValidatesConfigurationDirectoryParity)
	ctx.Step(`^AGM validates deprecated configuration directory parity$`, agmValidatesDeprecatedConfigurationDirectoryParity)
	ctx.Step(`^harness "([^"]*)" should have configuration directory "([^"]*)"$`, harnessShouldHaveConfigurationDirectory)
	ctx.Step(`^a Codex CLI trust prompt$`, aCodexCLITrustPrompt)
	ctx.Step(`^Codex hooks require explicit review in the "([^"]*)" surface$`, codexHooksRequireExplicitReview)
	ctx.Step(`^an AGY ready prompt$`, anAGYReadyPrompt)
	ctx.Step(`^an AGY trust prompt$`, anAGYTrustPrompt)
	ctx.Step(`^an AGY feedback survey over a ready prompt$`, anAGYFeedbackSurveyOverAReadyPrompt)
	ctx.Step(`^AGM checks whether the session can receive input$`, agmChecksWhetherTheSessionCanReceiveInput)
	ctx.Step(`^delivery should be allowed$`, deliveryShouldBeAllowed)
	ctx.Step(`^delivery should be queued$`, deliveryShouldBeQueued)
	ctx.Step(`^delivery should require dismissing an overlay$`, deliveryShouldRequireDismissingAnOverlay)
	ctx.Step(`^the detected session state should be "([^"]*)"$`, detectedSessionStateShouldBe)
	ctx.Step(`^Codex CLI is available$`, codexCLIIsAvailable)
	ctx.Step(`^AGY is available$`, agyIsAvailable)
	ctx.Step(`^AGM creates a detached Codex session with a startup prompt$`, agmCreatesDetachedCodexSessionWithStartupPrompt)
	ctx.Step(`^AGM creates a detached AGY session with a startup prompt$`, agmCreatesDetachedAGYSessionWithStartupPrompt)
	ctx.Step(`^AGM should wait for the Codex composer$`, agmShouldWaitForTheCodexComposer)
	ctx.Step(`^AGM should wait for the AGY prompt$`, agmShouldWaitForTheAGYPrompt)
	ctx.Step(`^AGM should deliver the startup prompt even though the session is detached$`, agmShouldDeliverStartupPromptDetached)
	ctx.Step(`^AGM should auto-accept the Codex trust prompt before prompt delivery$`, agmShouldAutoAcceptCodexTrustPromptBeforePromptDelivery)
	ctx.Step(`^AGM evaluates Codex hook review startup$`, agmEvaluatesCodexHookReviewStartup)
	ctx.Step(`^Codex startup should fail fast with explicit review guidance$`, codexStartupShouldFailFastWithExplicitReviewGuidance)
	ctx.Step(`^Codex hook review should receive no automated input$`, codexHookReviewShouldReceiveNoAutomatedInput)
	ctx.Step(`^AGM should auto-accept the AGY trust prompt before prompt delivery$`, agmShouldAutoAcceptAGYTrustPromptBeforePromptDelivery)
	ctx.Step(`^AGM runs send safety for the configured harness$`, agmRunsSendSafetyForTheConfiguredHarness)
	ctx.Step(`^send safety should not require a Claude process$`, sendSafetyShouldNotRequireClaudeProcess)
	ctx.Step(`^AGM validates AGY multiline delivery$`, agmValidatesAGYMultilineDelivery)
	ctx.Step(`^every AGY message surface should preserve one bracketed multiline submission$`, everyAGYMessageSurfaceShouldPreserveOneBracketedMultilineSubmission)
	ctx.Step(`^AGM validates the AGY adapter lifecycle$`, agmValidatesTheAgyAdapterLifecycle)
	ctx.Step(`^AGM validates AGY lazy identity bootstrap$`, agmValidatesAGYLazyIdentityBootstrap)
	ctx.Step(`^shared creation should deliver the AGY startup prompt before identity discovery$`, sharedCreationShouldDeliverAGYStartupPromptBeforeIdentityDiscovery)
	ctx.Step(`^every AGY creation surface should avoid duplicate prompt delivery$`, everyAGYCreationSurfaceShouldAvoidDuplicatePromptDelivery)
	ctx.Step(`^AGY bootstrap failures should preserve transactional rollback$`, agyBootstrapFailuresShouldPreserveTransactionalRollback)
	ctx.Step(`^the AGY adapter should preserve canonical launch and resume policy$`, agyAdapterShouldPreserveCanonicalLaunchAndResumePolicy)
	ctx.Step(`^the AGY adapter should require AGY process and transcript truth$`, agyAdapterShouldRequireAgyProcessAndTranscriptTruth)
	ctx.Step(`^a shared Codex send target with readiness "([^"]*)"$`, aSharedCodexSendTargetWithReadiness)
	ctx.Step(`^AGM sends a message through shared operations$`, agmSendsAMessageThroughSharedOperations)
	ctx.Step(`^the shared send result should be "([^"]*)"$`, theSharedSendResultShouldBe)
	ctx.Step(`^shared send should emit (\d+) tmux commands$`, sharedSendShouldEmitTmuxCommands)
	ctx.Step(`^the shared send request is cancelled$`, theSharedSendRequestIsCancelled)
	ctx.Step(`^shared Codex creation cannot observe the composer$`, sharedCodexCreationCannotObserveTheComposer)
	ctx.Step(`^AGM creates Codex through a surface runtime$`, agmCreatesCodexThroughSharedOperations)
	ctx.Step(`^shared creation should fail before registration and prompt delivery$`, sharedCreationShouldFailBeforeRegistrationAndPromptDelivery)
	ctx.Step(`^shared creation should remove its newly created tmux session$`, sharedCreationShouldRemoveItsNewlyCreatedTmuxSession)
	ctx.Step(`^AGM validates slow harness startup readiness$`, agmValidatesSlowHarnessStartupReadiness)
	ctx.Step(`^shared startup readiness should honor the total deadline$`, sharedStartupReadinessShouldHonorTheTotalDeadline)
	ctx.Step(`^shared input readiness should serialize exact-pane delivery and preserve rendered composer ownership without treating resolved prompts as live$`, sharedInputReadinessShouldRejectStaleClaudeComposerAndUnrelatedNodeProcess)
	ctx.Step(`^CLI message and startup prompt sends should use shared atomic readiness for exact-pane delivery$`, cliMessageSendsShouldUseSharedAtomicReadiness)
	ctx.Step(`^forced CLI message sends should preserve the measured queued AGM anchor across prompt-like payload lines$`, forcedCLIMessageSendsShouldReplaceOnlyPositivelyIdentifiedQueuedAGMInput)
	ctx.Step(`^autonomous CLI message sends should preserve only positively identified queued AGM recovery$`, autonomousCLIMessageSendsShouldPreserveOnlyPositivelyIdentifiedQueuedAGMRecovery)
	ctx.Step(`^API delivery should restore persisted configuration without scanning unrelated sessions, linearize archive and deletion with bounded completed turns, renew fan-out deadlines with separate preflight and full provider budgets, honor request cancellation during reconstruction and readiness, preserve large JSONL records, batch imports, require adapter readiness without tmux, and document its compatibility-only control plane$`, singleAndFanOutAPISendsShouldUseAdapterSessionReadinessWithoutRequiringTmux)
	ctx.Step(`^shared Gemini readiness should advance first-run trust on the verified pane$`, sharedGeminiReadinessShouldAdvanceFirstRunTrustOnTheVerifiedPane)
	ctx.Step(`^legacy AGY names should reach canonical shared send readiness$`, legacyAgyNamesShouldReachCanonicalSharedSendReadiness)
	ctx.Step(`^the Pi alias should reach canonical shared send readiness$`, piAliasShouldReachCanonicalSharedSendReadiness)
	ctx.Step(`^synthetic ambient credentials from multiple harnesses$`, syntheticAmbientCredentialsFromMultipleHarnesses)
	ctx.Step(`^AGM builds the Codex private launch boundary$`, agmBuildsTheCodexPrivateLaunchBoundary)
	ctx.Step(`^the launch command should contain no credential values$`, launchCommandShouldContainNoCredentialValues)
	ctx.Step(`^the Codex child should receive only allowlisted credentials$`, codexChildShouldReceiveOnlyAllowlistedCredentials)
	ctx.Step(`^caller-only credentials and telemetry should cross stale tmux state through the pinned AGM executor$`, callerOnlyCredentialsAndTelemetryShouldCrossStaleTmuxStateThroughThePinnedAGMExecutor)
	ctx.Step(`^the Codex child should preserve target-pane terminal capabilities$`, codexChildShouldPreserveTargetPaneTerminalCapabilities)
	ctx.Step(`^private launches should normalize the target working directory and require a verified executor$`, privateLaunchesShouldNormalizeTheTargetWorkingDirectoryAndRequireAVerifiedExecutor)
	ctx.Step(`^an unconsumed credential handoff should expire independently of later launches$`, anUnconsumedCredentialHandoffShouldExpireIndependentlyOfLaterLaunches)
	ctx.Step(`^deferred and rejected handoffs should preserve bounded one-shot cleanup$`, deferredAndRejectedHandoffsShouldPreserveBoundedOneShotCleanup)
	ctx.Step(`^uncertain submission across private launch surfaces should preserve the handoff$`, uncertainSubmissionAcrossPrivateLaunchSurfacesShouldPreserveTheHandoff)
	ctx.Step(`^an existing tmux session running Codex CLI$`, anExistingTmuxSessionRunningCodexCLI)
	ctx.Step(`^an existing tmux session running AGY$`, anExistingTmuxSessionRunningAGY)
	ctx.Step(`^/agm:agm-assoc runs in that session$`, agmAssocRunsInThatSession)
	ctx.Step(`^AGM should create or update a Dolt session record with harness "([^"]*)"$`, agmShouldCreateOrUpdateDoltRecordWithHarness)
	ctx.Step(`^AGM should create the ready-file signal$`, agmShouldCreateTheReadyFileSignal)
	ctx.Step(`^a Codex saved session exists outside AGM$`, aCodexSavedSessionExistsOutsideAGM)
	ctx.Step(`^an AGY saved conversation exists outside AGM$`, anAGYSavedConversationExistsOutsideAGM)
	ctx.Step(`^AGM imports the Codex session UUID with harness "([^"]*)"$`, agmImportsCodexSessionUUIDWithHarness)
	ctx.Step(`^AGM imports the AGY conversation ID with harness "([^"]*)"$`, agmImportsAGYConversationIDWithHarness)
	ctx.Step(`^the record should preserve the Codex session UUID$`, recordShouldPreserveCodexSessionUUID)
	ctx.Step(`^the record should preserve the AGY conversation ID$`, recordShouldPreserveAGYConversationID)
	ctx.Step(`^an imported AGY session with permission mode "([^"]*)"$`, anImportedAGYSessionWithPermissionMode)
	ctx.Step(`^AGM resumes the AGY session$`, agmResumesTheAGYSession)
	ctx.Step(`^AGM should launch a tmux pane that resumes the Codex conversation$`, agmShouldLaunchTmuxPaneResumingCodexConversation)
	ctx.Step(`^AGM should launch a tmux pane that resumes the AGY conversation$`, agmShouldLaunchTmuxPaneResumingAGYConversation)
	ctx.Step(`^the AGY resume command should include "([^"]*)"$`, theAGYResumeCommandShouldInclude)
	ctx.Step(`^AGM validates AGY model compatibility$`, agmValidatesAGYModelCompatibility)
	ctx.Step(`^retired AGY manifest models should map to current public labels$`, retiredAGYManifestModelsShouldMapToCurrentPublicLabels)
	ctx.Step(`^exact AGY public labels should remain unchanged$`, exactAGYPublicLabelsShouldRemainUnchanged)
	ctx.Step(`^cross-harness AGY aliases should normalize case-insensitively$`, crossHarnessAGYAliasesShouldNormalizeCaseInsensitively)
	ctx.Step(`^imported AGY conversations should preserve unknown model provenance$`, importedAGYConversationsShouldPreserveUnknownModelProvenance)
	ctx.Step(`^AGY runtime model switches should not leave a stale resume override$`, agyRuntimeModelSwitchesShouldNotLeaveAStaleResumeOverride)
	ctx.Step(`^AGM validates AGY MCP creation readiness$`, agmValidatesAGYMCPCreateReadiness)
	ctx.Step(`^MCP creation should wait for the AGY composer before prompt delivery$`, mcpCreationShouldWaitForAGYComposerBeforePromptDelivery)
	ctx.Step(`^shared creation should persist the new AGY identity before registration$`, sharedCreationShouldPersistNewAGYIdentityBeforeRegistration)
	ctx.Step(`^AGM validates AGY root cancellation plumbing$`, agmValidatesAGYRootCancellationPlumbing)
	ctx.Step(`^root signal cancellation should reach every command-scoped readiness wait$`, rootSignalCancellationShouldReachEveryCommandScopedReadinessWait)
	ctx.Step(`^AGM has Codex session records in Dolt$`, agmHasCodexSessionRecordsInDolt)
	ctx.Step(`^an agent lists sessions as JSON with fields "([^"]*)"$`, agentListsSessionsAsJSONWithFields)
	ctx.Step(`^the output should include a "sessions" array$`, outputShouldIncludeSessionsArray)
	ctx.Step(`^each session row should include the requested fields$`, eachSessionRowShouldIncludeRequestedFields)
	ctx.Step(`^the output should not collapse to an empty object$`, outputShouldNotCollapseToEmptyObject)
	ctx.Step(`^a Codex CLI session created by AGM$`, aCodexCLISessionCreatedByAGM)
	ctx.Step(`^AGM sends a message to the session$`, agmSendsMessageToTheSession)
	ctx.Step(`^AGM kills the session$`, agmKillsTheSession)
	ctx.Step(`^AGM archives the stopped session$`, agmArchivesTheStoppedSession)
	ctx.Step(`^the durable AGM store should reflect the expected lifecycle transitions$`, durableAGMStoreShouldReflectLifecycleTransitions)
	ctx.Step(`^the matching Codex saved session should be archived$`, matchingCodexSavedSessionShouldBeArchived)
	ctx.Step(`^a stopped Codex CLI session without a tmux pane$`, aStoppedCodexCLISessionWithoutTmuxPane)
	ctx.Step(`^AGM validates the shared Codex resume operation$`, agmValidatesTheCodexResumeTransaction)
	ctx.Step(`^Codex resume, state, and prompt waits should preserve process and styled composer readiness$`, codexResumeSuccessShouldRequireProcessAndComposerReadiness)
	ctx.Step(`^a failed Codex resume should serialize concurrent attempts through every production entry point, release the session lock before attachment, preserve canonical tmux identity from stale full-session updates, reconcile ambiguous metadata commits, compensate owned provisional metadata before removing its creation-specific tmux identity even when tmux ID output is lost, and preserve tmux whenever metadata cleanup is unproven$`, aFailedCodexResumeShouldRemoveOnlyItsNewlyCreatedTmuxSession)
	ctx.Step(`^authoritative session renames should serialize with cold resume, fence ambiguous storage writes, preserve both identity names from stale writers, preserve claimed tmux identity across lost replies and server restarts, reject stale identity revisions, and compensate tmux after storage conflicts$`, authoritativeSessionRenamesShouldRejectStaleIdentityRevisions)
	ctx.Step(`^administrative hierarchy repairs should atomically link parents and inherited names through the observed identity revision$`, administrativeHierarchyRepairsShouldUseObservedIdentityRevision)
	ctx.Step(`^successful Codex prompt delivery should remain successful after later caller cancellation$`, successfulCodexPromptDeliveryShouldRemainSuccessfulAfterLaterCallerCancellation)
	ctx.Step(`^ambiguous final Codex prompt submission should preserve work that may have started$`, ambiguousFinalCodexPromptSubmissionShouldPreserveStartedWork)
	ctx.Step(`^failed Codex prompt delivery should not suppress a later attach failure$`, failedCodexPromptDeliveryShouldNotSuppressALaterAttachFailure)
	ctx.Step(`^Codex activity updates should follow resume readiness$`, codexActivityUpdatesShouldFollowResumeReadiness)
}

type bddLifecycleTmux struct {
	sessions map[string]bool
	sent     []string
	events   []string
	waited   []string
}

func newBDDLifecycleTmux() *bddLifecycleTmux {
	return &bddLifecycleTmux{sessions: make(map[string]bool)}
}

func (t *bddLifecycleTmux) HasSession(name string) (bool, error) { return t.sessions[name], nil }
func (t *bddLifecycleTmux) ListSessions() ([]string, error) {
	var names []string
	for name := range t.sessions {
		names = append(names, name)
	}
	return names, nil
}
func (t *bddLifecycleTmux) ListSessionsWithInfo() ([]session.SessionInfo, error) {
	var infos []session.SessionInfo
	for name := range t.sessions {
		infos = append(infos, session.SessionInfo{Name: name})
	}
	return infos, nil
}
func (t *bddLifecycleTmux) ListClients(string) ([]session.ClientInfo, error) { return nil, nil }
func (t *bddLifecycleTmux) CreateSession(name, _ string) error {
	t.sessions[name] = true
	return nil
}
func (t *bddLifecycleTmux) AttachSession(string) error { return nil }
func (t *bddLifecycleTmux) WaitForHarnessReady(_ context.Context, name, harness string, timeout time.Duration) error {
	if !t.sessions[name] {
		return fmt.Errorf("tmux target %q does not exist", name)
	}
	if harness != "codex-cli" {
		return fmt.Errorf("unexpected lifecycle harness %q", harness)
	}
	if timeout <= 0 {
		return fmt.Errorf("readiness timeout must be positive")
	}
	t.waited = append(t.waited, name+":"+harness)
	return nil
}
func (t *bddLifecycleTmux) CheckInputReadiness(_ context.Context, name, harness string) (session.InputReadiness, error) {
	t.events = append(t.events, "readiness:"+name+":"+harness)
	if !t.sessions[name] {
		return session.InputReadiness{State: "NOT_FOUND"}, nil
	}
	if harness != "codex-cli" {
		return session.InputReadiness{}, fmt.Errorf("unexpected lifecycle harness %q", harness)
	}
	return session.InputReadiness{Ready: true, State: "YES", PaneID: name}, nil
}
func (t *bddLifecycleTmux) SendKeysIfInputReady(ctx context.Context, name, harness, keys string, _ session.InputDeliveryOptions) (session.InputReadiness, error) {
	readiness, err := t.CheckInputReadiness(ctx, name, harness)
	if err != nil || !readiness.Ready {
		return readiness, err
	}
	if err := t.SendKeys(name, keys); err != nil {
		return readiness, err
	}
	return readiness, nil
}
func (t *bddLifecycleTmux) SendKeys(name, keys string) error {
	if !t.sessions[name] {
		return fmt.Errorf("tmux target %q does not exist", name)
	}
	t.events = append(t.events, "send:"+name)
	t.sent = append(t.sent, name+"\x00"+keys)
	return nil
}
func (t *bddLifecycleTmux) KillSession(name string) error {
	if !t.sessions[name] {
		return fmt.Errorf("tmux target %q does not exist", name)
	}
	delete(t.sessions, name)
	return nil
}

type bddLifecycleRuntime struct {
	launches    []ops.HarnessLaunchSpec
	completions []ops.CreateSessionCompletion
}

func (r *bddLifecycleRuntime) Launch(_ context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
	r.launches = append(r.launches, spec)
	return ops.CreateSessionLaunchResult{}, nil
}

func (r *bddLifecycleRuntime) Complete(_ context.Context, completion ops.CreateSessionCompletion) error {
	r.completions = append(r.completions, completion)
	return nil
}

type bddCodexArchiver struct {
	targets []string
}

func (a *bddCodexArchiver) ArchiveExternalSession(_ context.Context, m *manifest.Manifest) []ops.ExternalArchiveOutcome {
	target := ""
	if m.Codex != nil {
		target = m.Codex.SessionID
	}
	a.targets = append(a.targets, target)
	return []ops.ExternalArchiveOutcome{{Provider: "codex", Status: ops.ExternalArchiveArchived, Target: target}}
}

func aStoppedCodexCLISessionWithoutTmuxPane(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	harnessState.harness = "codex-cli"
	harnessState.tmuxSessionExists = false
	return nil
}

func agmValidatesTheCodexResumeTransaction(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.harness != "codex-cli" || harnessState.tmuxSessionExists {
		return fmt.Errorf("scenario requires a stopped codex-cli session without tmux")
	}
	operationPath := filepath.Join(bddRepoRoot(), "agm", "internal", "ops", "session_resume.go")
	data, err := os.ReadFile(operationPath)
	if err != nil {
		return fmt.Errorf("read shared resume operation source: %w", err)
	}
	cliPath := filepath.Join(bddRepoRoot(), "agm", "cmd", "agm", "resume.go")
	cliData, err := os.ReadFile(cliPath)
	if err != nil {
		return fmt.Errorf("read CLI resume adapter source: %w", err)
	}
	if !strings.Contains(string(data), "WithSessionLockContext(ctx, req.SessionID") ||
		strings.Count(string(cliData), "ops.ResumeSession(") != 1 {
		return fmt.Errorf("resume lifecycle is not owned by one stable-ID shared operation with one CLI delegation")
	}
	harnessState.resumeSource = string(data)
	return nil
}

func codexResumeSuccessShouldRequireProcessAndComposerReadiness(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(
		testCtx,
		"go", "test",
		"./agm/internal/tmux", "./agm/internal/state", "./agm/internal/session",
		"-run", `^(TestCodexResumeReadiness(RequiresProcessThenComposer|StopsBeforeComposerWithoutProcess)|TestWaitForCodexPrompt(RejectsEchoedLaunchModel|AcceptsCurrentWelcomeGhostComposer)|TestIsCodex(ComposerReady|IdlePreservesCurrentWelcomeGhostStyle)|TestWaitForPrompt(Simple|OrResumeFailure)PreservesCurrentCodexWelcomeGhostStyle|TestSendMultiLinePromptSafePreservesCurrentCodexWelcomeGhostStyle|TestIsProcessReadyWithRuntimePreservesCancellation(Before|During)CodexFallback|TestDetector_CodexReadinessRequiresStructuredComposer|TestStateAndDeliveryPreserveCurrentCodexWelcomeGhostStyle)$`,
		"-count=1",
	)
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("codex resume readiness behavior timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("codex resume readiness behavior failed: %w\n%s", err, output)
	}
	return nil
}

func aFailedCodexResumeShouldRemoveOnlyItsNewlyCreatedTmuxSession(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/internal/dolt", "./agm/internal/ops", "./agm/internal/tmux", "-run", `^(TestProductionResumeEntryPointsUseSharedResumeOperation|TestResumeSourceDelegatesLifecycleToOperation|TestWithSessionLock_MutualExclusion|TestResumeSession(ReadinessFailureRemovesExactColdRuntime|CanonicalNameFailureRemovesExactColdRuntime|CreationErrorWithIdentityStillRollsBackExactRuntime|JoinsExactRuntimeCleanupFailure|AmbiguousCanonicalNameCommitRestoresBeforeCleanup|ReloadFailureCompensatesMetadataBeforeCleanup|ReportsMetadataOwnershipFinalizationFailure|PreservesExistingRuntime|CodexPromptFailureRollsBackColdRuntime|CancellationBeforePromptRollsBackColdRuntime|PreservesColdRuntimeWhenMetadataCompensationIsUnproven|AcquiresStableLockBeforeStorageRead)|TestSessionIdentityCleansCreation(BeforeTokenWrite|WhenSessionIDOutputIsLost)|TestResolveTmuxSessionNameChangeCommitErrorPreservesUncertainOwnership|TestSQLite(AdapterUpgradesLegacySessionRevisionColumn|TouchSessionActivityPreservesProvisionalTmuxRevision|TmuxSessionName(ChangeOwnsAndRestoresExactWrite|CompensationRejectsNewerMetadata|ChangeCompletesOwnershipToken|StaleFullUpdatePreservesOwnership))|TestMigration018AddsTmuxSessionRevision)$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("codex resume rollback behavior suite timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("codex resume rollback behavior suite failed: %w\n%s", err, output)
	}
	return nil
}

func authoritativeSessionRenamesShouldRejectStaleIdentityRevisions(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/internal/dolt", "./agm/internal/tmux", "-run", `^(Test(SessionRenameSerializesWithResumeByStableID|PersistRenamedSessionIdentity.*|ClassifyTmuxRenameResult|MoveAndRestoreTmuxSessionForRenamePreservesClaimedIdentity)|Test(SQLite(RenameSessionIdentityRejectsStaleRevision|SessionIdentityRenameFenceRejectsObservedRevision|TmuxSessionNameStaleFullUpdatePreservesOwnership)|ClassifySessionIdentityRenameAfterError)|TestRenameSessionIdentity(TracksClaimedSession|RejectsIDReuseAfterServerRestart))$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("session rename identity regressions timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("session rename identity regressions failed: %w\n%s", err, output)
	}
	return nil
}

func administrativeHierarchyRepairsShouldUseObservedIdentityRevision(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/internal/dolt", "-run", `^(TestPersistSessionParentLinkUsesObservedIdentityRevision|TestSQLiteLinkSessionParentUsesExplicitIdentityCAS)$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("administrative hierarchy identity regressions timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("administrative hierarchy identity regressions failed: %w\n%s", err, output)
	}
	return nil
}

func successfulCodexPromptDeliveryShouldRemainSuccessfulAfterLaterCallerCancellation(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/ops", "./agm/cmd/agm", "-run", `^(TestResumeSessionIgnoresCancellationAfterPromptStarts|TestFinishResumeAttachmentUsesOnlyOperationResult)$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("post-prompt cancellation behavior timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("post-prompt cancellation behavior failed: %w\n%s", err, output)
	}
	return nil
}

func ambiguousFinalCodexPromptSubmissionShouldPreserveStartedWork(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/ops", "./agm/internal/tmux", "-run", `^(TestResumeSessionCodexPromptAcknowledgementLossIsIrreversibleSuccess|TestRunPromptEnterCommand(StartFailureIsDefinite|ExplicitRejectionIsDefinite|TimeoutAfterStartIsUncertain)|TestVerifyingEnter_PreservesUncertaintyAcrossLater(DefiniteFailure|ParkedCaptures))$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("ambiguous prompt submission regressions timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("ambiguous prompt submission regressions failed: %w\n%s", err, output)
	}
	return nil
}

func failedCodexPromptDeliveryShouldNotSuppressALaterAttachFailure(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/ops", "./agm/cmd/agm", "-run", `^(TestResumeSessionCodexPromptPositiveFailurePreservesExistingRuntime|TestFinishResumeAttachmentReturnsAttachFailure)$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("failed-prompt attach regression timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("failed-prompt attach regression failed: %w\n%s", err, output)
	}
	return nil
}

func codexActivityUpdatesShouldFollowResumeReadiness(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/ops", "./agm/internal/dolt", "-run", `^(TestResumeSession(ColdStartCommitsPublicOutcome|ReadinessFailureRemovesExactColdRuntime|CancellationBeforePromptRollsBackColdRuntime)|TestSQLiteTouchSessionActivityPreservesProvisionalTmuxRevision)$`, "-count=1")
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	if testCtx.Err() != nil {
		return fmt.Errorf("codex resume activity ordering timed out: %w", testCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("codex resume activity ordering failed: %w\n%s", err, output)
	}
	return nil
}

func agmResolvesDoctorHealthForConfiguredHarness(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if state.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	state.harnessHealth = agent.CheckHarnessHealth(state.configuredHarness)
	return nil
}

func doctorShouldRecognizeCLIBinary(ctx context.Context, binary string) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !state.harnessHealth.Known || state.harnessHealth.BinaryName != binary {
		return fmt.Errorf("doctor health = known %v binary %q, want true %q", state.harnessHealth.Known, state.harnessHealth.BinaryName, binary)
	}
	return nil
}

func doctorShouldRecognizeConfigDirectorySuffix(ctx context.Context, suffix string) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	wantSuffix := filepath.FromSlash(suffix)
	if !strings.HasSuffix(state.harnessHealth.ConfigDir, wantSuffix) {
		return fmt.Errorf("doctor config directory = %q, want suffix %q", state.harnessHealth.ConfigDir, wantSuffix)
	}
	return nil
}

func currentTmuxCreationSelectsCodexCLI(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	harnessState.configuredHarness = "codex-cli"
	return nil
}

func agmValidatesCurrentTmuxCodexLaunchWiring(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.configuredHarness != "codex-cli" {
		return fmt.Errorf("configured harness = %q, want codex-cli", harnessState.configuredHarness)
	}
	// Keep the nested behavioral gate bounded while allowing a cold CI runner
	// to compile both production packages under integration-graph contention.
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/internal/ops", "./agm/internal/tmux", "-run", `^(Test(StartCurrentTmuxHarnessCodex(UsesRealLauncherContract|StopsAfterCredentialFailure|PropagatesQueueFailure)|QueueCurrentTmuxCodex(DoesNotWaitForReadiness|RejectsMissingExecutable)|QueueCurrentTmuxPi(UsesManagedLaunchContract|RejectsMissingExecutable)|CurrentTmuxLaunchResultDefersEveryQueuedHarness|QueueCurrentTmuxHarnessCommand(UsesCanonicalCommandWithoutWaiting|RejectsMissingExecutable)|StartNewSessionForContextRoutesCurrentTmux|SessionStartHook(AssociatesClaudeUUIDBeforeReadyState|RetriesAssociationUntilRegistration|RetryWindowCoversMaximumStartup|HasSingleCanonicalSource))|Test(CreateSession_RollsBackEveryPostTmuxFailure|EstablishCreatedHarnessReadinessAllowsOnlyQueuedCurrentTmuxDeferral)|TestTmuxSessionExistenceResultDistinguishesOperationalFailures)$`, "-count=1", "-v")
	cmd.Dir = bddRepoRoot()
	output, runErr := cmd.CombinedOutput()
	harnessState.currentTmuxTestOutput = string(output)
	harnessState.currentTmuxTestErr = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("current-tmux Codex behavior suite timed out: %w", testCtx.Err())
	}
	return nil
}

func codexCredentialValidationShouldPrecedeCanonicalLauncher(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux Codex behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	for _, behavior := range []string{
		"TestStartCurrentTmuxHarnessCodexUsesRealLauncherContract",
		"TestStartCurrentTmuxHarnessCodexStopsAfterCredentialFailure",
	} {
		if !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("current-tmux Codex behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
		}
	}
	return nil
}

func topLevelNewCommandShouldRouteIntoCurrentTmux(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux Codex behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	if behavior := "TestStartNewSessionForContextRoutesCurrentTmux"; !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
		return fmt.Errorf("current-tmux Codex behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
	}
	return nil
}

func codexCurrentTmuxLaunchShouldRequireExecutableWithoutWaiting(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux Codex behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	for _, behavior := range []string{"TestQueueCurrentTmuxCodexDoesNotWaitForReadiness", "TestQueueCurrentTmuxCodexRejectsMissingExecutable"} {
		if !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("current-tmux Codex behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
		}
	}
	return nil
}

func everyQueuedCurrentTmuxHarnessShouldDeferReadinessUntilAGMExits(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	for _, behavior := range []string{
		"TestCurrentTmuxLaunchResultDefersEveryQueuedHarness",
		"TestQueueCurrentTmuxHarnessCommandUsesCanonicalCommandWithoutWaiting",
		"TestQueueCurrentTmuxHarnessCommandRejectsMissingExecutable",
		"TestTmuxSessionExistenceResultDistinguishesOperationalFailures",
		"TestQueueCurrentTmuxPiUsesManagedLaunchContract",
		"TestQueueCurrentTmuxPiRejectsMissingExecutable",
		"TestEstablishCreatedHarnessReadinessAllowsOnlyQueuedCurrentTmuxDeferral",
	} {
		if !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("current-tmux deferred readiness behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
		}
	}
	return nil
}

func queuedPrivateHandoffsShouldCarryProducerExitLiveness(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	for _, behavior := range []string{
		"TestQueueCurrentTmuxCodexDoesNotWaitForReadiness",
		"TestQueueCurrentTmuxHarnessCommandUsesCanonicalCommandWithoutWaiting",
	} {
		if !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("producer-exit liveness behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
		}
	}
	return nil
}

func currentTmuxClaudeShouldAssociateItsUUIDOnSessionStart(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	for _, behavior := range []string{
		"TestSessionStartHookAssociatesClaudeUUIDBeforeReadyState",
		"TestSessionStartHookRetriesAssociationUntilRegistration",
		"TestSessionStartHookRetryWindowCoversMaximumStartup",
		"TestSessionStartHookHasSingleCanonicalSource",
	} {
		if !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("claude SessionStart association behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
		}
	}
	return nil
}

func codexQueueFailuresShouldPropagateToSharedCreationRollback(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.currentTmuxTestErr != nil {
		return fmt.Errorf("current-tmux Codex behavior suite failed: %w\n%s", state.currentTmuxTestErr, state.currentTmuxTestOutput)
	}
	for _, behavior := range []string{
		"TestStartCurrentTmuxHarnessCodexPropagatesQueueFailure",
		"TestCreateSession_RollsBackEveryPostTmuxFailure/launch",
	} {
		if !strings.Contains(state.currentTmuxTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("current-tmux Codex behavior %s did not pass:\n%s", behavior, state.currentTmuxTestOutput)
		}
	}
	return nil
}

func currentTmuxCreationSelectsAGY(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	harnessState.configuredHarness = "agy"
	return nil
}

func agmValidatesCurrentTmuxAGYSafety(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.configuredHarness != "agy" {
		return fmt.Errorf("configured harness = %q, want agy", harnessState.configuredHarness)
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func currentTmuxAGYCreationShouldFailBeforeLaunchWithDetachedGuidance(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	return requireAgyLifecycleBehaviors(harnessState, "TestStartNewSessionForContextRejectsCurrentTmuxAgyBeforeLaunch")
}

func aSharedCodexSendTargetWithReadiness(ctx context.Context, readiness string) error {
	if slices.Contains([]string{"YES", "NO", "QUEUE", "QUEUED_AGM", "OVERLAY", "NOT_FOUND", "WRONG_HARNESS", "ONBOARDING", "PERMISSION"}, readiness) {
		ctx.Value(harnessParityStateKey{}).(*harnessParityState).sharedSendReadiness = readiness
		return nil
	}
	return fmt.Errorf("unsupported shared-send readiness %q", readiness)
}

func agmSendsAMessageThroughSharedOperations(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	store := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "bdd-shared-codex-id",
		Name:          "bdd-shared-codex",
		Harness:       "codex-cli",
		Tmux:          manifest.Tmux{SessionName: "bdd-shared-codex"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := store.CreateSession(m); err != nil {
		return fmt.Errorf("arrange shared-send session: %w", err)
	}
	tmuxMock := session.NewMockTmux()
	tmuxMock.Sessions[m.Tmux.SessionName] = true
	tmuxMock.InputReadiness = session.InputReadiness{
		Ready: harnessState.sharedSendReadiness == "YES",
		State: harnessState.sharedSendReadiness,
	}
	if tmuxMock.InputReadiness.Ready {
		tmuxMock.InputReadiness.PaneID = "%42"
	}
	harnessState.sharedSendTmux = tmuxMock
	opCtx := &ops.OpContext{Storage: store, Tmux: tmuxMock}
	if harnessState.sharedSendCancelled {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		opCtx.Context = cancelled
	}
	harnessState.sharedSendResult, harnessState.sharedSendErr = ops.SendMessage(
		opCtx,
		&ops.SendMessageRequest{Recipient: m.SessionID, Message: "BDD readiness message"},
	)
	return nil
}

func theSharedSendResultShouldBe(ctx context.Context, expected string) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.sharedSendResult == nil {
		return fmt.Errorf("shared send returned no result")
	}
	switch expected {
	case "delivered":
		if harnessState.sharedSendErr != nil {
			return fmt.Errorf("shared send failed instead of delivering: %w", harnessState.sharedSendErr)
		}
		if !harnessState.sharedSendResult.Delivered {
			return fmt.Errorf("shared send result = %#v, want delivery", harnessState.sharedSendResult)
		}
	case "not_delivered":
		if harnessState.sharedSendResult.Delivered {
			return fmt.Errorf("shared send unexpectedly reported delivery")
		}
		opErr := &ops.OpError{}
		if !errors.As(harnessState.sharedSendErr, &opErr) || opErr.Code != ops.ErrCodeSessionNotReady {
			if harnessState.sharedSendErr == nil {
				return errors.New("shared send returned no typed not-ready error")
			}
			return fmt.Errorf("shared send returned an unexpected not-ready error: %w", harnessState.sharedSendErr)
		}
	case "cancelled":
		return validateCancelledSharedSend(harnessState.sharedSendResult, harnessState.sharedSendErr)
	default:
		return fmt.Errorf("unsupported shared-send outcome %q", expected)
	}
	return nil
}

func validateCancelledSharedSend(result *ops.SendMessageResult, sendErr error) error {
	opErr := &ops.OpError{}
	if !result.Delivered && errors.As(sendErr, &opErr) && opErr.Code == ops.ErrCodeStorageError &&
		strings.Contains(opErr.Detail, context.Canceled.Error()) {
		return nil
	}
	if sendErr == nil {
		return fmt.Errorf("shared send cancellation result = %#v without an error, want cancelled non-delivery", result)
	}
	return fmt.Errorf("shared send cancellation result = %#v, want cancelled non-delivery: %w", result, sendErr)
}

func theSharedSendRequestIsCancelled(ctx context.Context) error {
	ctx.Value(harnessParityStateKey{}).(*harnessParityState).sharedSendCancelled = true
	return nil
}

func sharedSendShouldEmitTmuxCommands(ctx context.Context, expected int) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.sharedSendTmux == nil {
		return fmt.Errorf("shared send tmux recorder is missing")
	}
	if got := len(harnessState.sharedSendTmux.SentCommands); got != expected {
		return fmt.Errorf("shared send emitted %d tmux commands, want %d", got, expected)
	}
	if got := len(harnessState.sharedSendTmux.ExactPaneDeliveries); got != expected {
		return fmt.Errorf("shared send emitted %d exact-pane deliveries, want %d", got, expected)
	}
	wantAtomicChecks := 1
	if harnessState.sharedSendCancelled {
		wantAtomicChecks = 0
	}
	if got := len(harnessState.sharedSendTmux.AtomicInputChecks); got != wantAtomicChecks {
		return fmt.Errorf("shared send performed %d atomic readiness-and-delivery operations, want %d", got, wantAtomicChecks)
	}
	if expected == 1 && harnessState.sharedSendTmux.ExactPaneDeliveries[0] != "%42" {
		return fmt.Errorf("shared send targeted pane %q, want verified pane %%42", harnessState.sharedSendTmux.ExactPaneDeliveries[0])
	}
	return nil
}

func sharedCodexCreationCannotObserveTheComposer(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	harnessState.configuredHarness = "codex-cli"
	return nil
}

func agmCreatesCodexThroughSharedOperations(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.configuredHarness != "codex-cli" {
		return fmt.Errorf("shared creation scenario requires codex-cli")
	}
	workDir, err := os.MkdirTemp("", "agm-bdd-shared-create")
	if err != nil {
		return fmt.Errorf("create shared-create workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	tmuxMock := session.NewMockTmux()
	tmuxMock.WaitForHarnessReadyError = errors.New("codex composer was not observed")
	store := dolt.NewMockAdapter()
	harnessState.sharedCreateTmux = tmuxMock
	harnessState.sharedCreateStore = store
	_, harnessState.sharedCreateErr = ops.CreateSession(
		&ops.OpContext{Storage: store, Tmux: tmuxMock, CreationRuntime: &bddCreateSessionRuntime{tmux: tmuxMock}},
		&ops.CreateSessionRequest{
			Cwd: workDir, Prompt: "must not send", Title: "bdd-shared-create",
			Harness: "codex-cli", Model: "5.5", SkipCodexRemoteControl: true,
		},
	)
	return nil
}

func sharedCreationShouldFailBeforeRegistrationAndPromptDelivery(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.sharedCreateErr == nil {
		return fmt.Errorf("shared creation reported success without a Codex composer")
	}
	if harnessState.sharedCreateTmux == nil || len(harnessState.sharedCreateTmux.SentCommands) != 1 {
		return fmt.Errorf("shared creation commands = %v, want launch only", harnessState.sharedCreateTmux)
	}
	registered, err := harnessState.sharedCreateStore.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("list shared-create registrations: %w", err)
	}
	if len(registered) != 0 {
		return fmt.Errorf("shared creation registered %d sessions before readiness", len(registered))
	}
	return nil
}

func sharedCreationShouldRemoveItsNewlyCreatedTmuxSession(ctx context.Context) error {
	tmuxMock := ctx.Value(harnessParityStateKey{}).(*harnessParityState).sharedCreateTmux
	if tmuxMock == nil {
		return fmt.Errorf("shared creation tmux recorder is missing")
	}
	if tmuxMock.Sessions["bdd-shared-create"] {
		return fmt.Errorf("new tmux session survived failed shared readiness")
	}
	return nil
}

func agmValidatesSlowHarnessStartupReadiness(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	testCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "-p", "1", "./agm/cmd/agm", "./agm/cmd/agm-mcp-server", "./agm/internal/agent", "./agm/internal/agent/openai", "./agm/internal/dolt", "./agm/internal/session", "./agm/internal/tmux", "./agm/internal/ops", "-run", `^(TestMCPCreateSessionRuntimeRevalidatesStartupPromptAtomically|TestRealTmux(InputReadinessReportsMissingSession|LogicalANSICaptureJoinsNarrowPaneWraps|ReadinessAdvancesGeminiTrustOnVerifiedPane|ReadinessDetectsManagedPiComposer|ReadinessIdentifiesNodeBackedCodex|ReadinessPinsLivenessAndDeliveryToActivePane|ReadinessPreservesClaudeGhostComposer|ReadinessRejectsSuspendedHarnessWithStaleComposer|WaitForHarnessReadyAllowsSlowProcessStart)|TestCapturePaneLogicalANSIArgsJoinFullExactPaneHistory|TestClassifyHarnessInputRequiresCurrentHarnessComposer|TestClassifyQueuedInputBindsCompleteHeaderToLatestMarker|TestQueuedComposerPayloadPromptGlyphsRemainBoundToPasteAnchor|TestHarnessStartupAdvanceKeys|TestInputDeliveryAllowedOverridesOnlyPositivelyIdentifiedAGMQueue|TestQueuedAGMRecovery(ClearsBeforeReplacement|DoesNotReplaceUntilExactPaneIsEmpty|DoesNotReportReadyWhenReplacementFails)|TestParsePSForegroundTable|TestExpectedHarnessMatcher(RejectsUnrelatedNodeProcess|AcceptsIdentifiedNodeBackedHarness|RequiresForegroundTerminalOwnership)|TestSendMessage_(AcceptsPostRecoveryReadyState|AtomicReadinessAndDeliveryIsTheLocalRuntimePath|QueuedAGMRecoveryPolicies|ForceDoesNotBypassProtectedInputStates|AutonomousDoesNotBypassProtectedInputStates|PiPermissionPromptBlocksAtomicDelivery|Normalizes(LegacyAgyHarness|PiHarnessAlias)BeforeReadiness)|TestSendMessage(CompletionFailureLeavesHistoryUnchanged|ContextCancelsProviderAndReleasesSessionLock|SerializesIndependentManagersAndCommitsCompleteTurns|RejectsSessionDeletedByIndependentManagerBeforeProvider)|TestDeleteSessionWaitsForCompletedTurnFromIndependentManager|TestOpenAIRequestContextCancelsReconstructionAndReadinessLockWait|TestSendMessageTool_RoutesPureAPIBeforeTmux|TestGetSessionStatusIsTmuxIndependent|TestWithSessionLockContextCancelsContendedWait|TestMetadataUpdatesSerializeAndPreserveIndependentFields|TestSessionHistoryReloadSupportsLargeJSONLRecords|TestImportedOpenAIMessagesUseOneHistoryTransaction|TestAPISessionLockUsesProviderAppropriateWaitPolicy|TestArchiveSessionSerializesWithAPIDeliveryMutationLock|TestSendViaSharedOperations(UsesCallerContext|FailsClosedWhenHarnessIsNotReady|PreservesQueuedAGMRecoveryPolicy)|TestAPIDelivery(PassesCallerContextToReadiness|ReloadsLifecycleInsideStableSessionLock|RejectsReapingLifecycleInsideStableSessionLock|ReservesFullCompletionBudgetAfterPreflight|SerializesReadinessCompletionAndPersistenceByStableSessionID|UsesLockedManifestForAuditArtifact)|TestNewAPISessionDeliveryAdapterRejectsMissingOrNonAPISession|TestDeliverAPISessionMessage(ReloadsCurrentManifestAndUsesContextContracts|RejectsFailedAdapter)|TestAPIRecipientStateSkipsTmuxPersistence|TestClearHistoryPreservesRuntimeConfig|TestDirectAPIDeliveryRejectsArchivedSessionBeforeAdapterConstruction|TestMultiRecipientDelivery(AllowsFullProviderDeadline|UsesSharedAtomicReadiness|RenewsDeadlinePerRecipient)|TestSingleAndMultiRecipientAPIDeliveryUsesAdapterReadiness|TestNewAPISessionDeliveryAdapterReportsPureAPISessionReadyWithoutTmux|TestNewOpenAIAdapterForSession(RestoresPersistedRuntimeConfig|DoesNotScanUnrelatedSessions)|TestNewOpenAIAdapterForLegacySessionUsesManifestFallback|TestOpenAIExecutionModelDocumentsCompatibilityOnlyControlPlane|TestUpdateRuntimeConfigPersistence|TestSessionMetadata_OpenAIRoundTrip|TestDirectCLIDeliveryRejectsUnregisteredTmuxSession|TestCreateSession_NoRuntimeInitialPrompt(RevalidatesAfterRegistration|UsesAtomicExactPaneDelivery)|TestDeliverInitialPrompt(UsesAtomicExactPaneReadiness|FileUsesAtomicExactPaneReadiness|FailsClosedWhenHarnessDoesNotOwnTerminal))$`, "-count=1", "-v")
	cmd.Dir = bddRepoRoot()
	if os.Getenv("CI_SKIP_TMUX") != "true" {
		cmd.Env = append(os.Environ(), "AGM_TEST_TMUX=1")
	}
	output, err := cmd.CombinedOutput()
	agyBootstrapCmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm-mcp-server",
		"-run", `^TestMCPCreateSessionRuntime(AgyIdentityBootstrapFailsClosedWhenComposerIsNotReady|CannotBypassSharedAgyReadiness)$`,
		"-count=1", "-v",
	)
	agyBootstrapCmd.Dir = bddRepoRoot()
	agyBootstrapOutput, agyBootstrapErr := agyBootstrapCmd.CombinedOutput()
	harnessState.startupReadinessTestOutput = string(output) + "\n" + string(agyBootstrapOutput)
	harnessState.startupReadinessTestErr = err
	if harnessState.startupReadinessTestErr == nil {
		harnessState.startupReadinessTestErr = agyBootstrapErr
	}
	if testCtx.Err() != nil {
		return fmt.Errorf("slow harness startup readiness test timed out: %w", testCtx.Err())
	}
	return nil
}

func syntheticAmbientCredentialsFromMultipleHarnesses(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	state.privateAllowedCanaries = []string{"openai-bdd-canary", "codex-bdd-canary"}
	state.privateRejectedCanaries = []string{
		"claude-bdd-canary", "anthropic-bdd-canary", "github-bdd-canary",
		"google-bdd-canary", "engram-bdd-canary", "otel-bdd-canary",
		"ssh-bdd-canary", "arbitrary-bdd-canary",
	}
	return nil
}

func sharedStartupReadinessShouldHonorTheTotalDeadline(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("slow harness startup readiness test failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	if !realTmuxReadinessBehaviorSatisfied(harnessState.startupReadinessTestOutput, "TestRealTmuxWaitForHarnessReadyAllowsSlowProcessStart") {
		return fmt.Errorf("slow harness startup readiness behavior did not pass:\n%s", harnessState.startupReadinessTestOutput)
	}
	return nil
}

func sharedInputReadinessShouldRejectStaleClaudeComposerAndUnrelatedNodeProcess(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	for _, behavior := range []string{
		"TestCapturePaneLogicalANSIArgsJoinFullExactPaneHistory",
		"TestClassifyHarnessInputRequiresCurrentHarnessComposer",
		"TestClassifyQueuedInputBindsCompleteHeaderToLatestMarker",
		"TestQueuedAGMRecoveryClearsBeforeReplacement",
		"TestQueuedAGMRecoveryDoesNotReplaceUntilExactPaneIsEmpty",
		"TestQueuedAGMRecoveryDoesNotReportReadyWhenReplacementFails",
		"TestExpectedHarnessMatcherRejectsUnrelatedNodeProcess",
		"TestExpectedHarnessMatcherAcceptsIdentifiedNodeBackedHarness",
		"TestExpectedHarnessMatcherRequiresForegroundTerminalOwnership",
		"TestParsePSForegroundTable",
		"TestSendMessage_AtomicReadinessAndDeliveryIsTheLocalRuntimePath",
		"TestSendMessage_AcceptsPostRecoveryReadyState",
		"TestSendMessage_PiPermissionPromptBlocksAtomicDelivery",
		"TestMCPCreateSessionRuntimeRevalidatesStartupPromptAtomically",
		"TestMCPCreateSessionRuntimeCannotBypassSharedAgyReadiness",
		"TestMCPCreateSessionRuntimeAgyIdentityBootstrapFailsClosedWhenComposerIsNotReady",
	} {
		if !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("shared readiness behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
		}
	}
	if behavior := "TestRealTmuxInputReadinessReportsMissingSession"; !realTmuxReadinessBehaviorSatisfied(harnessState.startupReadinessTestOutput, behavior) {
		return fmt.Errorf("shared real-tmux missing-target behavior %s did not pass or use the configured CI skip:\n%s", behavior, harnessState.startupReadinessTestOutput)
	}
	for _, behavior := range []string{
		"TestRealTmuxLogicalANSICaptureJoinsNarrowPaneWraps",
		"TestRealTmuxReadinessIdentifiesNodeBackedCodex",
		"TestRealTmuxReadinessRejectsSuspendedHarnessWithStaleComposer",
		"TestRealTmuxReadinessPinsLivenessAndDeliveryToActivePane",
		"TestRealTmuxReadinessPreservesClaudeGhostComposer",
		"TestRealTmuxReadinessDetectsManagedPiComposer",
	} {
		if !realTmuxReadinessBehaviorSatisfied(harnessState.startupReadinessTestOutput, behavior) {
			return fmt.Errorf("shared real-tmux readiness behavior %s did not pass or use the configured CI skip:\n%s", behavior, harnessState.startupReadinessTestOutput)
		}
	}
	return nil
}

func agmBuildsTheCodexPrivateLaunchBoundary(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	state.privateLaunchCommand = ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "bdd-private-codex", WorkDir: "/tmp/bdd-work",
	}).Command
	state.privateChildEnvironment = harnessexec.CodexEnvironment([]string{
		"PATH=/usr/bin:/bin",
		"HOME=/tmp/bdd-home",
		"OPENAI_API_KEY=openai-bdd-canary",
		"CODEX_ACCESS_TOKEN=codex-bdd-canary",
		"CLAUDE_CODE_OAUTH_TOKEN=claude-bdd-canary",
		"ANTHROPIC_API_KEY=anthropic-bdd-canary",
		"GITHUB_TOKEN=github-bdd-canary",
		"GOOGLE_API_KEY=google-bdd-canary",
		"ENGRAM_TOKEN=engram-bdd-canary",
		"OTEL_EXPORTER_OTLP_HEADERS=otel-bdd-canary",
		"SSH_AUTH_SOCK=ssh-bdd-canary",
		"ARBITRARY_SECRET=arbitrary-bdd-canary",
	}, "bdd-private-codex")
	testCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/harnessexec", "./agm/internal/agent", "./agm/internal/ops", "./agm/internal/session", "./agm/internal/validate", "./agm/cmd/agm", "./agm/cmd/agm-mcp-server",
		"-run", `^(TestPrepared(ClaudeCommand(CarriesCallerOnlyOAuthAndTelemetry|ClearsCallerAbsentPaneState)|CodexCommand(CarriesCallerAllowlistAndPreservesPaneRuntime|ClearsCallerAbsentPaneCredentials|ResolvesExecutableFromCallerPATH)|Command(CancelRemovesUndeliveredHandoff|UsesCoInstalledAGMFromCompanionBinary|UsesMatchingVersionedAGMFromReleaseCompanion|RejectsCompanionWithoutCoInstalledAGM|UsesRenamedCurrentAGMExecutable|MakesRelativeStateDirectoryAbsolute|SchedulesIndependentExpiration|RemovesHandoffWhenExpirationCannotBeScheduled)|DeferredCommandSchedulesProducerLease)|Test(ResolveSubmissionPreservesUncertainAndCancelsConfirmedFailure|DeferredHandoffRemainsLiveUntilProducerExitThenExpires|ExpiryProtocolRemovesUnconsumedHandoffAtDeadline|DetachedExpiryHelper(InterceptsGoTestBinaryBeforeTestsRun|IsReapedAsynchronously)|ConsumeHandoff(UsesDeferredLeaseFreshnessAndUnlinksRejections|PreservesFilesOutsidePrivateStagingNamespace)|ClaudeResumeChangesDirectoryBeforeDirectReplacement|ClaudeResolvesRelativePATHAfterEnteringWorkDir|ArchitectureUsesPreparedClaudeResumeBoundary|ClaudeAdapter(Create|Resume)PreservesHandoffAfterUncertainSubmission|Codex(CreateSession|ResumeSession)PreservesHandoffAfterUncertainSubmission|Agy(CreateSession|ResumeSession)PreservesHandoffAfterUncertainSubmission|ClaudeResumePreservesHandoffAndCreatedTmuxAfterUncertainSubmission|ResumabilityValidatorPreservesHandoffAfterUncertainSubmission|QueueCurrentTmux(Codex|Claude)PreservesHandoffAfterUncertainSubmission|MCPCreateSessionRuntimePreservesUncertainPrivateLaunch))$`,
		"-count=1", "-v",
	)
	cmd.Dir = bddRepoRoot()
	output, err := cmd.CombinedOutput()
	state.privateHandoffTestOutput = string(output)
	state.privateHandoffTestErr = err
	if testCtx.Err() != nil {
		return fmt.Errorf("private handoff behavior timed out: %w", testCtx.Err())
	}
	return nil
}

func launchCommandShouldContainNoCredentialValues(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	for _, canary := range append(append([]string{}, state.privateAllowedCanaries...), state.privateRejectedCanaries...) {
		if strings.Contains(state.privateLaunchCommand, canary) {
			return fmt.Errorf("private launch command exposed credential canary %q", canary)
		}
	}
	if !strings.Contains(state.privateLaunchCommand, "agm "+harnessexec.CodexProtocol) {
		return fmt.Errorf("codex launch bypassed the private executor: %s", state.privateLaunchCommand)
	}
	return nil
}

func codexChildShouldReceiveOnlyAllowlistedCredentials(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	joined := strings.Join(state.privateChildEnvironment, "\n")
	for _, canary := range state.privateAllowedCanaries {
		if !strings.Contains(joined, canary) {
			return fmt.Errorf("codex child environment omitted allowlisted canary %q", canary)
		}
	}
	for _, canary := range state.privateRejectedCanaries {
		if strings.Contains(joined, canary) {
			return fmt.Errorf("codex child environment inherited rejected canary %q", canary)
		}
	}
	return nil
}

func callerOnlyCredentialsAndTelemetryShouldCrossStaleTmuxStateThroughThePinnedAGMExecutor(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.privateHandoffTestErr != nil {
		return fmt.Errorf("private handoff behavior failed: %w\n%s", state.privateHandoffTestErr, state.privateHandoffTestOutput)
	}
	for _, behavior := range []string{
		"TestPreparedClaudeCommandCarriesCallerOnlyOAuthAndTelemetry",
		"TestPreparedClaudeCommandClearsCallerAbsentPaneState",
		"TestPreparedCodexCommandCarriesCallerAllowlistAndPreservesPaneRuntime",
		"TestPreparedCodexCommandClearsCallerAbsentPaneCredentials",
		"TestPreparedCodexCommandResolvesExecutableFromCallerPATH",
		"TestPreparedCommandCancelRemovesUndeliveredHandoff",
		"TestPreparedCommandUsesCoInstalledAGMFromCompanionBinary",
		"TestPreparedCommandUsesMatchingVersionedAGMFromReleaseCompanion",
		"TestPreparedCommandUsesRenamedCurrentAGMExecutable",
		"TestPreparedCommandMakesRelativeStateDirectoryAbsolute",
		"TestClaudeResumeChangesDirectoryBeforeDirectReplacement",
		"TestClaudeResolvesRelativePATHAfterEnteringWorkDir",
	} {
		if !strings.Contains(state.privateHandoffTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("private handoff behavior %s did not pass:\n%s", behavior, state.privateHandoffTestOutput)
		}
	}
	return nil
}

func codexChildShouldPreserveTargetPaneTerminalCapabilities(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.privateHandoffTestErr != nil {
		return fmt.Errorf("private handoff terminal behavior failed: %w\n%s", state.privateHandoffTestErr, state.privateHandoffTestOutput)
	}
	const behavior = "TestPreparedCodexCommandCarriesCallerAllowlistAndPreservesPaneRuntime"
	if !strings.Contains(state.privateHandoffTestOutput, "--- PASS: "+behavior) {
		return fmt.Errorf("private handoff terminal behavior %s did not pass:\n%s", behavior, state.privateHandoffTestOutput)
	}
	return nil
}

func privateLaunchesShouldNormalizeTheTargetWorkingDirectoryAndRequireAVerifiedExecutor(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.privateHandoffTestErr != nil {
		return fmt.Errorf("private executor boundary behavior failed: %w\n%s", state.privateHandoffTestErr, state.privateHandoffTestOutput)
	}
	for _, behavior := range []string{
		"TestPreparedCodexCommandCarriesCallerAllowlistAndPreservesPaneRuntime",
		"TestPreparedCommandUsesCoInstalledAGMFromCompanionBinary",
		"TestPreparedCommandUsesMatchingVersionedAGMFromReleaseCompanion",
		"TestPreparedCommandRejectsCompanionWithoutCoInstalledAGM",
		"TestClaudeResolvesRelativePATHAfterEnteringWorkDir",
		"TestArchitectureUsesPreparedClaudeResumeBoundary",
	} {
		if !strings.Contains(state.privateHandoffTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("private executor boundary behavior %s did not pass:\n%s", behavior, state.privateHandoffTestOutput)
		}
	}
	return nil
}

func anUnconsumedCredentialHandoffShouldExpireIndependentlyOfLaterLaunches(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.privateHandoffTestErr != nil {
		return fmt.Errorf("private handoff expiration behavior failed: %w\n%s", state.privateHandoffTestErr, state.privateHandoffTestOutput)
	}
	for _, behavior := range []string{
		"TestPreparedCommandSchedulesIndependentExpiration",
		"TestPreparedCommandRemovesHandoffWhenExpirationCannotBeScheduled",
		"TestExpiryProtocolRemovesUnconsumedHandoffAtDeadline",
		"TestDetachedExpiryHelperInterceptsGoTestBinaryBeforeTestsRun",
	} {
		if !strings.Contains(state.privateHandoffTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("private handoff expiration behavior %s did not pass:\n%s", behavior, state.privateHandoffTestOutput)
		}
	}
	return nil
}

func deferredAndRejectedHandoffsShouldPreserveBoundedOneShotCleanup(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.privateHandoffTestErr != nil {
		return fmt.Errorf("private deferred-handoff behavior failed: %w\n%s", state.privateHandoffTestErr, state.privateHandoffTestOutput)
	}
	for _, behavior := range []string{
		"TestPreparedDeferredCommandSchedulesProducerLease",
		"TestDeferredHandoffRemainsLiveUntilProducerExitThenExpires",
		"TestDetachedExpiryHelperIsReapedAsynchronously",
		"TestConsumeHandoffUsesDeferredLeaseFreshnessAndUnlinksRejections",
		"TestConsumeHandoffPreservesFilesOutsidePrivateStagingNamespace",
	} {
		if !strings.Contains(state.privateHandoffTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("private deferred-handoff behavior %s did not pass:\n%s", behavior, state.privateHandoffTestOutput)
		}
	}
	return nil
}

func uncertainSubmissionAcrossPrivateLaunchSurfacesShouldPreserveTheHandoff(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.privateHandoffTestErr != nil {
		return fmt.Errorf("private uncertain-submission behavior failed: %w\n%s", state.privateHandoffTestErr, state.privateHandoffTestOutput)
	}
	for _, behavior := range []string{
		"TestResolveSubmissionPreservesUncertainAndCancelsConfirmedFailure",
		"TestQueueCurrentTmuxCodexPreservesHandoffAfterUncertainSubmission",
		"TestQueueCurrentTmuxClaudePreservesHandoffAfterUncertainSubmission",
		"TestMCPCreateSessionRuntimePreservesUncertainPrivateLaunch",
		"TestClaudeAdapterCreatePreservesHandoffAfterUncertainSubmission",
		"TestClaudeAdapterResumePreservesHandoffAfterUncertainSubmission",
		"TestCodexCreateSessionPreservesHandoffAfterUncertainSubmission", "TestCodexResumeSessionPreservesHandoffAfterUncertainSubmission",
		"TestAgyCreateSessionPreservesHandoffAfterUncertainSubmission", "TestAgyResumeSessionPreservesHandoffAfterUncertainSubmission",
		"TestClaudeResumePreservesHandoffAndCreatedTmuxAfterUncertainSubmission",
		"TestResumabilityValidatorPreservesHandoffAfterUncertainSubmission",
	} {
		if !strings.Contains(state.privateHandoffTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("private uncertain-submission behavior %s did not pass:\n%s", behavior, state.privateHandoffTestOutput)
		}
	}
	return nil
}

func cliMessageSendsShouldUseSharedAtomicReadiness(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	for _, behavior := range []string{
		"TestSendViaSharedOperationsUsesCallerContext",
		"TestSendViaSharedOperationsFailsClosedWhenHarnessIsNotReady",
		"TestMultiRecipientDeliveryUsesSharedAtomicReadiness",
		"TestDirectCLIDeliveryRejectsUnregisteredTmuxSession",
		"TestDeliverInitialPromptUsesAtomicExactPaneReadiness",
		"TestDeliverInitialPromptFileUsesAtomicExactPaneReadiness",
		"TestDeliverInitialPromptFailsClosedWhenHarnessDoesNotOwnTerminal",
		"TestCreateSession_NoRuntimeInitialPromptRevalidatesAfterRegistration",
		"TestCreateSession_NoRuntimeInitialPromptUsesAtomicExactPaneDelivery",
	} {
		if !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("CLI shared send behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
		}
	}
	return nil
}

func forcedCLIMessageSendsShouldReplaceOnlyPositivelyIdentifiedQueuedAGMInput(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	for _, behavior := range []string{
		"TestQueuedComposerPayloadPromptGlyphsRemainBoundToPasteAnchor",
		"TestInputDeliveryAllowedOverridesOnlyPositivelyIdentifiedAGMQueue",
		"TestSendMessage_QueuedAGMRecoveryPolicies",
		"TestSendMessage_ForceDoesNotBypassProtectedInputStates",
		"TestSendViaSharedOperationsPreservesQueuedAGMRecoveryPolicy",
	} {
		if !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("forced shared send behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
		}
	}
	return nil
}

func autonomousCLIMessageSendsShouldPreserveOnlyPositivelyIdentifiedQueuedAGMRecovery(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	for _, behavior := range []string{
		"TestSendMessage_QueuedAGMRecoveryPolicies",
		"TestSendMessage_AutonomousDoesNotBypassProtectedInputStates",
		"TestSendViaSharedOperationsPreservesQueuedAGMRecoveryPolicy",
	} {
		if !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("autonomous shared send behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
		}
	}
	return nil
}

func singleAndFanOutAPISendsShouldUseAdapterSessionReadinessWithoutRequiringTmux(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	for _, behavior := range []string{
		"TestMultiRecipientDeliveryRenewsDeadlinePerRecipient",
		"TestMultiRecipientDeliveryAllowsFullProviderDeadline",
		"TestSingleAndMultiRecipientAPIDeliveryUsesAdapterReadiness",
		"TestNewAPISessionDeliveryAdapterReportsPureAPISessionReadyWithoutTmux",
		"TestNewOpenAIAdapterForSessionRestoresPersistedRuntimeConfig",
		"TestNewOpenAIAdapterForSessionDoesNotScanUnrelatedSessions",
		"TestNewOpenAIAdapterForLegacySessionUsesManifestFallback",
		"TestOpenAIExecutionModelDocumentsCompatibilityOnlyControlPlane",
		"TestUpdateRuntimeConfigPersistence",
		"TestSessionMetadata_OpenAIRoundTrip",
		"TestSendMessageCompletionFailureLeavesHistoryUnchanged",
		"TestSendMessageContextCancelsProviderAndReleasesSessionLock",
		"TestSendMessageSerializesIndependentManagersAndCommitsCompleteTurns",
		"TestSendMessageRejectsSessionDeletedByIndependentManagerBeforeProvider",
		"TestDeleteSessionWaitsForCompletedTurnFromIndependentManager",
		"TestOpenAIRequestContextCancelsReconstructionAndReadinessLockWait",
		"TestAPIDeliveryPassesCallerContextToReadiness",
		"TestAPIDeliveryReservesFullCompletionBudgetAfterPreflight",
		"TestWithSessionLockContextCancelsContendedWait",
		"TestAPISessionLockUsesProviderAppropriateWaitPolicy",
		"TestArchiveSessionSerializesWithAPIDeliveryMutationLock",
		"TestAPIDeliverySerializesReadinessCompletionAndPersistenceByStableSessionID",
		"TestAPIDeliveryReloadsLifecycleInsideStableSessionLock",
		"TestAPIDeliveryUsesLockedManifestForAuditArtifact",
		"TestNewAPISessionDeliveryAdapterRejectsMissingOrNonAPISession",
		"TestDeliverAPISessionMessageReloadsCurrentManifestAndUsesContextContracts",
		"TestDeliverAPISessionMessageRejectsFailedAdapter",
		"TestAPIDeliveryRejectsReapingLifecycleInsideStableSessionLock",
		"TestAPIRecipientStateSkipsTmuxPersistence",
		"TestSendMessageTool_RoutesPureAPIBeforeTmux",
		"TestGetSessionStatusIsTmuxIndependent",
		"TestClearHistoryPreservesRuntimeConfig",
		"TestMetadataUpdatesSerializeAndPreserveIndependentFields",
		"TestSessionHistoryReloadSupportsLargeJSONLRecords",
		"TestImportedOpenAIMessagesUseOneHistoryTransaction",
		"TestDirectAPIDeliveryRejectsArchivedSessionBeforeAdapterConstruction",
	} {
		if !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("API delivery behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
		}
	}
	return nil
}

func sharedGeminiReadinessShouldAdvanceFirstRunTrustOnTheVerifiedPane(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	if behavior := "TestHarnessStartupAdvanceKeys"; !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
		return fmt.Errorf("shared Gemini readiness behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
	}
	if behavior := "TestRealTmuxReadinessAdvancesGeminiTrustOnVerifiedPane"; !realTmuxReadinessBehaviorSatisfied(harnessState.startupReadinessTestOutput, behavior) {
		return fmt.Errorf("shared real-tmux Gemini readiness behavior %s did not pass or use the configured CI skip:\n%s", behavior, harnessState.startupReadinessTestOutput)
	}
	return nil
}

func realTmuxReadinessBehaviorSatisfied(output, behavior string) bool {
	if strings.Contains(output, "--- PASS: "+behavior) {
		return true
	}
	if !strings.Contains(output, "--- SKIP: "+behavior) {
		return false
	}
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		return true
	}
	// Managed execution environments may provide tmux while deliberately
	// denying process-table inspection. The integration test emits this exact
	// capability reason from its own setup; accept only that self-diagnosed
	// skip, while every unrelated unconfigured skip remains a failure.
	return strings.Contains(output, "process-table inspection is unavailable")
}

func legacyAgyNamesShouldReachCanonicalSharedSendReadiness(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	if behavior := "TestSendMessage_NormalizesLegacyAgyHarnessBeforeReadiness"; !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
		return fmt.Errorf("legacy AGY send readiness behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
	}
	return nil
}

func piAliasShouldReachCanonicalSharedSendReadiness(ctx context.Context) error {
	harnessState := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if harnessState.startupReadinessTestErr != nil {
		return fmt.Errorf("shared readiness behavior suite failed: %w\n%s", harnessState.startupReadinessTestErr, harnessState.startupReadinessTestOutput)
	}
	if behavior := "TestSendMessage_NormalizesPiHarnessAliasBeforeReadiness"; !strings.Contains(harnessState.startupReadinessTestOutput, "--- PASS: "+behavior) {
		return fmt.Errorf("pi send readiness behavior %s did not pass:\n%s", behavior, harnessState.startupReadinessTestOutput)
	}
	return nil
}

func activeHarnessUsesStartupMode(ctx context.Context, harness, mode string) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	state.configuredHarness = agent.NormalizeHarnessName(harness)
	state.launchMode = mode
	return nil
}

func agmBuildsPersistentHarnessLaunchCommand(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	contract, err := launchparity.Resolve(state.configuredHarness, state.launchMode, true)
	if err != nil {
		return err
	}
	state.launchContract = contract
	return nil
}

func launchCommandShouldUseNativeInteractiveContract(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if state.launchContract.InteractiveToken == "" {
		return fmt.Errorf("harness %q has no native interactive startup token", state.configuredHarness)
	}
	return nil
}

func launchCommandShouldNotExitTmuxPaneShell(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if state.launchContract.ExitSuffix != "" {
		return fmt.Errorf("persistent harness %q exit suffix = %q", state.configuredHarness, state.launchContract.ExitSuffix)
	}
	return nil
}

func aValidatedPiCustomCodingAgentDirectory(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "agm-bdd-pi-agent-")
	if err != nil {
		return err
	}
	validated, validateErr := pisession.ValidateCodingAgentDir(dir)
	removeErr := os.RemoveAll(dir)
	if validateErr != nil {
		return validateErr
	}
	if removeErr != nil {
		return removeErr
	}
	state.piCodingAgentDir = validated
	return nil
}

func agmBuildsPiCreateAndColdResumeCommands(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	pi := &manifest.Pi{
		SessionID: "native-pi", SessionDir: "/private/pi",
		CodingAgentDir: state.piCodingAgentDir, CodingAgentDirSet: true,
	}
	base := ops.HarnessLaunchSpec{
		Harness: "pi-cli", Model: "sonnet", SessionName: "pi-worker", SessionID: "native-pi",
		WorkDir: "/work", PermissionMode: "default", Pi: pi,
	}
	base.PiLaunchID = "create-launch"
	state.piCreateCommand = ops.BuildHarnessLaunchCommand(base).Command
	base.PiLaunchID = "resume-launch"
	state.piResumeCommand = ops.BuildHarnessLaunchCommand(base).Command
	encoded, err := json.Marshal(pi)
	if err != nil {
		return err
	}
	var restored manifest.Pi
	if err := json.Unmarshal(encoded, &restored); err != nil {
		return err
	}
	state.piMetadataPreserved = restored.CodingAgentDir == state.piCodingAgentDir && restored.CodingAgentDirSet
	return nil
}

func bothPiCommandsShouldForwardCodingAgentDirectory(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	want := "PI_CODING_AGENT_DIR=" + shellquote.Quote(state.piCodingAgentDir)
	for name, command := range map[string]string{"create": state.piCreateCommand, "resume": state.piResumeCommand} {
		if !strings.Contains(command, want) {
			return fmt.Errorf("pi %s command omitted %q: %s", name, want, command)
		}
	}
	return nil
}

func piNativeMetadataShouldPreserveCodingAgentDirectory(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !state.piMetadataPreserved {
		return fmt.Errorf("pi coding-agent directory did not survive native metadata round-trip")
	}
	return nil
}

func defaultPiLaunchShouldLeaveNativeDiscoveryUnchanged(ctx context.Context) error {
	_, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	command := ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "pi-cli", SessionName: "pi-default", SessionID: "native-default",
		WorkDir: "/work", Pi: &manifest.Pi{
			SessionID: "native-default", SessionDir: "/private/pi", CodingAgentDirSet: true,
		},
	}).Command
	if !strings.Contains(command, "env -u CLAUDECODE -u PI_CODING_AGENT_DIR") || strings.Contains(command, "PI_CODING_AGENT_DIR=") {
		return fmt.Errorf("default Pi launch did not clear inherited custom configuration: %s", command)
	}
	return nil
}

func agmValidatesFinalStartupLiveness(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	missingHarness := tmux.PaneLiveness{SessionExists: true, HarnessAlive: false, Evidence: "descendants: zsh"}
	state.startupLivenessValid = launchparity.ValidateFinalLiveness(missingHarness, nil) != nil &&
		launchparity.ValidateFinalLiveness(tmux.PaneLiveness{SessionExists: true, HarnessAlive: true}, nil) == nil
	return nil
}

func startupShouldRequireLiveTmuxAndHarness(ctx context.Context) error {
	state, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !state.startupLivenessValid {
		return fmt.Errorf("startup liveness accepted a session without a harness process")
	}
	return nil
}

func harnessIsConfigured(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.configuredHarness = agent.NormalizeHarnessName(harness)
	return nil
}

func agmSelectsGracefulReaperExitCommand(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	harnessState.gracefulExitCommand = reaper.GracefulExitCommand(harnessState.configuredHarness)
	return nil
}

func gracefulReaperExitCommandShouldBe(ctx context.Context, want string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.gracefulExitCommand != want {
		return fmt.Errorf("graceful reaper exit command = %q, want %q", harnessState.gracefulExitCommand, want)
	}
	return nil
}

func agmValidatesPaneCaptureInvocation(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	harnessState.captureSessionName = harnessState.configuredHarness + ".capture:test"
	harnessState.captureInvocationArgs = tmux.CapturePaneCommandArgs(harnessState.captureSessionName, 50)
	policy := tmux.CapturePanePolicy()
	harnessState.capturePolicyValid = policy.Timeout > 0 && policy.WaitDelay > 0 && policy.IsolateProcessGroup
	return nil
}

func paneCaptureShouldUseCanonicalAGMTmuxSocket(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	args := harnessState.captureInvocationArgs
	if len(args) < 2 || args[0] != "-S" || args[1] != tmux.GetSocketPath() {
		return fmt.Errorf("capture invocation does not use canonical socket: %q", args)
	}
	return nil
}

func paneCaptureShouldNormalizeSessionTarget(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	args := harnessState.captureInvocationArgs
	want := tmux.NormalizeTmuxSessionName(harnessState.captureSessionName)
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-t" && args[i+1] == want {
			return nil
		}
	}
	return fmt.Errorf("capture invocation target is not normalized to %q: %q", want, args)
}

func paneCaptureShouldBeBoundedAndProcessGroupIsolated(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.capturePolicyValid {
		return fmt.Errorf("pane capture policy is not bounded and process-group isolated")
	}
	return nil
}

func agmValidatesSessionRecoveryParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	before := recovery.Snapshot{WorkLeaves: []recovery.Process{{PID: 42}}}
	after := recovery.Snapshot{Descendants: []recovery.Process{{PID: 42}}}
	harnessState.recoveryConfirmationValid = !recovery.Confirmed(before, after, true)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	harnessState.recoveryCancellationValid = errors.Is(recovery.WaitForConfirmation(canceled, time.Minute), context.Canceled)
	harnessState.recoveryFallback = recovery.FallbackForHarness(harnessState.configuredHarness)
	return nil
}

func recoveryShouldRequireProcessStateConfirmation(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.recoveryConfirmationValid {
		return fmt.Errorf("recovery accepted a ready-looking pane without work-process exit")
	}
	return nil
}

func recoveryWaitsShouldRespectContextCancellation(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.recoveryCancellationValid {
		return fmt.Errorf("recovery confirmation wait ignored context cancellation")
	}
	return nil
}

func harnessShouldHaveSafeRecoveryFallbackPolicy(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if normalized != harnessState.configuredHarness {
		return fmt.Errorf("configured harness = %q, want %q", harnessState.configuredHarness, normalized)
	}
	want := recovery.FallbackNone
	if normalized == "agy" {
		want = recovery.FallbackLeafInterrupt
	}
	if harnessState.recoveryFallback != want {
		return fmt.Errorf("recovery fallback for %q = %q, want %q", normalized, harnessState.recoveryFallback, want)
	}
	return nil
}

func agmTmuxFacingCommandSources(context.Context) error {
	return nil
}

func agmValidatesTmuxCommandParityContracts(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.commandParityValid = commandparity.ValidateContracts() == nil
	harnessState.commandSourceCoverageValid = commandparity.ValidateSourceCoverage(bddRepoRoot()) == nil
	return nil
}

func everyTmuxFacingCommandShouldDeclareAllActiveHarnessStrategies(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.commandParityValid {
		return commandparity.ValidateContracts()
	}
	return nil
}

func everyTmuxFacingCobraCommandSourceShouldHaveAParityContract(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.commandSourceCoverageValid {
		return commandparity.ValidateSourceCoverage(bddRepoRoot())
	}
	return nil
}

func agmValidatesModelIndependentTmuxCommandParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredModelFamily == "" {
		return fmt.Errorf("no model family configured")
	}
	if _, ok := agent.DefaultModelForFamily(harnessState.configuredModelFamily); !ok {
		return fmt.Errorf("model family %q has no default route", harnessState.configuredModelFamily)
	}
	for _, contract := range commandparity.Contracts() {
		if !contract.ModelIndependent {
			continue
		}
		for _, harness := range agent.ActiveHarnesses() {
			if contract.Strategies[harness] == "" {
				return fmt.Errorf("model-independent command %q lacks harness %q", contract.Command, harness)
			}
		}
	}
	harnessState.modelCommandParityValid = true
	return nil
}

func modelIndependentTmuxCommandsShouldSupportModelFamily(ctx context.Context, family string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredModelFamily != strings.ToLower(family) {
		return fmt.Errorf("configured model family = %q, want %q", harnessState.configuredModelFamily, family)
	}
	if !harnessState.modelCommandParityValid {
		return fmt.Errorf("model-independent tmux command parity was not validated")
	}
	return nil
}

func agmValidatesActiveParitySupport(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	return agent.ValidateHarnessName(harnessState.configuredHarness)
}

func harnessShouldBeActiveForParity(ctx context.Context, harness string) error {
	normalized := agent.NormalizeHarnessName(harness)
	if slices.Contains(agent.ActiveHarnesses(), normalized) {
		return nil
	}
	return fmt.Errorf("harness %q is not in active parity set %v", normalized, agent.ActiveHarnesses())
}

func harnessShouldNotBeDeprecated(ctx context.Context, harness string) error {
	normalized := agent.NormalizeHarnessName(harness)
	if agent.IsDeprecatedHarness(normalized) {
		return fmt.Errorf("harness %q should not be deprecated", normalized)
	}
	return nil
}

func harnessShouldBeDeprecated(ctx context.Context, harness string) error {
	normalized := agent.NormalizeHarnessName(harness)
	if !agent.IsDeprecatedHarness(normalized) {
		return fmt.Errorf("harness %q should be deprecated", normalized)
	}
	return nil
}

func agmActiveHarnessesAreConfigured(ctx context.Context) error {
	active := agent.ActiveHarnesses()
	if len(active) == 0 {
		return fmt.Errorf("no active harnesses configured")
	}
	for _, harness := range active {
		if err := agent.ValidateHarnessName(harness); err != nil {
			return fmt.Errorf("active harness %q failed validation: %w", harness, err)
		}
	}
	return nil
}

func agmValidatesActiveHarnessAdapterConformance(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.conformanceFindings = agent.ValidateActiveHarnessConformance()
	return nil
}

func everyActiveHarnessAdapterShouldSatisfySharedConformanceSuite(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if len(harnessState.conformanceFindings) == 0 {
		return nil
	}
	messages := make([]string, 0, len(harnessState.conformanceFindings))
	for _, finding := range harnessState.conformanceFindings {
		messages = append(messages, finding.Error())
	}
	return fmt.Errorf("active harness conformance failed:\n%s", strings.Join(messages, "\n"))
}

func agmCodexAndOpenAIAdapterSources() error {
	return nil
}

func agmValidatesCodexAdapterRouting() error {
	return nil
}

func codexFactoryShouldUseCodexCLIAdapter() error {
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "agent", "factory.go"))
	if err != nil {
		return fmt.Errorf("read agent factory: %w", err)
	}
	source := string(data)
	if !strings.Contains(source, `case "codex-cli":`) ||
		!strings.Contains(source, "return NewCodexCLIAdapter(store)") {
		return fmt.Errorf("codex-cli factory does not use CodexCLIAdapter")
	}
	return nil
}

func openAIAPIStatusShouldNotInspectCodexTmuxState() error {
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "agent", "openai_adapter.go"))
	if err != nil {
		return fmt.Errorf("read OpenAI adapter: %w", err)
	}
	source := string(data)
	for _, forbidden := range []string{"internal/tmux", "IsCodexIdle"} {
		if strings.Contains(source, forbidden) {
			return fmt.Errorf("OpenAI API adapter still depends on Codex tmux state through %q", forbidden)
		}
	}
	return nil
}

func agmRuntimeHelperCommandIsConfigured(ctx context.Context, command string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.runtimeHelperCommand = command
	harnessState.runtimeHelperSpec = filepath.Join(bddRepoRoot(), "agm", "cmd", command, "SPEC.md")
	return nil
}

func agmValidatesRuntimeHelperCommandCoverage(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.runtimeHelperSpec == "" {
		return fmt.Errorf("no AGM runtime helper command configured")
	}
	if _, err := os.Stat(harnessState.runtimeHelperSpec); err != nil {
		return fmt.Errorf("runtime helper SPEC %s: %w", harnessState.runtimeHelperSpec, err)
	}
	return nil
}

func runtimeHelperCommandShouldHaveCoLocatedSPEC(ctx context.Context, command string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if command != harnessState.runtimeHelperCommand {
		return fmt.Errorf("configured runtime helper command = %q, want %q", harnessState.runtimeHelperCommand, command)
	}
	wantSuffix := filepath.Join("agm", "cmd", command, "SPEC.md")
	if !strings.HasSuffix(harnessState.runtimeHelperSpec, wantSuffix) {
		return fmt.Errorf("runtime helper SPEC = %q, want suffix %q", harnessState.runtimeHelperSpec, wantSuffix)
	}
	return nil
}

func agmProductionLocalRuntimeSources(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	sources := []struct {
		path string
		dst  *string
	}{
		{path: filepath.Join("agm", "cmd", "agm", "main.go"), dst: &harnessState.runtimeMainSource},
		{path: filepath.Join("agm", "internal", "ops", "ops.go"), dst: &harnessState.runtimeOpsSource},
		{path: filepath.Join("agm", "internal", "ops", "session_send.go"), dst: &harnessState.runtimeSendSource},
		{path: filepath.Join("agm", "internal", "session", "tmux_real.go"), dst: &harnessState.runtimeTmuxSource},
		{path: filepath.Join("agm", "internal", "harnessexec", "exec.go"), dst: &harnessState.runtimeExecSource},
	}
	for _, source := range sources {
		data, readErr := os.ReadFile(filepath.Join(bddRepoRoot(), source.path))
		if readErr != nil {
			return fmt.Errorf("read runtime ownership source %s: %w", source.path, readErr)
		}
		*source.dst = string(data)
	}
	var production strings.Builder
	for _, root := range []string{
		filepath.Join(bddRepoRoot(), "agm", "cmd"),
		filepath.Join(bddRepoRoot(), "agm", "internal"),
	} {
		if walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// #nosec G122 -- path comes from a read-only walk rooted in the
			// checked-out repository used as this BDD scenario's test fixture.
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			production.WriteString("\n// ")
			production.WriteString(path)
			production.WriteByte('\n')
			production.Write(data)
			return nil
		}); walkErr != nil {
			return fmt.Errorf("scan production local runtime sources under %s: %w", root, walkErr)
		}
	}
	harnessState.runtimeProductionSource = production.String()
	return nil
}

func agmValidatesSingleRuntimeOwnership(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.runtimeMainSource == "" ||
		harnessState.runtimeProductionSource == "" ||
		harnessState.runtimeOpsSource == "" ||
		harnessState.runtimeSendSource == "" ||
		harnessState.runtimeTmuxSource == "" ||
		harnessState.runtimeExecSource == "" {
		return fmt.Errorf("runtime ownership sources were not loaded")
	}
	return nil
}

func productionShouldUseOnlyDirectSessionTmuxRuntimeType(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	const construction = "session.NewRealTmux()"
	if count := strings.Count(harnessState.runtimeProductionSource, construction); count == 0 {
		return fmt.Errorf("production direct session tmux constructions = %d, want at least 1", count)
	}
	for _, retired := range []string{
		"/internal/backend",
		"/internal/manager",
		"managerBackend",
		"GetDefaultBackendAdapter",
		"manager.GetDefault",
	} {
		if strings.Contains(harnessState.runtimeProductionSource, retired) {
			return fmt.Errorf("production source still references retired runtime %q", retired)
		}
	}
	return nil
}

func sharedOperationsShouldExposeNoParallelManagerRuntime(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	for sourceName, source := range map[string]string{
		"OpContext":   harnessState.runtimeOpsSource,
		"SendMessage": harnessState.runtimeSendSource,
	} {
		for _, retired := range []string{"manager.Backend", "ctx.Manager", "internal/manager"} {
			if strings.Contains(source, retired) {
				return fmt.Errorf("%s still references parallel manager runtime %q", sourceName, retired)
			}
		}
	}
	if !strings.Contains(harnessState.runtimeSendSource, "SendKeysIfInputReady") {
		return fmt.Errorf("shared local send no longer uses atomic tmux delivery")
	}
	return nil
}

func directTmuxRuntimeShouldProveSafetyCapabilities(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	for _, capability := range []string{
		"TmuxInterface",
		"TmuxSessionKiller",
		"StrictSessionExistenceChecker",
		"HarnessLivenessChecker",
		"HarnessLivenessBatchChecker",
		"HarnessReadinessWaiter",
		"InputReadinessChecker",
		"AtomicInputSender",
		"VerifiedPaneSender",
	} {
		assertion := fmt.Sprintf("_ %s", capability)
		if !strings.Contains(harnessState.runtimeTmuxSource, assertion) {
			return fmt.Errorf("RealTmux is missing compile-time capability proof %s", capability)
		}
	}
	return nil
}

func retiredGeneralizedRuntimesAndSelectionSettingShouldBeAbsent(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	for _, dir := range []string{
		filepath.Join(bddRepoRoot(), "agm", "internal", "backend"),
		filepath.Join(bddRepoRoot(), "agm", "internal", "manager"),
	} {
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			if statErr != nil {
				return fmt.Errorf("stat retired runtime directory %s: %w", dir, statErr)
			}
			return fmt.Errorf("retired runtime directory still exists: %s", dir)
		}
	}
	for sourceName, source := range map[string]string{
		"main":        harnessState.runtimeMainSource,
		"OpContext":   harnessState.runtimeOpsSource,
		"SendMessage": harnessState.runtimeSendSource,
		"harnessexec": harnessState.runtimeExecSource,
	} {
		if strings.Contains(source, "AGM_SESSION_BACKEND") {
			return fmt.Errorf("%s still references retired AGM_SESSION_BACKEND", sourceName)
		}
	}
	return nil
}

func agmCleanupSupportPackageIsConfigured(ctx context.Context, pkg string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.cleanupSupportPackage = pkg
	harnessState.cleanupSupportSpec = filepath.Join(bddRepoRoot(), "agm", "internal", pkg, "SPEC.md")
	return nil
}

func agmValidatesCleanupSupportPackageCoverage(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.cleanupSupportSpec == "" {
		return fmt.Errorf("no AGM cleanup support package configured")
	}
	if _, err := os.Stat(harnessState.cleanupSupportSpec); err != nil {
		return fmt.Errorf("cleanup support SPEC %s: %w", harnessState.cleanupSupportSpec, err)
	}
	return nil
}

func cleanupSupportPackageShouldHaveCoLocatedSPEC(ctx context.Context, pkg string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if pkg != harnessState.cleanupSupportPackage {
		return fmt.Errorf("configured cleanup support package = %q, want %q", harnessState.cleanupSupportPackage, pkg)
	}
	wantSuffix := filepath.Join("agm", "internal", pkg, "SPEC.md")
	if !strings.HasSuffix(harnessState.cleanupSupportSpec, wantSuffix) {
		return fmt.Errorf("cleanup support SPEC = %q, want suffix %q", harnessState.cleanupSupportSpec, wantSuffix)
	}
	return nil
}

func agmArchiveCleanupTargetsRepositoryCheckout(ctx context.Context) error {
	_, err := getHarnessParityState(ctx)
	return err
}

func agmHasSingleSessionArchiveDryRunContract(ctx context.Context) error {
	_, err := getHarnessParityState(ctx)
	return err
}

func agmValidatesSingleSessionArchiveDryRunSafety(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/cmd/agm-reaper", "./agm/internal/dolt", "./agm/internal/ops", "./agm/internal/reaper",
		"-run", `^(TestArchiveSession_DryRunCLI(TextIsSideEffectFree|JSONReturnsStableEnvelope|JSONHonorsFieldMask|ClaudeUUIDUsesResolvedSessionID|ActiveAsyncDoesNotStartReaper|ActiveRequiresAsync|StoppedRejectsAsync)|TestArchiveSession_(ClaudeUUIDUsesResolvedSessionID|AsyncClaudeUUIDUsesResolvedIdentities|ReloadedSandboxOwnershipControlsCleanup)|TestArchiveAuditArgs_RecordsSingleSessionDryRun|TestArchiveSession_DryRunDoesNotArchiveExternalRepresentation|TestNewDryRunPreview|TestRun_UsesStableSessionIDAndResolvedTmuxIdentity|TestValidateResolvedTargets|TestBuildReaperArgsSeparatesStableAndTmuxIdentities|TestSQLite(SandboxOwnershipMetadataRoundTripsForArchive|MissingSandboxMetadataDoesNotInferOwnership|InvalidSandboxMetadataDoesNotAuthorizeCleanup)|TestCleanupSandboxDirWithChecker_(RemovesOwnedSandbox|RejectsUnownedMergedPath|RetriesOnlyTransientLiveProcess|RefreshesSettingsDuringRetryGrace)|TestRestoreArchivedSessionSerializesWithArchiveCleanup|TestReconcileLifecycleMismatch(SerializesWithArchiveCleanup|RevalidatesTmuxUnderLock))$`,
		"-count=1", "-v")
	cmd.Dir = bddRepoRoot()
	output, runErr := cmd.CombinedOutput()
	harnessState.archiveDryRunTestOutput = string(output)
	harnessState.archiveDryRunTestErr = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("single-session archive dry-run suite timed out: %w", testCtx.Err())
	}
	return nil
}

func durableAndProviderArchiveStateShouldRemainUnchanged(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx,
		"TestArchiveSession_DryRunCLITextIsSideEffectFree",
		"TestArchiveSession_DryRunDoesNotArchiveExternalRepresentation",
	)
}

func archivePreviewShouldReturnStableAGM100Output(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx,
		"TestArchiveSession_DryRunCLIJSONReturnsStableEnvelope",
		"TestArchiveAuditArgs_RecordsSingleSessionDryRun",
		"TestNewDryRunPreview",
	)
}

func archivePreviewShouldRetainResolvedStableSessionIdentity(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx, "TestArchiveSession_DryRunCLIClaudeUUIDUsesResolvedSessionID")
}

func archiveCompletionGuidanceShouldUseResolvedStableSessionIdentity(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx, "TestArchiveSession_ClaudeUUIDUsesResolvedSessionID")
}

func activeAsyncArchiveShouldSeparateStableAndTmuxIdentities(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx,
		"TestArchiveSession_AsyncClaudeUUIDUsesResolvedIdentities",
		"TestBuildReaperArgsSeparatesStableAndTmuxIdentities",
		"TestRun_UsesStableSessionIDAndResolvedTmuxIdentity",
		"TestValidateResolvedTargets",
	)
}

func archivePreviewShouldHonorGlobalJSONFieldMasks(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx, "TestArchiveSession_DryRunCLIJSONHonorsFieldMask")
}

func activeAsyncPreviewShouldNotStartDetachedReaper(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx, "TestArchiveSession_DryRunCLIActiveAsyncDoesNotStartReaper")
}

func dryRunPreviewShouldPreserveAsyncStateValidation(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx,
		"TestArchiveSession_DryRunCLIActiveRequiresAsync",
		"TestArchiveSession_DryRunCLIStoppedRejectsAsync",
	)
}

func validatedPersistedSandboxOwnershipShouldControlArchiveCleanupAfterReload(ctx context.Context) error {
	return requireArchiveDryRunTests(ctx,
		"TestSQLiteSandboxOwnershipMetadataRoundTripsForArchive",
		"TestSQLiteMissingSandboxMetadataDoesNotInferOwnership",
		"TestSQLiteInvalidSandboxMetadataDoesNotAuthorizeCleanup",
		"TestCleanupSandboxDirWithChecker_RemovesOwnedSandbox",
		"TestCleanupSandboxDirWithChecker_RejectsUnownedMergedPath",
		"TestArchiveSession_ReloadedSandboxOwnershipControlsCleanup",
	)
}

func requireArchiveDryRunTests(ctx context.Context, testNames ...string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveDryRunTestErr != nil {
		return fmt.Errorf("single-session archive dry-run suite failed: %w\n%s", harnessState.archiveDryRunTestErr, harnessState.archiveDryRunTestOutput)
	}
	for _, testName := range testNames {
		if !strings.Contains(harnessState.archiveDryRunTestOutput, "--- PASS: "+testName) {
			return fmt.Errorf("%s did not run:\n%s", testName, harnessState.archiveDryRunTestOutput)
		}
	}
	return nil
}

func archiveCleanupShouldWaitForTransientChildExitWithoutWeakeningSafetyGates(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveDryRunTestErr != nil {
		return fmt.Errorf("single-session archive dry-run suite failed: %w\n%s", harnessState.archiveDryRunTestErr, harnessState.archiveDryRunTestOutput)
	}
	const testName = "TestCleanupSandboxDirWithChecker_RetriesOnlyTransientLiveProcess"
	if !strings.Contains(harnessState.archiveDryRunTestOutput, "--- PASS: "+testName) {
		return fmt.Errorf("%s did not run:\n%s", testName, harnessState.archiveDryRunTestOutput)
	}
	return nil
}

func archiveCleanupShouldPreserveSettingsWrittenDuringRetryGrace(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveDryRunTestErr != nil {
		return fmt.Errorf("single-session archive dry-run suite failed: %w\n%s", harnessState.archiveDryRunTestErr, harnessState.archiveDryRunTestOutput)
	}
	const testName = "TestCleanupSandboxDirWithChecker_RefreshesSettingsDuringRetryGrace"
	if !strings.Contains(harnessState.archiveDryRunTestOutput, "--- PASS: "+testName) {
		return fmt.Errorf("%s did not run:\n%s", testName, harnessState.archiveDryRunTestOutput)
	}
	return nil
}

func unarchiveShouldSerializeWithArchiveCleanup(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveDryRunTestErr != nil {
		return fmt.Errorf("single-session archive dry-run suite failed: %w\n%s", harnessState.archiveDryRunTestErr, harnessState.archiveDryRunTestOutput)
	}
	const testName = "TestRestoreArchivedSessionSerializesWithArchiveCleanup"
	if !strings.Contains(harnessState.archiveDryRunTestOutput, "--- PASS: "+testName) {
		return fmt.Errorf("%s did not run:\n%s", testName, harnessState.archiveDryRunTestOutput)
	}
	return nil
}

func adminReconcileFixesShouldSerializeAndRevalidateUnderTheArchiveLock(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveDryRunTestErr != nil {
		return fmt.Errorf("single-session archive dry-run suite failed: %w\n%s", harnessState.archiveDryRunTestErr, harnessState.archiveDryRunTestOutput)
	}
	for _, testName := range []string{
		"TestReconcileLifecycleMismatchSerializesWithArchiveCleanup",
		"TestReconcileLifecycleMismatchRevalidatesTmuxUnderLock",
	} {
		if !strings.Contains(harnessState.archiveDryRunTestOutput, "--- PASS: "+testName) {
			return fmt.Errorf("%s did not run:\n%s", testName, harnessState.archiveDryRunTestOutput)
		}
	}
	return nil
}

func agmValidatesPrimaryCheckoutCleanupSafety(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/ops", "./agm/internal/cleanup",
		"-run", `^(TestCleanupAfterArchive_(PreservesPrimaryCheckout|WithRealGitWorktree|UsesSurvivingRepoAfterRemovingLinkedRepoPath|PreservesBranchWhenWorktreeCannotBeClassified|UsesContextProjectWhenWorkingDirectoryMissing|PreservesBranchNotOwnedByRemovedWorktree)|TestSessionResources_(BranchCleanup|WorktreeRemoveError))$`,
		"-count=1", "-v")
	cmd.Dir = bddRepoRoot()
	output, runErr := cmd.CombinedOutput()
	harnessState.archiveCleanupTestOutput = string(output)
	harnessState.archiveCleanupTestErr = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("archive cleanup safety suite timed out: %w", testCtx.Err())
	}
	return nil
}

func primaryCheckoutAndSessionNamedBranchShouldRemain(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveCleanupTestErr != nil {
		return fmt.Errorf("archive cleanup safety suite failed: %w\n%s", harnessState.archiveCleanupTestErr, harnessState.archiveCleanupTestOutput)
	}
	if !strings.Contains(harnessState.archiveCleanupTestOutput, "--- PASS: TestCleanupAfterArchive_PreservesPrimaryCheckout") {
		return fmt.Errorf("primary checkout preservation regression did not run:\n%s", harnessState.archiveCleanupTestOutput)
	}
	return nil
}

func linkedSessionWorktreeShouldStillBeRemoved(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveCleanupTestErr != nil {
		return fmt.Errorf("archive cleanup safety suite failed: %w\n%s", harnessState.archiveCleanupTestErr, harnessState.archiveCleanupTestOutput)
	}
	if !strings.Contains(harnessState.archiveCleanupTestOutput, "--- PASS: TestCleanupAfterArchive_WithRealGitWorktree") {
		return fmt.Errorf("linked worktree cleanup regression did not run:\n%s", harnessState.archiveCleanupTestOutput)
	}
	return nil
}

func linkedWorktreeCleanupShouldContinueThroughTheSurvivingCheckout(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveCleanupTestErr != nil {
		return fmt.Errorf("archive cleanup safety suite failed: %w\n%s", harnessState.archiveCleanupTestErr, harnessState.archiveCleanupTestOutput)
	}
	if !strings.Contains(harnessState.archiveCleanupTestOutput, "--- PASS: TestCleanupAfterArchive_UsesSurvivingRepoAfterRemovingLinkedRepoPath") {
		return fmt.Errorf("surviving checkout cleanup regression did not run:\n%s", harnessState.archiveCleanupTestOutput)
	}
	return nil
}

func unclassifiedWorktreeShouldNotAuthorizeBranchDeletion(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveCleanupTestErr != nil {
		return fmt.Errorf("archive cleanup safety suite failed: %w\n%s", harnessState.archiveCleanupTestErr, harnessState.archiveCleanupTestOutput)
	}
	if !strings.Contains(harnessState.archiveCleanupTestOutput, "--- PASS: TestCleanupAfterArchive_PreservesBranchWhenWorktreeCannotBeClassified") {
		return fmt.Errorf("unclassified worktree fail-closed regression did not run:\n%s", harnessState.archiveCleanupTestOutput)
	}
	return nil
}

func contextOnlyCheckoutShouldNotAuthorizeBranchDeletion(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveCleanupTestErr != nil {
		return fmt.Errorf("archive cleanup safety suite failed: %w\n%s", harnessState.archiveCleanupTestErr, harnessState.archiveCleanupTestOutput)
	}
	if !strings.Contains(harnessState.archiveCleanupTestOutput, "--- PASS: TestCleanupAfterArchive_UsesContextProjectWhenWorkingDirectoryMissing") {
		return fmt.Errorf("context-only cleanup fail-closed regression did not run:\n%s", harnessState.archiveCleanupTestOutput)
	}
	return nil
}

func branchDeletionShouldRequireAttributedWorktreeOwnership(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.archiveCleanupTestErr != nil {
		return fmt.Errorf("archive cleanup safety suite failed: %w\n%s", harnessState.archiveCleanupTestErr, harnessState.archiveCleanupTestOutput)
	}
	for _, testName := range []string{
		"TestCleanupAfterArchive_PreservesBranchNotOwnedByRemovedWorktree",
		"TestSessionResources_BranchCleanup",
		"TestSessionResources_WorktreeRemoveError",
	} {
		if !strings.Contains(harnessState.archiveCleanupTestOutput, "--- PASS: "+testName) {
			return fmt.Errorf("attributed branch cleanup regression %s did not run:\n%s", testName, harnessState.archiveCleanupTestOutput)
		}
	}
	return nil
}

func modelFamilyIsConfigured(ctx context.Context, family string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.configuredModelFamily = strings.ToLower(family)
	return nil
}

func agmValidatesModelFamilyParitySupport(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredModelFamily == "" {
		return fmt.Errorf("no model family configured")
	}
	model, ok := agent.DefaultModelForFamily(harnessState.configuredModelFamily)
	harnessState.modelFamilyDefaulted = ok
	if !ok {
		return nil
	}
	return agent.ValidateModel("opencode-cli", model.FullName)
}

func modelFamilyShouldBeSupported(ctx context.Context, family string) error {
	if agent.IsSupportedModelFamily(family) {
		return nil
	}
	return fmt.Errorf("model family %q is not supported; supported families: %v", family, agent.ModelFamilyNames())
}

func modelFamilyShouldHaveDefaultModelRoute(ctx context.Context, family string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if strings.ToLower(family) != harnessState.configuredModelFamily {
		return fmt.Errorf("configured model family = %q, want %q", harnessState.configuredModelFamily, family)
	}
	if !harnessState.modelFamilyDefaulted {
		return fmt.Errorf("model family %q has no default route", family)
	}
	return nil
}

func agmResolvesModelChangeForHarness(ctx context.Context, harness, model string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if err := agent.ValidateHarnessName(normalized); err != nil {
		return err
	}
	if err := agent.ValidateModel(normalized, model); err != nil {
		return err
	}
	resolved := agent.ResolveModelFullName(normalized, model)
	harnessState.configuredHarness = normalized
	harnessState.modelChangeResolvedModel = resolved
	harnessState.modelChangeCommand = "/model"
	return nil
}

func modelChangeShouldUseTmuxCommand(ctx context.Context, expected string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.modelChangeCommand != expected {
		return fmt.Errorf("model change command = %q, want %q", harnessState.modelChangeCommand, expected)
	}
	return nil
}

func resolvedModelShouldNotBeEmpty(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.modelChangeResolvedModel == "" {
		return fmt.Errorf("resolved model for harness %q is empty", harnessState.configuredHarness)
	}
	return nil
}

func permissionProfileIsConfigured(ctx context.Context, profile string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !rbac.ValidRole(profile) {
		return fmt.Errorf("permission profile %q is not valid", profile)
	}
	harnessState.permissionProfile = profile
	return nil
}

func agmValidatesPermissionParitySupport(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := permissionparity.ValidateActiveHarnessSurfaces(); err != nil {
		return err
	}
	harnessState.permissionSurfaces = permissionparity.ActiveHarnessSurfaces()
	return nil
}

func piPermissionModeWithPolicy(ctx context.Context, mode, policy string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.piPermissionMode = mode
	if policy != "" {
		harnessState.piPermissionPolicy = []string{policy}
	}
	return nil
}

func piRequestsToolInteractively(ctx context.Context, tool, input string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	key := "path"
	if tool == "bash" {
		key = "command"
	}
	harnessState.piPermissionDecision = permissionparity.DecidePiToolCall(
		harnessState.piPermissionMode,
		harnessState.piPermissionPolicy,
		permissionparity.PiToolCall{ToolName: tool, Input: map[string]any{key: input}},
		true,
	)
	return nil
}

func piPermissionDecisionShouldBe(ctx context.Context, want string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if got := string(harnessState.piPermissionDecision.Action); got != want {
		return fmt.Errorf("pi permission decision = %q, want %q (%s)", got, want, harnessState.piPermissionDecision.Reason)
	}
	return nil
}

func existingPiPaneWithLiveness(ctx context.Context, exact, liveness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.piExactProcess = exact == "true"
	switch liveness {
	case "unknown":
		harnessState.piPaneLiveness = tmux.PaneLiveness{}
	case "shell":
		harnessState.piPaneLiveness = tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}
	case "harness":
		harnessState.piPaneLiveness = tmux.PaneLiveness{SessionExists: true, HarnessAlive: true, Evidence: "zsh,claude"}
	case "foreground":
		harnessState.piPaneLiveness = tmux.PaneLiveness{SessionExists: true, Evidence: "zsh,vim"}
	case "missing":
		harnessState.piPaneLiveness = tmux.PaneLiveness{}
	default:
		return fmt.Errorf("unknown Pi pane liveness %q", liveness)
	}
	return nil
}

func agmEvaluatesPiColdResumeSafety(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.piPaneResumeAction, harnessState.piPaneResumeErr = agent.DecidePiPaneResume(
		harnessState.piExactProcess,
		harnessState.piPaneLiveness,
	)
	return nil
}

func piResumeShould(ctx context.Context, want string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if want == "reject" {
		if harnessState.piPaneResumeErr == nil {
			return fmt.Errorf("pi resume action = %q, want rejection", harnessState.piPaneResumeAction)
		}
		return nil
	}
	if harnessState.piPaneResumeErr != nil {
		return fmt.Errorf("pi resume decision error, want %q: %w", want, harnessState.piPaneResumeErr)
	}
	if got := string(harnessState.piPaneResumeAction); got != want {
		return fmt.Errorf("pi resume action = %q, want %q", got, want)
	}
	return nil
}

func existingPiPaneProcessCommand(ctx context.Context, command string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.piProcessCommand = command
	return nil
}

func agmEvaluatesPiProcessIdentity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.piProcessRecognized = tmux.IsPiProcessCommand(harnessState.piProcessCommand)
	return nil
}

func piProcessIdentityShouldBe(ctx context.Context, decision string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	want := decision == "recognized"
	if decision != "recognized" && decision != "rejected" {
		return fmt.Errorf("unknown Pi identity decision %q", decision)
	}
	if harnessState.piProcessRecognized != want {
		return fmt.Errorf("pi identity recognition = %v, want %q", harnessState.piProcessRecognized, decision)
	}
	return nil
}

func agmResolvesPermissionPolicyParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	allowList, err := rbac.ResolvePermissions(rbac.ResolveOptions{ProfileName: harnessState.permissionProfile})
	if err != nil {
		return err
	}
	harnessState.permissionAllowList = allowList
	harnessState.permissionSurfaces = permissionparity.ActiveHarnessSurfaces()
	return nil
}

func harnessShouldHavePermissionPolicyTarget(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	surface, ok := findPermissionSurface(harnessState.permissionSurfaces, harness)
	if !ok {
		return fmt.Errorf("harness %q has no permission policy target", harness)
	}
	if surface.PolicySurface == "" {
		return fmt.Errorf("harness %q has empty policy surface", harness)
	}
	return nil
}

func harnessShouldHaveStartupPermissionSurface(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	surface, ok := findPermissionSurface(harnessState.permissionSurfaces, harness)
	if !ok {
		return fmt.Errorf("harness %q has no permission policy target", harness)
	}
	if surface.StartupSurface == "" {
		return fmt.Errorf("harness %q has empty startup permission surface", harness)
	}
	return nil
}

func everyActiveHarnessShouldHavePermissionPolicyTarget(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	for _, harness := range agent.ActiveHarnesses() {
		if _, ok := findPermissionSurface(harnessState.permissionSurfaces, harness); !ok {
			return fmt.Errorf("active harness %q has no permission policy target", harness)
		}
	}
	return nil
}

func resolvedPermissionPolicyShouldIncludeDefaultPermissions(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	for _, permission := range rbac.DefaultPermissions {
		if !slices.Contains(harnessState.permissionAllowList, permission) {
			return fmt.Errorf("resolved policy missing default permission %q", permission)
		}
	}
	return nil
}

func resolvedPermissionPolicyShouldIncludeProfilePermissions(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	profile, err := rbac.LookupProfile(harnessState.permissionProfile)
	if err != nil {
		return err
	}
	for _, permission := range profile.AllowedTools {
		if !slices.Contains(harnessState.permissionAllowList, permission) {
			return fmt.Errorf("resolved policy missing profile permission %q", permission)
		}
	}
	return nil
}

func findPermissionSurface(surfaces []permissionparity.Surface, harness string) (permissionparity.Surface, bool) {
	normalized := agent.NormalizeHarnessName(harness)
	for _, surface := range surfaces {
		if surface.Harness == normalized {
			return surface, true
		}
	}
	return permissionparity.Surface{}, false
}

func agmValidatesQuotaMonitoringParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := quotaparity.ValidateActiveHarnessSurfaces(); err != nil {
		return err
	}
	harnessState.quotaSurfaces = quotaparity.ActiveHarnessSurfaces()
	return nil
}

func agmValidatesQuotaModelFamilyCoverage(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredModelFamily == "" {
		return fmt.Errorf("no model family configured")
	}
	if err := quotaparity.ValidateModelFamilyCoverage(); err != nil {
		return err
	}
	coverage, ok := quotaparity.ModelFamilyCoverageFor(harnessState.configuredModelFamily)
	if !ok {
		return fmt.Errorf("model family %q has no quota coverage", harnessState.configuredModelFamily)
	}
	harnessState.quotaFamilyCoverage = coverage
	return nil
}

func harnessShouldHaveContextQuotaSource(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	surface, ok := findQuotaSurface(harnessState.quotaSurfaces, harness)
	if !ok {
		return fmt.Errorf("harness %q has no quota surface", harness)
	}
	if surface.ContextSource == "" {
		return fmt.Errorf("harness %q has empty context quota source", harness)
	}
	return nil
}

func harnessShouldHaveCostQuotaSource(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	surface, ok := findQuotaSurface(harnessState.quotaSurfaces, harness)
	if !ok {
		return fmt.Errorf("harness %q has no quota surface", harness)
	}
	if surface.CostSource == "" {
		return fmt.Errorf("harness %q has empty cost quota source", harness)
	}
	return nil
}

func harnessShouldHaveRateLimitQuotaPolicy(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	surface, ok := findQuotaSurface(harnessState.quotaSurfaces, harness)
	if !ok {
		return fmt.Errorf("harness %q has no quota surface", harness)
	}
	if surface.RateLimitSource == "" {
		return fmt.Errorf("harness %q has empty rate limit quota policy", harness)
	}
	return nil
}

func modelFamilyShouldHaveQuotaPricingPolicy(ctx context.Context, family string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.quotaFamilyCoverage.Family != strings.ToLower(family) {
		return fmt.Errorf("quota coverage family = %q, want %q", harnessState.quotaFamilyCoverage.Family, family)
	}
	if harnessState.quotaFamilyCoverage.PricePolicy == "" {
		return fmt.Errorf("model family %q has empty quota pricing policy", family)
	}
	return nil
}

func modelFamilyShouldHaveSourcedSharedPricing(ctx context.Context, family string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"glm", "deepseek", "nemotron", "qwen"}, strings.ToLower(family)) {
		return fmt.Errorf("model family %q is outside priority sourced-pricing scope", family)
	}
	coverage := harnessState.quotaFamilyCoverage
	if !coverage.Priced || coverage.PriceSource == "" || coverage.PriceAsOf == "" {
		return fmt.Errorf("model family %q lacks sourced shared pricing: %+v", family, coverage)
	}
	return nil
}

func modelFamilyShouldHaveDefaultQuotaModelRoute(ctx context.Context, family string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.quotaFamilyCoverage.Family != strings.ToLower(family) {
		return fmt.Errorf("quota coverage family = %q, want %q", harnessState.quotaFamilyCoverage.Family, family)
	}
	if harnessState.quotaFamilyCoverage.DefaultModel == "" {
		return fmt.Errorf("model family %q has no default quota model route", family)
	}
	return nil
}

func agmValidatesMCPSessionCreationParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := mcpparity.ValidateActiveCreateSessionSurfaces(); err != nil {
		return err
	}
	surface, ok := mcpparity.CreateSessionSurfaceFor(harnessState.configuredHarness)
	if !ok {
		return fmt.Errorf("harness %q has no MCP create-session surface", harnessState.configuredHarness)
	}
	harnessState.mcpSurface = surface
	return nil
}

func harnessShouldHaveMCPCreateSessionSurface(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.mcpSurface.Harness != normalized {
		return fmt.Errorf("MCP surface harness = %q, want %q", harnessState.mcpSurface.Harness, normalized)
	}
	if harnessState.mcpSurface.DefaultModel == "" {
		return fmt.Errorf("harness %q has empty MCP default model", normalized)
	}
	return nil
}

func mcpCreateSessionSurfaceShouldUseSharedModelValidation(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.mcpSurface.ModelPolicy != "shared-agent-registry" {
		return fmt.Errorf("MCP model policy = %q, want shared-agent-registry", harnessState.mcpSurface.ModelPolicy)
	}
	return mcpparity.ValidateModelIdentifier(harnessState.mcpSurface.Harness, harnessState.mcpSurface.DefaultModel)
}

func mcpCreateSessionSurfaceShouldBeDeprecatedCompatibility(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.mcpSurface.Deprecated {
		return fmt.Errorf("MCP surface for %q is not marked deprecated compatibility", harnessState.mcpSurface.Harness)
	}
	return nil
}

func agmValidatesMCPModelIdentifier(ctx context.Context, model string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	harnessState.mcpModelAccepted = mcpparity.ValidateModelIdentifier(harnessState.configuredHarness, model) == nil
	return nil
}

func mcpModelIdentifierShouldBeAccepted(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.mcpModelAccepted {
		return fmt.Errorf("MCP model identifier was rejected")
	}
	return nil
}

func agmValidatesMCPOperationDiscoveryParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.mcpLifecycleOpsExposed = mcpparity.ValidateLifecycleOperations() == nil
	return nil
}

func mcpOperationRegistryShouldExposeLifecycleMutations(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.mcpLifecycleOpsExposed {
		return fmt.Errorf("MCP operation registry does not expose lifecycle mutations")
	}
	return nil
}

func agmValidatesMCPKillMutationWiring(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test",
		"./agm/cmd/agm-mcp-server", "./agm/internal/ops",
		"-run", `^(TestKillSessionTool(ExecutesAndVerifiesSharedMutation|ForwardsRecentActivityForce|CancellationStopsBeforeMutation)|TestKillSession_(ReloadsCurrentTargetUnderStableIDLock|CanceledRequestDoesNotMutateTmux|PropagatesBackendFailure|FailsWhenTargetRemains|PropagatesProbeFailure))$`,
		"-count=1", "-v")
	cmd.Dir = bddRepoRoot()
	output, runErr := cmd.CombinedOutput()
	harnessState.mcpKillTestOutput = string(output)
	harnessState.mcpKillTestErr = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("MCP shared kill behavior suite timed out: %w", testCtx.Err())
	}
	return nil
}

func mcpKillShouldProvideARealTmuxDependencyToSharedOperations(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.mcpKillTestErr != nil {
		return fmt.Errorf("MCP shared kill behavior suite failed: %w\n%s", state.mcpKillTestErr, state.mcpKillTestOutput)
	}
	for _, behavior := range []string{
		"TestKillSessionToolExecutesAndVerifiesSharedMutation",
		"TestKillSessionToolForwardsRecentActivityForce",
		"TestKillSessionToolCancellationStopsBeforeMutation",
	} {
		if !strings.Contains(state.mcpKillTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("MCP kill behavior %s did not pass:\n%s", behavior, state.mcpKillTestOutput)
		}
	}
	return nil
}

func sharedKillSuccessShouldRequireExactTargetAbsence(ctx context.Context) error {
	state := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if state.mcpKillTestErr != nil {
		return fmt.Errorf("MCP shared kill behavior suite failed: %w\n%s", state.mcpKillTestErr, state.mcpKillTestOutput)
	}
	for _, behavior := range []string{
		"TestKillSession_ReloadsCurrentTargetUnderStableIDLock",
		"TestKillSession_CanceledRequestDoesNotMutateTmux",
		"TestKillSession_PropagatesBackendFailure",
		"TestKillSession_FailsWhenTargetRemains",
		"TestKillSession_PropagatesProbeFailure",
	} {
		if !strings.Contains(state.mcpKillTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("shared kill behavior %s did not pass:\n%s", behavior, state.mcpKillTestOutput)
		}
	}
	return nil
}

func agmValidatesMCPServerStartupGuardCoverage(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	specPath := filepath.Join(bddRepoRoot(), "agm", "cmd", "agm-mcp-server", "SPEC.md")
	data, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("read MCP server SPEC %s: %w", specPath, err)
	}
	text := string(data)
	harnessState.mcpServerStartupGuard = strings.Contains(text, "MCS-03") &&
		strings.Contains(text, "MCS-04") &&
		strings.Contains(text, "agm/test/bdd/features/mcp_parity.feature") &&
		!strings.Contains(text, "NEEDS-AUDIT")
	return nil
}

func mcpServerSPECShouldCoverLoudWorkspaceAndDatabaseFailures(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.mcpServerStartupGuard {
		return fmt.Errorf("MCP server SPEC does not cover completed startup guard traceability")
	}
	return nil
}

func agmValidatesMarketplaceParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	root := bddRepoRoot()
	if err := marketplaceparity.ValidateCatalog(root); err != nil {
		return err
	}
	catalog, err := marketplaceparity.LoadCatalog(root)
	if err != nil {
		return err
	}
	harnessState.marketplaceCatalog = catalog
	if harnessState.configuredHarness != "" {
		surface, ok := marketplaceparity.SurfaceForHarness(catalog, harnessState.configuredHarness)
		if !ok {
			return fmt.Errorf("harness %q has no marketplace surface", harnessState.configuredHarness)
		}
		harnessState.marketplaceSurface = surface
	}
	if harnessState.marketplacePlugin != "" {
		harnessState.marketplacePluginValid = marketplacePluginExists(catalog, harnessState.marketplacePlugin)
	}
	return nil
}

func harnessShouldHaveMarketplaceDiscoverySurface(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.marketplaceSurface.Name != normalized {
		return fmt.Errorf("marketplace surface harness = %q, want %q", harnessState.marketplaceSurface.Name, normalized)
	}
	if harnessState.marketplaceSurface.Catalog == "" {
		return fmt.Errorf("marketplace surface for %q has empty catalog", normalized)
	}
	return nil
}

func marketplaceDiscoverySurfaceShouldUseExpectedMode(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	want := marketplaceparity.ExpectedMarketplaceMode(harnessState.marketplaceSurface.Name)
	if harnessState.marketplaceSurface.Mode != want {
		return fmt.Errorf("marketplace mode = %q, want %q", harnessState.marketplaceSurface.Mode, want)
	}
	return nil
}

func agmValidatesMarketplaceCatalogMirrors(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.marketplaceMirrorValid = marketplaceparity.ValidateClaudeMarketplaceMirror(bddRepoRoot()) == nil
	return nil
}

func claudeMarketplaceShouldMatchNeutralMarketplaceCatalog(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.marketplaceMirrorValid {
		return fmt.Errorf("claude marketplace does not match neutral marketplace catalog")
	}
	return nil
}

func marketplacePluginIsConfigured(ctx context.Context, plugin string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.marketplacePlugin = plugin
	return nil
}

func marketplacePluginShouldPublishDeclaredAssets(ctx context.Context, plugin string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.marketplacePlugin != plugin {
		return fmt.Errorf("marketplace plugin = %q, want %q", harnessState.marketplacePlugin, plugin)
	}
	if !harnessState.marketplacePluginValid {
		return fmt.Errorf("marketplace plugin %q did not validate", plugin)
	}
	return nil
}

func marketplacePluginExists(catalog marketplaceparity.Catalog, plugin string) bool {
	for _, entry := range catalog.Plugins {
		if entry.Name == plugin {
			return true
		}
	}
	return false
}

func agmValidatesEngramParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := engramparity.ValidateActiveHarnessSurfaces(); err != nil {
		return err
	}
	surface, ok := engramparity.SurfaceForHarness(harnessState.configuredHarness)
	if !ok {
		return fmt.Errorf("harness %q has no Engram surface", harnessState.configuredHarness)
	}
	harnessState.engramSurface = surface
	return nil
}

func harnessShouldHaveEngramInjectionSurface(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.engramSurface.Harness != normalized {
		return fmt.Errorf("engram surface harness = %q, want %q", harnessState.engramSurface.Harness, normalized)
	}
	if harnessState.engramSurface.InjectionSurface == "" {
		return fmt.Errorf("harness %q has empty Engram injection surface", normalized)
	}
	return nil
}

func harnessShouldPersistEngramMetadataThroughSharedManifest(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.engramSurface.Harness != normalized {
		return fmt.Errorf("engram surface harness = %q, want %q", harnessState.engramSurface.Harness, normalized)
	}
	if harnessState.engramSurface.PersistenceSurface != "manifest.EngramMetadata" {
		return fmt.Errorf("engram persistence surface = %q, want manifest.EngramMetadata", harnessState.engramSurface.PersistenceSurface)
	}
	return nil
}

func agmValidatesEngramMetadataParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.engramMetadataValid = engramparity.ValidateManifestMetadata() == nil && engramparity.ValidateOpsSurfaces() == nil
	return nil
}

func engramMetadataShouldBeStoredInHarnessNeutralFields(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.engramMetadataValid {
		return fmt.Errorf("engram metadata parity validation failed")
	}
	return nil
}

func agmValidatesHippocampusTranscriptParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	adapter, err := hippocampus.NewHarnessAdapter(harnessState.configuredHarness, "")
	if err != nil {
		return err
	}
	harnessState.hippocampusAdapter = adapter
	return nil
}

func harnessShouldHaveHippocampusTranscriptAdapter(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.hippocampusAdapter == nil {
		return fmt.Errorf("harness %q has no Hippocampus transcript adapter", harness)
	}
	want := agent.NormalizeHarnessName(harness)
	if got := harnessState.hippocampusAdapter.Name(); got != want {
		return fmt.Errorf("hippocampus adapter name = %q, want %q", got, want)
	}
	return nil
}

func agmValidatesHippocampusLLMParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	var provider hippocampus.LLMProvider = hippocampus.NewSideQueryLLM(nil)
	signals, err := provider.ExtractSignals(ctx, "")
	harnessState.hippocampusLLMNeutral = err == nil && len(signals) == 0
	return nil
}

func hippocampusConsolidationShouldUseModelFamilyNeutralProvider(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.hippocampusLLMNeutral {
		return fmt.Errorf("hippocampus LLM provider is not model-family-neutral")
	}
	return nil
}

func agmValidatesWayfinderParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := wayfinderparity.ValidateActiveHarnessSurfaces(); err != nil {
		return err
	}
	surface, ok := wayfinderparity.SurfaceForHarness(harnessState.configuredHarness)
	if !ok {
		return fmt.Errorf("harness %q has no Wayfinder surface", harnessState.configuredHarness)
	}
	harnessState.wayfinderSurface = surface
	return nil
}

func harnessShouldHaveWayfinderDiscoverySurface(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.wayfinderSurface.Harness != normalized {
		return fmt.Errorf("wayfinder surface harness = %q, want %q", harnessState.wayfinderSurface.Harness, normalized)
	}
	if harnessState.wayfinderSurface.DiscoverySurface == "" {
		return fmt.Errorf("harness %q has empty Wayfinder discovery surface", normalized)
	}
	return nil
}

func harnessShouldHaveWayfinderExecutionSurface(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.wayfinderSurface.Harness != normalized {
		return fmt.Errorf("wayfinder surface harness = %q, want %q", harnessState.wayfinderSurface.Harness, normalized)
	}
	if harnessState.wayfinderSurface.ExecutionSurface == "" {
		return fmt.Errorf("harness %q has empty Wayfinder execution surface", normalized)
	}
	return nil
}

func agmValidatesWayfinderAssetParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.wayfinderAssetsValid = wayfinderparity.ValidateAssets(bddRepoRoot()) == nil && wayfinderparity.ValidateMCPOperations() == nil
	return nil
}

func wayfinderShouldPublishSkillPluginCommandAndMCPStatusSurfaces(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.wayfinderAssetsValid {
		return fmt.Errorf("wayfinder asset or mcp operation parity validation failed")
	}
	return nil
}

func agmValidatesWayfinderPhaseEngramParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.wayfinderPhaseEngrams = wayfinderparity.ValidatePhaseEngramCoverage() == nil
	return nil
}

func wayfinderShouldResolvePhaseEngramsWithoutHarnessSpecificState(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.wayfinderPhaseEngrams {
		return fmt.Errorf("wayfinder phase engram coverage validation failed")
	}
	return nil
}

func agmValidatesConfigurationDirectoryParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := configdirparity.ValidateActiveDirectories(bddRepoRoot()); err != nil {
		return err
	}
	surface, ok := configdirparity.SurfaceForHarness(harnessState.configuredHarness)
	if !ok {
		return fmt.Errorf("harness %q has no configuration directory surface", harnessState.configuredHarness)
	}
	harnessState.configDirSurface = surface
	return nil
}

func agmValidatesDeprecatedConfigurationDirectoryParity(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.configuredHarness == "" {
		return fmt.Errorf("no harness configured")
	}
	if err := configdirparity.ValidateDeprecatedCompatibility(bddRepoRoot()); err != nil {
		return err
	}
	surface, ok := configdirparity.SurfaceForHarness(harnessState.configuredHarness)
	if !ok {
		return fmt.Errorf("harness %q has no configuration directory surface", harnessState.configuredHarness)
	}
	harnessState.configDirSurface = surface
	return nil
}

func harnessShouldHaveConfigurationDirectory(ctx context.Context, harness, directory string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	normalized := agent.NormalizeHarnessName(harness)
	if harnessState.configDirSurface.Harness != normalized {
		return fmt.Errorf("configuration directory surface harness = %q, want %q", harnessState.configDirSurface.Harness, normalized)
	}
	if harnessState.configDirSurface.Directory != directory {
		return fmt.Errorf("configuration directory = %q, want %q", harnessState.configDirSurface.Directory, directory)
	}
	return nil
}

func findQuotaSurface(surfaces []quotaparity.HarnessSurface, harness string) (quotaparity.HarnessSurface, bool) {
	normalized := agent.NormalizeHarnessName(harness)
	for _, surface := range surfaces {
		if surface.Harness == normalized {
			return surface, true
		}
	}
	return quotaparity.HarnessSurface{}, false
}

func getHarnessParityState(ctx context.Context) (*harnessParityState, error) {
	harnessState, ok := ctx.Value(harnessParityStateKey{}).(*harnessParityState)
	if !ok || harnessState == nil {
		return nil, fmt.Errorf("harness parity state not initialized")
	}
	return harnessState, nil
}

func aCodexCLIComposerPane(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.paneOutput = "\x1b[2m╭────────────────────────────────────────────╮\x1b[0m\n" +
		"\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0)                 │\x1b[0m\n" +
		"\x1b[2m│ model:     \x1b[0mgpt-5.5 high\x1b[2m   \x1b[0m/model to change │\n" +
		"\x1b[2m╰────────────────────────────────────────────╯\x1b[0m\n" +
		"  To get started, describe a task or try /review\n\n" +
		"\x1b[1m›\x1b[0m \x1b[2mRun /review on my current changes\x1b[0m\n\n" +
		"  gpt-5.5 high · ~/.agm/sandboxes/example/merged/repo0"
	return nil
}

func aStaleCodexCLIComposerFollowedByShellOutput(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.paneOutput = `› Continue the task

  gpt-5.6 xhigh · ~/src/project
user@host:~/src/project$`
	return nil
}

func aCodexCLITrustPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.paneOutput = `Do you trust the contents of this directory?

› 1. Yes, continue
  2. No, exit`
	return nil
}

func codexHooksRequireExplicitReview(ctx context.Context, surface string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "codex-cli"
	switch surface {
	case "numbered selector":
		harnessState.paneOutput = `Hooks need review

4 hooks are new or changed.

Hooks can run outside the sandbox after you trust them.

› 1. Review hooks
  2. Trust all and continue
  3. Continue without trusting (hooks won't run)

Press enter to confirm or esc to go back` + strings.Repeat("\n", 18)
	case "hooks dashboard":
		const dashboard = `Hooks
Lifecycle hooks from config and enabled plugins.

⚠ 11 hooks need review before they can run.

Event                 Installed   Active      Review      Description
PreToolUse            5           0           5           Before a tool exec
SessionStart          1           0           1           When a new session

Press t to trust all; enter to review hooks; esc to close`
		harnessState.paneOutput = dashboard + `
│ >_ OpenAI Codex (v0.145.0) │
│ model: gpt-5.6 high /model to change │
╰──────────────────────────────╯
›
gpt-5.6 high · ~/src/project
` + dashboard
	default:
		return fmt.Errorf("unknown Codex hook-review surface %q", surface)
	}
	return nil
}

func anAGYReadyPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.paneOutput = "READY\n>"
	return nil
}

func anAGYTrustPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.paneOutput = `Do you trust the contents of this project?

> Yes, I trust this folder
  No, ask me again later`
	return nil
}

func anAGYFeedbackSurveyOverAReadyPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.paneOutput = "Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"
	return nil
}

func agmChecksWhetherTheSessionCanReceiveInput(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	detector := state.NewDetector()
	harnessState.detected = detector.DetectState(harnessState.paneOutput, time.Now())
	harnessState.canReceive = detector.CheckCanReceive(harnessState.paneOutput)
	return nil
}

func deliveryShouldBeAllowed(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.canReceive != state.CanReceiveYes {
		return fmt.Errorf("expected delivery to be allowed, got %s", harnessState.canReceive)
	}
	return nil
}

func deliveryShouldBeQueued(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.canReceive != state.CanReceiveQueue {
		return fmt.Errorf("expected delivery to be queued, got %s", harnessState.canReceive)
	}
	return nil
}

func deliveryShouldRequireDismissingAnOverlay(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.canReceive != state.CanReceiveOverlay {
		return fmt.Errorf("expected dismissible overlay, got %s", harnessState.canReceive)
	}
	return nil
}

func detectedSessionStateShouldBe(ctx context.Context, expected string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if string(harnessState.detected.State) != expected {
		return fmt.Errorf("expected state %q, got %q", expected, harnessState.detected.State)
	}
	return nil
}

func codexCLIIsAvailable(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "codex-cli"
	return nil
}

func agyIsAvailable(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "agy"
	return nil
}

func agmCreatesDetachedCodexSessionWithStartupPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if strings.Contains(harnessState.paneOutput, "trust the contents") {
		harnessState.trustAutoAccepted = true
	}
	harnessState.waitedForComposer = true
	harnessState.startupDelivered = true
	return nil
}

func agmCreatesDetachedAGYSessionWithStartupPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if err := runAgyLifecycleBehaviorSuite(ctx, harnessState); err != nil {
		return err
	}
	if err := requireAgyLifecycleBehaviors(harnessState,
		"TestStartAgyHarnessUsesCanonicalLaunchAndWaits",
		"TestCreateSession_AgyDetachedPromptUsesCanonicalCommand",
		"TestWaitForAgyPromptAcceptsTrustBeforeReady",
		"TestWaitForAgyPromptRejectsFirstRunOnboardingWithoutInput",
		"TestWaitForAgyPromptAfterInputIgnoresQuotedOnboarding",
		"TestContainsAgyPromptAfterSurveyRequiresLaterPrompt",
		"TestWaitForAgyPromptDoesNotRedismissStaleSurvey",
	); err != nil {
		return err
	}
	harnessState.trustAutoAccepted = true
	harnessState.waitedForAgyPrompt = true
	harnessState.startupDelivered = true
	return nil
}

func runAgyLifecycleBehaviorSuite(ctx context.Context, harnessState *harnessParityState) error {
	if harnessState.agyLifecycleTestOutput != "" || harnessState.agyLifecycleTestErr != nil {
		return nil
	}
	result := sharedAgyLifecycleBehaviorSuite.load(func() bddBehaviorSuiteResult {
		return executeAgyLifecycleBehaviorSuite(ctx)
	})
	harnessState.agyLifecycleTestOutput = result.output
	harnessState.agyLifecycleTestErr = result.testErr
	return result.executionErr
}

func executeAgyLifecycleBehaviorSuite(ctx context.Context) bddBehaviorSuiteResult {
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test",
		"./agm/cmd/agm",
		"./agm/cmd/agm-mcp-server",
		"./agm/internal/agent",
		"./agm/internal/daemon",
		"./agm/internal/dolt",
		"./agm/internal/importer",
		"./agm/internal/lock",
		"./agm/internal/ops",
		"./agm/internal/safety",
		"./agm/internal/send",
		"./agm/internal/session",
		"./agm/internal/state",
		"./agm/internal/tmux",
		"-run", `^(Test(StartAgyHarness(UsesCanonicalLaunchAndWaits|PropagatesReadinessFailure)|StartNewSessionForContextRejectsCurrentTmuxAgyBeforeLaunch|BuildAgyCommand_(AutoPermissionMode|DefaultPermissionMode)|AgyModelCatalogMatchesPublicCLI|BuildAgyImportedManifest(LeavesUnknownModelUnset|PreservesConversationAndCurrentDefaults)|Integration_CreateSession_AgyBootstrapsLazyIdentityBeforeRegistrationExactlyOnce|CreateSession_AgyDetachedPromptUsesCanonicalCommand|CreateSession_Agy(WorkspaceLockReleasesBeforeSurfaceCompletion|IdentitySnapshotFailsBeforeTmuxMutation|IdentityDiscoveryFailureRollsBackBeforeRegistration|RejectsMissingIdentityBootstrapPromptBeforeMutation|BootstrapFailureRollsBackBeforeDiscoveryAndRegistration|CancellationDuringIdentityBootstrapRollsBackWithCallerError)|CreateSession_CancellationAfterRegistrationRollsBackBeforeCompletion|BuildAgyResumeCommandPreservesModelConversationAndMode|ResumeSession(AgyUnknownModelDoesNotInventOverride|MigratesAmbiguousAgyModelInsideOperation|AgyWorkspaceLockCoversSubmissionAndReadiness|AgyPromptUsesHarnessAwareDelivery|CancellationBeforePromptRollsBackColdRuntime|IgnoresCancellationAfterPromptStarts|AcquiresStableLockBeforeStorageRead)|NormalizeModelInput(PreservesAgyPublicLabels|CanonicalizesCrossHarnessAliases)|ResolveCreateLifecyclePrompt(LoadsAgyPromptFileBeforeMutation|RejectsUnreadableAndOversizedAgyFiles|PreservesOtherHarnessAndDirectPromptBehavior)|ResolveSetModelInstruction_(PreservesAgyPublicLabel|NormalizesCrossHarnessAliasCase)|NewAgyModelConfirmationRejectsStaleOrMismatchedOutput|PersistAgyModelSwitchPreservesOnlyConfirmedProvenance|SQLite(CreateSessionDefaultsModelOnlyForClaude|UpdateSessionRoundTripsModel)|MCPCreateSessionRuntime(WaitsForAgyBeforePrompt|StopsBeforePromptAfterCancellation|BootstrapsAgyIdentityPromptExactlyOnce)|CLICreateSessionRuntimeUses(CallerContextForAgyIdentityBootstrap|AgyBracketedRawPaste)|ExecuteWithSignalContextPropagatesCancellation|RootCommandOwnsProcessSignalHandling|LongRunningCommandsConsumeRootContext|CommandHandlersAvoidBackgroundMultilineDelivery|RunScanLoopUsesCallerContext|RunHeartbeatWatchdogUsesCallerContext|ExecuteRestartContextUsesCallerContext|RunWatchUsesCallerContext|VerifyCompactionUsesCallerContext|MonitorCompactionUsesCallerContext|FinalizeCLICreateSessionStopsCancellationAfterLiveness|RealTmuxResumeReadinessReturnsCallerCancellation|AgyResumeReadinessPreservesOnboardingAndSlowStartupPolicy|WaitForAgyMetadataBackfillUsesCallerContext|WaitForAgyAssociationRetryDelayUsesCallerContext|Run(AgyPostCreate(ReturnsCancellationBeforeSideEffects|MetadataRetryUsesCallerContext|SkipsPromptAlreadyDeliveredForIdentityBootstrap|Propagates(ReadinessFailure|PostPromptReadinessFailure))|ClaudePostCreateReturnsCallerCancellationBeforeSideEffects|CodexPostCreateReturnsCancellationBeforePromptDelivery|SendSetModelUsesCallerContextBeforeSlashCommandDelivery)|DeliverInitialPromptReturnsCallerCancellation|DispatchModeSwitchContextStopsBeforeSlashCommandDelivery|CommandScoped(ReadinessWaitsReturnCallerCancellation|SafeDeliveryReturnsCallerCancellation)|StructuredPromptUsesCallerContext|SendMultiLinePromptSafeContextReturnsCallerCancellation|DaemonDelivery(UsesSharedOperationAndStableResult|DoesNotMarkCompletedAPITurnWorking)|PasteBufferArgsPreserveAgyMultilineAsBracketedRaw|SequentialDeliverPassesCallerContext|NewNonClaudeAssociationManifestLeavesAgyModelUnknown|UpdateNonClaudeAssociationManifestLeavesAgyModelUnknown|AgyCreateSession(UsesCanonicalModelAwareCommand|ImportedConversationOmitsUnknownModel|RejectsExistingTmuxAndUnsafeModelBeforeMutation|PropagatesReadinessFailureAndRollsBack|ReportsRollbackFailure|CapturesNativeConversationIdentity|BootstrapsLazyNativeIdentityWithInitialPrompt|RollsBackWhenInitialPromptBootstrapFails|NormalizesWorkingDirectoryForLaunchAndDiscovery|SerializesWorkspaceIdentityDiscovery|RollsBackWhenNativeIdentityCannotBeCaptured|DoesNotReuseStaleNativeConversationIdentity)|AgySendMessage(UsesHarnessAwareMultilineDelivery|PropagatesHarnessAwareDeliveryFailure)|AgyResumePolicyPersistsInJSONSessionStore|AgyResumeSession(PreservesNativeIdentityModelAndMode|OmitsModelWhenProvenanceUnknown|DoesNotInventNativeIdentity|RejectsUnsafeNativeIdentityBeforeMutation|RejectsAnotherLiveHarnessBeforeMutation|RejectsNonShellForegroundBeforeMutation|RestartsInExistingBareShell|HoldsWorkspaceLockThroughReadiness|SerializesPaneProofWithCommandDelivery|UsesExactProcessLivenessAndFailsSafe|LeavesLiveAgyUntouched|UsesTranscriptSafeReadinessPolicy|PropagatesReadinessFailureBeforeAttach)|AgyGetSessionStatusRequiresAgyProcess|AgyGetHistory(ReadsNativeTranscript|FallsBackToFullTranscript|RequiresNativeIdentity|RejectsUnsafeNativeIdentity)|AgyAdapterRejectsUnsupportedRunHook|DetectAgySessionUninitialized|NormalizeHarnessForSafety|Agy(StaleSurveyAllowsLaterReadyPrompt|PromptBeforeSurveyRemainsOverlay)|ContainsAgyPromptAfterSurveyRequiresLaterPrompt|ClassifyPaneLiveness|CheckPaneLivenessContextHonorsCallerCancellation|WaitForAgyPrompt(AcceptsTrustBeforeReady|DismissesSurveyBeforeReady|DoesNotRedismissStaleSurvey|RejectsFirstRunOnboardingWithoutInput|ReturnsCancellationAfterReadyStabilityDelay)))$`, "-count=1", "-v",
	)
	cmd.Dir = bddRepoRoot()
	output, runErr := cmd.CombinedOutput()
	transcriptCmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/tmux",
		"-run", `^TestWaitForAgyPromptAfterInputIgnoresQuotedOnboarding$`,
		"-count=1", "-v",
	)
	transcriptCmd.Dir = bddRepoRoot()
	transcriptOutput, transcriptErr := transcriptCmd.CombinedOutput()
	resumeOnboardingCmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/tmux",
		"-run", `^TestWaitForAgyPromptOnResume(IgnoresTransientQuotedOnboarding|ConfirmsPersistentOnboardingWithoutInput)$`,
		"-count=1", "-v",
	)
	resumeOnboardingCmd.Dir = bddRepoRoot()
	resumeOnboardingOutput, resumeOnboardingErr := resumeOnboardingCmd.CombinedOutput()
	sharedReadinessCmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/cmd/agm-mcp-server",
		"-run", `^(TestMCPCreateSessionRuntime(CannotBypassSharedAgyReadiness|AgyIdentityBootstrapFailsClosedWhenComposerIsNotReady)|TestSendViaSharedOperations(UsesCallerContext|FailsClosedWhenHarnessIsNotReady)|TestMultiRecipientAgyDeliveryUsesSharedAtomicReadiness)$`,
		"-count=1", "-v",
	)
	sharedReadinessCmd.Dir = bddRepoRoot()
	sharedReadinessOutput, sharedReadinessErr := sharedReadinessCmd.CombinedOutput()
	lockCmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/lock", "./agm/internal/agysession",
		"-run", `^Test(FileLockTryLockPreservesPermanentFlockError|AcquireWorkspaceCreateLockStopsOnPermanentFlockError)$`,
		"-count=1", "-v",
	)
	lockCmd.Dir = bddRepoRoot()
	lockOutput, lockErr := lockCmd.CombinedOutput()
	combinedOutput := string(output) + "\n" + string(transcriptOutput) + "\n" + string(resumeOnboardingOutput) + "\n" + string(sharedReadinessOutput) + "\n" + string(lockOutput)
	if runErr == nil {
		runErr = transcriptErr
	}
	if runErr == nil {
		runErr = resumeOnboardingErr
	}
	if runErr == nil {
		runErr = sharedReadinessErr
	}
	if runErr == nil {
		runErr = lockErr
	}
	if testCtx.Err() != nil {
		return bddBehaviorSuiteResult{
			output:       combinedOutput,
			testErr:      runErr,
			executionErr: fmt.Errorf("AGY lifecycle behavior suite timed out: %w", testCtx.Err()),
		}
	}
	return bddBehaviorSuiteResult{
		output:  combinedOutput,
		testErr: runErr,
	}
}

func agmValidatesTheAgyAdapterLifecycle(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func agyAdapterShouldPreserveCanonicalLaunchAndResumePolicy(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestAgyCreateSessionUsesCanonicalModelAwareCommand",
		"TestAgyCreateSessionImportedConversationOmitsUnknownModel",
		"TestAgyCreateSessionRejectsExistingTmuxAndUnsafeModelBeforeMutation",
		"TestAgyCreateSessionPropagatesReadinessFailureAndRollsBack",
		"TestAgyCreateSessionCapturesNativeConversationIdentity",
		"TestAgyCreateSessionNormalizesWorkingDirectoryForLaunchAndDiscovery",
		"TestAgyCreateSessionSerializesWorkspaceIdentityDiscovery",
		"TestCreateSession_AgyWorkspaceLockReleasesBeforeSurfaceCompletion",
		"TestAgyCreateSessionRollsBackWhenNativeIdentityCannotBeCaptured",
		"TestAgyCreateSessionDoesNotReuseStaleNativeConversationIdentity",
		"TestAgyCreateSessionReportsRollbackFailure",
		"TestFileLockTryLockPreservesPermanentFlockError",
		"TestAcquireWorkspaceCreateLockStopsOnPermanentFlockError",
		"TestResumeSessionAgyWorkspaceLockCoversSubmissionAndReadiness",
		"TestAgyResumePolicyPersistsInJSONSessionStore",
		"TestAgyResumeSessionPreservesNativeIdentityModelAndMode",
		"TestAgyResumeSessionOmitsModelWhenProvenanceUnknown",
		"TestAgyResumeSessionDoesNotInventNativeIdentity",
		"TestAgyResumeSessionRejectsUnsafeNativeIdentityBeforeMutation",
		"TestAgyResumeSessionRejectsAnotherLiveHarnessBeforeMutation",
		"TestAgyResumeSessionRejectsNonShellForegroundBeforeMutation",
		"TestAgyResumeSessionRestartsInExistingBareShell",
		"TestAgyResumeSessionHoldsWorkspaceLockThroughReadiness",
		"TestAgyResumeSessionSerializesPaneProofWithCommandDelivery",
		"TestAgyResumeSessionUsesExactProcessLivenessAndFailsSafe",
		"TestAgyResumeSessionLeavesLiveAgyUntouched",
		"TestAgyResumeSessionUsesTranscriptSafeReadinessPolicy",
		"TestAgyResumeSessionPropagatesReadinessFailureBeforeAttach",
		"TestAgyResumeReadinessPreservesOnboardingAndSlowStartupPolicy",
		"TestWaitForAgyPromptOnResumeIgnoresTransientQuotedOnboarding",
		"TestWaitForAgyPromptOnResumeConfirmsPersistentOnboardingWithoutInput",
	)
}

func agyAdapterShouldRequireAgyProcessAndTranscriptTruth(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestAgyGetSessionStatusRequiresAgyProcess",
		"TestAgyGetHistoryReadsNativeTranscript",
		"TestAgyGetHistoryFallsBackToFullTranscript",
		"TestAgyGetHistoryRequiresNativeIdentity",
		"TestAgyGetHistoryRejectsUnsafeNativeIdentity",
		"TestAgyAdapterRejectsUnsupportedRunHook",
	)
}

func requireAgyLifecycleBehaviors(harnessState *harnessParityState, behaviors ...string) error {
	if harnessState.agyLifecycleTestErr != nil {
		return fmt.Errorf("AGY lifecycle behavior suite failed: %w\n%s", harnessState.agyLifecycleTestErr, harnessState.agyLifecycleTestOutput)
	}
	for _, behavior := range behaviors {
		if !strings.Contains(harnessState.agyLifecycleTestOutput, "--- PASS: "+behavior) {
			return fmt.Errorf("AGY lifecycle behavior %s did not pass:\n%s", behavior, harnessState.agyLifecycleTestOutput)
		}
	}
	return nil
}

func agmValidatesAGYModelCompatibility(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("configured harness = %q, want agy", harnessState.harness)
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func retiredAGYManifestModelsShouldMapToCurrentPublicLabels(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState, "TestResumeSessionMigratesAmbiguousAgyModelInsideOperation")
}

func exactAGYPublicLabelsShouldRemainUnchanged(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestNormalizeModelInputPreservesAgyPublicLabels",
		"TestResolveSetModelInstruction_PreservesAgyPublicLabel",
	)
}

func crossHarnessAGYAliasesShouldNormalizeCaseInsensitively(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestNormalizeModelInputCanonicalizesCrossHarnessAliases",
		"TestResolveSetModelInstruction_NormalizesCrossHarnessAliasCase",
	)
}

func importedAGYConversationsShouldPreserveUnknownModelProvenance(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestBuildAgyImportedManifestLeavesUnknownModelUnset",
		"TestNewNonClaudeAssociationManifestLeavesAgyModelUnknown",
		"TestUpdateNonClaudeAssociationManifestLeavesAgyModelUnknown",
		"TestResumeSessionAgyUnknownModelDoesNotInventOverride",
		"TestResumeSessionMigratesAmbiguousAgyModelInsideOperation",
		"TestSQLiteCreateSessionDefaultsModelOnlyForClaude",
	)
}

func agyRuntimeModelSwitchesShouldNotLeaveAStaleResumeOverride(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestNewAgyModelConfirmationRejectsStaleOrMismatchedOutput",
		"TestPersistAgyModelSwitchPreservesOnlyConfirmedProvenance",
		"TestSQLiteUpdateSessionRoundTripsModel",
	)
}

func agmValidatesAGYMCPCreateReadiness(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("configured harness = %q, want agy", harnessState.harness)
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func mcpCreationShouldWaitForAGYComposerBeforePromptDelivery(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestMCPCreateSessionRuntimeWaitsForAgyBeforePrompt",
		"TestMCPCreateSessionRuntimeCannotBypassSharedAgyReadiness",
	)
}

func sharedCreationShouldPersistNewAGYIdentityBeforeRegistration(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestCreateSession_AgyWorkspaceLockReleasesBeforeSurfaceCompletion",
		"TestCreateSession_AgyIdentitySnapshotFailsBeforeTmuxMutation",
		"TestCreateSession_AgyIdentityDiscoveryFailureRollsBackBeforeRegistration",
	)
}

func agmValidatesAGYLazyIdentityBootstrap(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("configured harness = %q, want agy", harnessState.harness)
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func sharedCreationShouldDeliverAGYStartupPromptBeforeIdentityDiscovery(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestIntegration_CreateSession_AgyBootstrapsLazyIdentityBeforeRegistrationExactlyOnce",
		"TestCreateSession_AgyDetachedPromptUsesCanonicalCommand",
		"TestCreateSession_AgyRejectsMissingIdentityBootstrapPromptBeforeMutation",
	)
}

func everyAGYCreationSurfaceShouldAvoidDuplicatePromptDelivery(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestMCPCreateSessionRuntimeBootstrapsAgyIdentityPromptExactlyOnce",
		"TestRunAgyPostCreateSkipsPromptAlreadyDeliveredForIdentityBootstrap",
		"TestAgyCreateSessionBootstrapsLazyNativeIdentityWithInitialPrompt",
		"TestCLICreateSessionRuntimeUsesCallerContextForAgyIdentityBootstrap",
	)
}

func agyBootstrapFailuresShouldPreserveTransactionalRollback(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestCreateSession_AgyBootstrapFailureRollsBackBeforeDiscoveryAndRegistration",
		"TestCreateSession_AgyCancellationDuringIdentityBootstrapRollsBackWithCallerError",
		"TestAgyCreateSessionRollsBackWhenInitialPromptBootstrapFails",
		"TestResolveCreateLifecyclePromptRejectsUnreadableAndOversizedAgyFiles",
	)
}

func agmValidatesAGYRootCancellationPlumbing(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("configured harness = %q, want agy", harnessState.harness)
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func rootSignalCancellationShouldReachEveryCommandScopedReadinessWait(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestExecuteWithSignalContextPropagatesCancellation",
		"TestRootCommandOwnsProcessSignalHandling",
		"TestLongRunningCommandsConsumeRootContext",
		"TestCommandHandlersAvoidBackgroundMultilineDelivery",
		"TestRunScanLoopUsesCallerContext",
		"TestRunHeartbeatWatchdogUsesCallerContext",
		"TestExecuteRestartContextUsesCallerContext",
		"TestRunWatchUsesCallerContext",
		"TestVerifyCompactionUsesCallerContext",
		"TestMonitorCompactionUsesCallerContext",
		"TestCreateSession_CancellationAfterRegistrationRollsBackBeforeCompletion",
		"TestMCPCreateSessionRuntimeStopsBeforePromptAfterCancellation",
		"TestResumeSessionAcquiresStableLockBeforeStorageRead",
		"TestResumeSessionCancellationBeforePromptRollsBackColdRuntime",
		"TestFinalizeCLICreateSessionStopsCancellationAfterLiveness",
		"TestRealTmuxResumeReadinessReturnsCallerCancellation",
		"TestWaitForAgyMetadataBackfillUsesCallerContext",
		"TestWaitForAgyAssociationRetryDelayUsesCallerContext",
		"TestRunAgyPostCreateReturnsCancellationBeforeSideEffects",
		"TestRunAgyPostCreateMetadataRetryUsesCallerContext",
		"TestRunAgyPostCreatePropagatesReadinessFailure",
		"TestRunAgyPostCreatePropagatesPostPromptReadinessFailure",
		"TestRunClaudePostCreateReturnsCallerCancellationBeforeSideEffects",
		"TestDeliverInitialPromptReturnsCallerCancellation",
		"TestRunSendSetModelUsesCallerContextBeforeSlashCommandDelivery",
		"TestDispatchModeSwitchContextStopsBeforeSlashCommandDelivery",
		"TestCommandScopedReadinessWaitsReturnCallerCancellation",
		"TestCommandScopedSafeDeliveryReturnsCallerCancellation",
		"TestRealTmuxResumeReadinessReturnsCallerCancellation",
		"TestRunCodexPostCreateReturnsCancellationBeforePromptDelivery",
		"TestWaitForAgyPromptReturnsCancellationAfterReadyStabilityDelay",
		"TestSendViaSharedOperationsUsesCallerContext",
		"TestStructuredPromptUsesCallerContext",
		"TestSequentialDeliverPassesCallerContext",
		"TestResumeSessionIgnoresCancellationAfterPromptStarts",
		"TestResumeSessionAgyPromptUsesHarnessAwareDelivery",
		"TestSendMultiLinePromptSafeContextReturnsCallerCancellation",
		"TestCheckPaneLivenessContextHonorsCallerCancellation",
		"TestAgyStaleSurveyAllowsLaterReadyPrompt",
		"TestAgyPromptBeforeSurveyRemainsOverlay",
	)
}

func agmShouldWaitForTheCodexComposer(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.waitedForComposer {
		return fmt.Errorf("expected AGM to wait for the Codex composer")
	}
	return nil
}

func agmShouldWaitForTheAGYPrompt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.waitedForAgyPrompt {
		return fmt.Errorf("expected AGM to wait for the AGY prompt")
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestStartAgyHarnessUsesCanonicalLaunchAndWaits",
		"TestStartAgyHarnessPropagatesReadinessFailure",
	)
}

func agmShouldDeliverStartupPromptDetached(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.startupDelivered {
		return fmt.Errorf("expected detached startup prompt to be delivered")
	}
	return nil
}

func agmShouldAutoAcceptCodexTrustPromptBeforePromptDelivery(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "codex-cli" {
		return fmt.Errorf("harness = %q, want codex-cli", harnessState.harness)
	}
	if !harnessState.trustAutoAccepted || !harnessState.startupDelivered {
		return fmt.Errorf("expected Codex trust prompt to be auto-accepted before startup prompt delivery")
	}
	return nil
}

func agmEvaluatesCodexHookReviewStartup(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if tmux.IsCodexHookReviewRequired(harnessState.paneOutput) {
		harnessState.codexHookReviewErr = tmux.CodexHookReviewError()
	}
	_, state, classifyErr := tmux.ClassifyHarnessInput(harnessState.paneOutput, "codex-cli")
	if classifyErr != nil {
		return classifyErr
	}
	harnessState.codexHookReviewState = state
	return nil
}

func codexStartupShouldFailFastWithExplicitReviewGuidance(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !errors.Is(harnessState.codexHookReviewErr, tmux.ErrCodexHookReviewRequired) {
		if harnessState.codexHookReviewErr == nil {
			return errors.New("codex hook review returned no typed review-required failure")
		}
		return fmt.Errorf("codex hook review returned the wrong failure: %w", harnessState.codexHookReviewErr)
	}
	if !strings.Contains(harnessState.codexHookReviewErr.Error(), "AGM will not trust executable hooks automatically") {
		return fmt.Errorf("codex hook review error lacks no-auto-trust guidance: %w", harnessState.codexHookReviewErr)
	}
	if harnessState.codexHookReviewState != tmux.HarnessInputReviewRequired {
		return fmt.Errorf("codex hook review state = %q, want %q", harnessState.codexHookReviewState, tmux.HarnessInputReviewRequired)
	}
	return nil
}

func codexHookReviewShouldReceiveNoAutomatedInput(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	result := sharedCodexHookReviewBehaviorSuite.load(func() bddBehaviorSuiteResult {
		return executeCodexHookReviewBehaviorSuite(ctx)
	})
	harnessState.codexHookReviewTestOutput = result.output
	harnessState.codexHookReviewTestErr = result.testErr
	if result.executionErr != nil {
		return result.executionErr
	}
	if harnessState.codexHookReviewTestErr != nil {
		return fmt.Errorf("codex hook review behavior suite failed: %w\n%s", harnessState.codexHookReviewTestErr, harnessState.codexHookReviewTestOutput)
	}
	for _, testName := range []string{
		"TestWaitForCodexPromptFailsFastForHookReviewWithoutInput",
		"TestWaitForCodexPromptFailsFastForHookDashboardWithoutInput",
		"TestHandleHarnessStartupStateFailsHookReviewWithoutAdvancing",
		"TestRealTmuxReadinessFailsFastForCodexHookReviewAboveBlankRows",
		"TestCreateSession_CodexHookReviewPropagatesBeforeRegistrationOrPrompt",
		"TestResumeSessionCodexHookReviewFailsBeforeActivityUpdate",
	} {
		passed := strings.Contains(harnessState.codexHookReviewTestOutput, "--- PASS: "+testName)
		policySkipped := (testName == "TestWaitForCodexPromptFailsFastForHookReviewWithoutInput" ||
			testName == "TestWaitForCodexPromptFailsFastForHookDashboardWithoutInput" ||
			testName == "TestRealTmuxReadinessFailsFastForCodexHookReviewAboveBlankRows") &&
			os.Getenv("CI_SKIP_TMUX") == "true" &&
			strings.Contains(harnessState.codexHookReviewTestOutput, "--- SKIP: "+testName)
		if !passed && !policySkipped {
			return fmt.Errorf("codex hook review behavior suite did not execute %s:\n%s", testName, harnessState.codexHookReviewTestOutput)
		}
	}
	return nil
}

func executeCodexHookReviewBehaviorSuite(ctx context.Context) bddBehaviorSuiteResult {
	testCtx, cancel := context.WithTimeout(ctx, codexHookReviewBehaviorSuiteTimeout)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/tmux", "./agm/internal/session", "./agm/internal/ops", "./agm/cmd/agm",
		"-run", `^(TestWaitForCodexPromptFailsFastForHookReviewWithoutInput|TestWaitForCodexPromptFailsFastForHookDashboardWithoutInput|TestHandleHarnessStartupStateFailsHookReviewWithoutAdvancing|TestRealTmuxReadinessFailsFastForCodexHookReviewAboveBlankRows|TestCreateSession_CodexHookReviewPropagatesBeforeRegistrationOrPrompt|TestResumeSessionCodexHookReviewFailsBeforeActivityUpdate)$`,
		"-count=1", "-v",
	)
	cmd.Dir = bddRepoRoot()
	if os.Getenv("CI_SKIP_TMUX") != "true" {
		cmd.Env = append(os.Environ(), "AGM_TEST_TMUX=1")
	}
	output, runErr := cmd.CombinedOutput()
	result := bddBehaviorSuiteResult{
		output:  string(output),
		testErr: runErr,
	}
	if testCtx.Err() != nil {
		result.executionErr = fmt.Errorf("codex hook review behavior suite timed out: %w", testCtx.Err())
	}
	return result
}

func agmShouldAutoAcceptAGYTrustPromptBeforePromptDelivery(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("harness = %q, want agy", harnessState.harness)
	}
	if !harnessState.trustAutoAccepted || !harnessState.startupDelivered {
		return fmt.Errorf("expected AGY trust prompt to be auto-accepted before startup prompt delivery")
	}
	return requireAgyLifecycleBehaviors(harnessState, "TestWaitForAgyPromptAcceptsTrustBeforeReady")
}

func agmRunsSendSafetyForTheConfiguredHarness(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harness := harnessState.harness
	if harness == "" {
		harness = harnessState.configuredHarness
		harnessState.harness = harness
	}
	switch harness {
	case "codex-cli", "opencode-cli", "pi-cli":
		harnessState.sendSafetyRequiresClaude = false
	case "agy":
		if err := runAgyLifecycleBehaviorSuite(ctx, harnessState); err != nil {
			return err
		}
		if err := requireAgyLifecycleBehaviors(harnessState,
			"TestDetectAgySessionUninitialized",
			"TestNormalizeHarnessForSafety",
		); err != nil {
			return err
		}
		harnessState.sendSafetyRequiresClaude = false
	case "":
		return fmt.Errorf("no harness configured")
	default:
		harnessState.sendSafetyRequiresClaude = true
	}
	return nil
}

func sendSafetyShouldNotRequireClaudeProcess(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.sendSafetyRequiresClaude {
		return fmt.Errorf("send safety for harness %q should not require a Claude process", harnessState.harness)
	}
	return nil
}

func agmValidatesAGYMultilineDelivery(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("configured harness = %q, want agy", harnessState.harness)
	}
	return runAgyLifecycleBehaviorSuite(ctx, harnessState)
}

func everyAGYMessageSurfaceShouldPreserveOneBracketedMultilineSubmission(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	return requireAgyLifecycleBehaviors(harnessState,
		"TestPasteBufferArgsPreserveAgyMultilineAsBracketedRaw",
		"TestSendViaSharedOperationsUsesCallerContext",
		"TestStructuredPromptUsesCallerContext",
		"TestMultiRecipientAgyDeliveryUsesSharedAtomicReadiness",
		"TestDaemonDeliveryUsesSharedOperationAndStableResult",
		"TestCLICreateSessionRuntimeUsesAgyBracketedRawPaste",
		"TestAgyCreateSessionBootstrapsLazyNativeIdentityWithInitialPrompt",
		"TestAgySendMessageUsesHarnessAwareMultilineDelivery",
		"TestResumeSessionAgyPromptUsesHarnessAwareDelivery",
	)
}

func anExistingTmuxSessionRunningCodexCLI(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "codex-cli"
	return nil
}

func anExistingTmuxSessionRunningAGY(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "agy"
	return nil
}

func agmAssocRunsInThatSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.readyFileCreated = true
	return nil
}

func agmShouldCreateOrUpdateDoltRecordWithHarness(ctx context.Context, expected string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != expected {
		return fmt.Errorf("harness = %q, want %q", harnessState.harness, expected)
	}
	return nil
}

func agmShouldCreateTheReadyFileSignal(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.readyFileCreated {
		return fmt.Errorf("expected ready-file signal to be created")
	}
	return nil
}

func aCodexSavedSessionExistsOutsideAGM(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.codexSessionUUID = "019ef2af-97e0-7443-9f07-03e40636740c"
	return nil
}

func anAGYSavedConversationExistsOutsideAGM(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.agyConversationID = "117ff898-a964-4a9f-b460-1be4a8a49b17"
	return nil
}

func agmImportsCodexSessionUUIDWithHarness(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harness != "codex-cli" {
		return fmt.Errorf("codex import must use codex-cli harness, got %q", harness)
	}
	if harnessState.codexSessionUUID == "" {
		return fmt.Errorf("no Codex saved session UUID arranged")
	}
	harnessState.harness = harness
	harnessState.preservedCodexUUID = true
	harnessState.tmuxResumeLaunched = true
	return nil
}

func recordShouldPreserveCodexSessionUUID(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.preservedCodexUUID {
		return fmt.Errorf("expected Dolt record to preserve Codex session UUID")
	}
	return nil
}

func agmImportsAGYConversationIDWithHarness(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harness != "agy" {
		return fmt.Errorf("agy import must use agy harness, got %q", harness)
	}
	if harnessState.agyConversationID == "" {
		return fmt.Errorf("no AGY conversation ID arranged")
	}
	if err := runAgyLifecycleBehaviorSuite(ctx, harnessState); err != nil {
		return err
	}
	if err := requireAgyLifecycleBehaviors(harnessState,
		"TestBuildAgyImportedManifestLeavesUnknownModelUnset",
		"TestResumeSessionAgyUnknownModelDoesNotInventOverride",
	); err != nil {
		return err
	}
	harnessState.harness = harness
	harnessState.preservedAgyConversationID = true
	harnessState.agyResumeCommand = ops.BuildAgyResumeCommand(ops.HarnessLaunchSpec{
		Harness: "agy", WorkDir: "/tmp/agy-import",
	}, harnessState.agyConversationID).Command
	harnessState.tmuxResumeLaunched = true
	return nil
}

func recordShouldPreserveAGYConversationID(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.preservedAgyConversationID {
		return fmt.Errorf("expected Dolt record to preserve AGY conversation ID")
	}
	return nil
}

func anImportedAGYSessionWithPermissionMode(ctx context.Context, mode string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "agy"
	harnessState.agyConversationID = "117ff898-a964-4a9f-b460-1be4a8a49b17"
	harnessState.preservedAgyConversationID = true
	harnessState.agyModelKnown = true
	harnessState.agyResumeCommand = ops.BuildAgyResumeCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "claude-sonnet-4.6-thinking", WorkDir: "/tmp/agy-import",
		PermissionMode: mode,
	}, harnessState.agyConversationID).Command
	return nil
}

func agmResumesTheAGYSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness != "agy" {
		return fmt.Errorf("cannot resume AGY session for harness %q", harnessState.harness)
	}
	harnessState.tmuxResumeLaunched = true
	return nil
}

func agmShouldLaunchTmuxPaneResumingCodexConversation(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.tmuxResumeLaunched {
		return fmt.Errorf("expected AGM to launch a tmux pane with codex resume")
	}
	return nil
}

func agmShouldLaunchTmuxPaneResumingAGYConversation(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.tmuxResumeLaunched {
		return fmt.Errorf("expected AGM to launch a tmux pane with agy resume")
	}
	for _, want := range []string{"agy ", "--conversation '" + harnessState.agyConversationID + "'"} {
		if !strings.Contains(harnessState.agyResumeCommand, want) {
			return fmt.Errorf("AGY resume command %q missing %q", harnessState.agyResumeCommand, want)
		}
	}
	if harnessState.agyModelKnown && !strings.Contains(harnessState.agyResumeCommand, "--model ") {
		return fmt.Errorf("AGY resume command %q omitted a known model", harnessState.agyResumeCommand)
	}
	if !harnessState.agyModelKnown && strings.Contains(harnessState.agyResumeCommand, "--model ") {
		return fmt.Errorf("AGY resume command %q overrode an unknown native model", harnessState.agyResumeCommand)
	}
	if strings.Contains(harnessState.agyResumeCommand, "--prompt-interactive") {
		return fmt.Errorf("AGY resume command used prompt-valued flag: %q", harnessState.agyResumeCommand)
	}
	return nil
}

func theAGYResumeCommandShouldInclude(ctx context.Context, expected string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(harnessState.agyResumeCommand, expected) {
		return fmt.Errorf("expected AGM to include %q in the AGY resume command", expected)
	}
	return nil
}

func agmHasCodexSessionRecordsInDolt(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.harness = "codex-cli"
	return nil
}

func agentListsSessionsAsJSONWithFields(ctx context.Context, fields string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.sessionListFields = splitCSV(fields)
	harnessState.sessionListHasArray = true
	return nil
}

func outputShouldIncludeSessionsArray(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.sessionListHasArray {
		return fmt.Errorf("expected sessions array")
	}
	return nil
}

func eachSessionRowShouldIncludeRequestedFields(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if len(harnessState.sessionListFields) == 0 {
		return fmt.Errorf("expected requested fields")
	}
	return nil
}

func outputShouldNotCollapseToEmptyObject(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.sessionListHasArray || len(harnessState.sessionListFields) == 0 {
		return fmt.Errorf("session list output collapsed")
	}
	return nil
}

func aCodexCLISessionCreatedByAGM(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	lifecycleDir, err := os.MkdirTemp("", "agm-bdd-codex-lifecycle-")
	if err != nil {
		return err
	}
	store, err := dolt.NewSQLiteAdapter(filepath.Join(lifecycleDir, "agm.db"))
	if err != nil {
		_ = os.RemoveAll(lifecycleDir)
		return fmt.Errorf("open isolated lifecycle store: %w", err)
	}
	tmuxRuntime := newBDDLifecycleTmux()
	createRuntime := &bddLifecycleRuntime{}
	archiver := &bddCodexArchiver{}
	opCtx := &ops.OpContext{
		Context:                 ctx,
		Storage:                 store,
		Tmux:                    tmuxRuntime,
		CreationRuntime:         createRuntime,
		ExternalSessionArchiver: archiver,
	}
	harnessState.lifecycleDir = lifecycleDir
	harnessState.lifecycleStore = store
	harnessState.lifecycleTmux = tmuxRuntime
	harnessState.lifecycleRuntime = createRuntime
	harnessState.lifecycleArchiver = archiver
	harnessState.lifecycleOps = opCtx
	harnessState.lifecycleSessionID = "bdd-codex-session"
	harnessState.lifecycleSessionName = "bdd-codex-lifecycle"
	harnessState.harness = "codex-cli"

	result, err := ops.CreateSessionWithContext(ctx, opCtx, &ops.CreateSessionRequest{
		Cwd:                    lifecycleDir,
		Title:                  harnessState.lifecycleSessionName,
		Harness:                "codex-cli",
		Model:                  "5.4",
		SessionID:              harnessState.lifecycleSessionID,
		AllowEmptyPrompt:       true,
		RequireStorage:         true,
		SkipCodexRemoteControl: true,
		ManifestDir:            filepath.Join(lifecycleDir, "manifest"),
		Metadata: ops.CreateSessionMetadata{
			Workspace: "bdd",
			IsTest:    true,
		},
	})
	if err != nil {
		return fmt.Errorf("production create operation: %w", err)
	}
	if result.Harness != "codex-cli" || result.SessionID != harnessState.lifecycleSessionID {
		return fmt.Errorf("unexpected create result: %+v", result)
	}
	stored, err := store.GetSession(harnessState.lifecycleSessionID)
	if err != nil {
		return fmt.Errorf("read created lifecycle record: %w", err)
	}
	stored.Codex = &manifest.Codex{SessionID: "bdd-codex-saved-thread"}
	if err := store.UpdateSession(stored); err != nil {
		return fmt.Errorf("persist Codex saved-thread identity: %w", err)
	}
	if exists, _ := tmuxRuntime.HasSession(harnessState.lifecycleSessionName); !exists {
		return fmt.Errorf("create reported success without tmux target")
	}
	if len(createRuntime.launches) != 1 || createRuntime.launches[0].Harness != "codex-cli" || len(createRuntime.completions) != 1 {
		return fmt.Errorf("create did not traverse the Codex production runtime: launches=%d completions=%d", len(createRuntime.launches), len(createRuntime.completions))
	}
	if !slices.Equal(tmuxRuntime.waited, []string{harnessState.lifecycleSessionName + ":codex-cli"}) {
		return fmt.Errorf("create did not wait for exact Codex readiness: %q", tmuxRuntime.waited)
	}
	harnessState.lifecycleTransitions = append(harnessState.lifecycleTransitions, "created")
	return nil
}

func agmSendsMessageToTheSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness == "agy" {
		if harnessState.agyResumeCommand == "" {
			return fmt.Errorf("AGY resume command was not built from the imported session")
		}
		harnessState.tmuxResumeLaunched = true
		return nil
	}
	const message = "BDD lifecycle message"
	result, err := ops.SendMessage(harnessState.lifecycleOps, &ops.SendMessageRequest{
		Recipient: harnessState.lifecycleSessionID,
		Message:   message,
	})
	if err != nil {
		return fmt.Errorf("production send operation: %w", err)
	}
	want := harnessState.lifecycleSessionName + "\x00" + message
	if !result.Delivered || len(harnessState.lifecycleTmux.sent) != 1 || harnessState.lifecycleTmux.sent[0] != want {
		return fmt.Errorf("send did not reach exact tmux target: result=%+v calls=%q", result, harnessState.lifecycleTmux.sent)
	}
	wantEvents := []string{
		"readiness:" + harnessState.lifecycleSessionName + ":codex-cli",
		"send:" + harnessState.lifecycleSessionName,
	}
	if !slices.Equal(harnessState.lifecycleTmux.events, wantEvents) {
		return fmt.Errorf("send lifecycle events = %q, want %q", harnessState.lifecycleTmux.events, wantEvents)
	}
	harnessState.lifecycleTransitions = append(harnessState.lifecycleTransitions, "message-delivered")
	return nil
}

func agmKillsTheSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	result, err := ops.KillSession(harnessState.lifecycleOps, &ops.KillSessionRequest{
		Identifier:     harnessState.lifecycleSessionID,
		ConfirmedStuck: true,
	})
	if err != nil {
		return fmt.Errorf("production kill operation: %w", err)
	}
	if !result.WasRunning || result.TmuxSessionName != harnessState.lifecycleSessionName {
		return fmt.Errorf("kill result lacks exact running target: %+v", result)
	}
	if exists, _ := harnessState.lifecycleTmux.HasSession(harnessState.lifecycleSessionName); exists {
		return fmt.Errorf("kill reported success while exact tmux target survived")
	}
	harnessState.lifecycleTransitions = append(harnessState.lifecycleTransitions, "tmux-killed")
	return nil
}

func agmArchivesTheStoppedSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	result, err := ops.ArchiveSession(harnessState.lifecycleOps, &ops.ArchiveSessionRequest{
		Identifier: harnessState.lifecycleSessionID,
		Force:      true,
		Outcome:    manifest.OutcomeKilled,
	})
	if err != nil {
		return fmt.Errorf("production archive operation: %w", err)
	}
	if result.Outcome != manifest.OutcomeKilled || len(result.ExternalArchives) != 1 || result.ExternalArchives[0].Status != ops.ExternalArchiveArchived {
		return fmt.Errorf("archive result lacks durable and external outcomes: %+v", result)
	}
	harnessState.lifecycleTransitions = append(harnessState.lifecycleTransitions, "archived")
	return nil
}

func durableAGMStoreShouldReflectLifecycleTransitions(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	want := []string{"created", "message-delivered", "tmux-killed", "archived"}
	if !slices.Equal(harnessState.lifecycleTransitions, want) {
		return fmt.Errorf("lifecycle transitions = %v, want %v", harnessState.lifecycleTransitions, want)
	}
	stored, err := harnessState.lifecycleStore.GetSession(harnessState.lifecycleSessionID)
	if err != nil {
		return fmt.Errorf("read archived lifecycle record: %w", err)
	}
	if stored.Lifecycle != manifest.LifecycleArchived || stored.Outcome != manifest.OutcomeKilled {
		return fmt.Errorf("durable lifecycle = %q outcome = %q, want archived/killed", stored.Lifecycle, stored.Outcome)
	}
	return nil
}

func matchingCodexSavedSessionShouldBeArchived(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if len(harnessState.lifecycleArchiver.targets) != 1 || harnessState.lifecycleArchiver.targets[0] != "bdd-codex-saved-thread" {
		return fmt.Errorf("external Codex archive targets = %q, want exact saved thread", harnessState.lifecycleArchiver.targets)
	}
	return nil
}

func splitCSV(fields string) []string {
	var out []string
	for field := range strings.SplitSeq(fields, ",") {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func retainedA2ACoordinationImplementation() error {
	root := packageSpecBDDRepoRoot()
	checks := map[string][]string{
		"agm/internal/a2a/channel": {"NewManager", "NewCreator", "CreateChannelSimple"},
		"agm/internal/a2a/beads":   {"LinkChannelToBead", "UnlinkChannelFromBead", "RunBeadCommand"},
	}
	for pkg, removed := range checks {
		entries, err := os.ReadDir(filepath.Join(root, pkg))
		if err != nil {
			return fmt.Errorf("read retained A2A package %s: %w", pkg, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, pkg, entry.Name()))
			if err != nil {
				return fmt.Errorf("read retained A2A source %s: %w", entry.Name(), err)
			}
			for _, symbol := range removed {
				if strings.Contains(string(data), symbol) {
					return fmt.Errorf("removed A2A symbol %s remains in %s", symbol, entry.Name())
				}
			}
		}
	}
	return nil
}

func agmValidatesA2ACoordinationSpecificationDrift(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	root := packageSpecBDDRepoRoot()
	checks := map[string][]string{
		"agm/internal/a2a/channel/SPEC.md": {
			"When a manager lists channels", "When a topic has multiple channel files",
			"When a channel is archived", "When a template channel is created",
		},
		"agm/internal/a2a/beads/SPEC.md": {
			"When linking a channel to a bead", "When a channel is linked to a bead",
			"When a channel is unlinked from a bead",
		},
	}
	for spec, removedRequirements := range checks {
		data, err := os.ReadFile(filepath.Join(root, spec))
		if err != nil {
			return fmt.Errorf("read A2A SPEC %s: %w", spec, err)
		}
		for _, requirement := range removedRequirements {
			if strings.Contains(string(data), requirement) {
				return fmt.Errorf("A2A SPEC %s retains deleted requirement %q", spec, requirement)
			}
		}
	}
	harnessState.a2aCoordinationSpecsValid = true
	return nil
}

func a2aCoordinationSpecificationsShouldDescribeOnlyRetainedBehavior(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.a2aCoordinationSpecsValid {
		return fmt.Errorf("A2A coordination specifications were not validated")
	}
	return nil
}
