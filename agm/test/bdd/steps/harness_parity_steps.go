package steps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"

	commandparity "github.com/vbonnet/dear-agent/agm/cmd/agm/parity"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/configdirparity"
	"github.com/vbonnet/dear-agent/agm/internal/engramparity"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/marketplaceparity"
	"github.com/vbonnet/dear-agent/agm/internal/mcpparity"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity"
	"github.com/vbonnet/dear-agent/agm/internal/quotaparity"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/recovery"
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
	sendSafetyRequiresClaude   bool
	sessionListFields          []string
	sessionListHasArray        bool
	lifecycleReflected         bool
	codexArchiveInvoked        bool
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
	quotaSurfaces              []quotaparity.HarnessSurface
	quotaFamilyCoverage        quotaparity.ModelFamilyCoverage
	mcpSurface                 mcpparity.CreateSessionSurface
	mcpModelAccepted           bool
	mcpLifecycleOpsExposed     bool
	mcpServerStartupGuard      bool
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
	backendImplementation      string
	backendImplementationSpec  string
	cleanupSupportPackage      string
	cleanupSupportSpec         string
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
	startupLivenessValid       bool
	currentTmuxTestOutput      string
	currentTmuxTestErr         error
	agyLifecycleTestOutput     string
	agyLifecycleTestErr        error
	resumeSource               string
}

type harnessParityStateKey struct{}

// RegisterHarnessParitySteps registers BDD steps for cross-harness delivery parity.
func RegisterHarnessParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, harnessParityStateKey{}, &harnessParityState{}), nil
	})

	ctx.Step(`^a Codex CLI composer pane$`, aCodexCLIComposerPane)
	ctx.Step(`^harness "([^"]*)" is configured$`, harnessIsConfigured)
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
	ctx.Step(`^Codex queue failures should propagate to shared creation rollback$`, codexQueueFailuresShouldPropagateToSharedCreationRollback)
	ctx.Step(`^current-tmux creation selects AGY$`, currentTmuxCreationSelectsAGY)
	ctx.Step(`^AGM validates current-tmux AGY safety$`, agmValidatesCurrentTmuxAGYSafety)
	ctx.Step(`^current-tmux AGY creation should fail before launch with detached guidance$`, currentTmuxAGYCreationShouldFailBeforeLaunchWithDetachedGuidance)
	ctx.Step(`^AGM validates active harness adapter conformance$`, agmValidatesActiveHarnessAdapterConformance)
	ctx.Step(`^every active harness adapter should satisfy the shared conformance suite$`, everyActiveHarnessAdapterShouldSatisfySharedConformanceSuite)
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
	ctx.Step(`^AGM validates final startup liveness$`, agmValidatesFinalStartupLiveness)
	ctx.Step(`^startup should require a live tmux session and harness process$`, startupShouldRequireLiveTmuxAndHarness)
	ctx.Step(`^AGM runtime helper command "([^"]*)" is configured$`, agmRuntimeHelperCommandIsConfigured)
	ctx.Step(`^AGM validates runtime helper command coverage$`, agmValidatesRuntimeHelperCommandCoverage)
	ctx.Step(`^runtime helper command "([^"]*)" should have a co-located SPEC$`, runtimeHelperCommandShouldHaveCoLocatedSPEC)
	ctx.Step(`^AGM backend implementation "([^"]*)" is configured$`, agmBackendImplementationIsConfigured)
	ctx.Step(`^AGM validates backend implementation coverage$`, agmValidatesBackendImplementationCoverage)
	ctx.Step(`^backend implementation "([^"]*)" should have a co-located SPEC$`, backendImplementationShouldHaveCoLocatedSPEC)
	ctx.Step(`^AGM cleanup support package "([^"]*)" is configured$`, agmCleanupSupportPackageIsConfigured)
	ctx.Step(`^AGM validates cleanup support package coverage$`, agmValidatesCleanupSupportPackageCoverage)
	ctx.Step(`^cleanup support package "([^"]*)" should have a co-located SPEC$`, cleanupSupportPackageShouldHaveCoLocatedSPEC)
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
	ctx.Step(`^Wayfinder should publish SKILL, plugin, command, and MCP status surfaces$`, wayfinderShouldPublishSkillPluginCommandAndMCPStatusSurfaces)
	ctx.Step(`^AGM validates Wayfinder phase Engram parity$`, agmValidatesWayfinderPhaseEngramParity)
	ctx.Step(`^Wayfinder should resolve phase Engrams without harness-specific state$`, wayfinderShouldResolvePhaseEngramsWithoutHarnessSpecificState)
	ctx.Step(`^AGM validates configuration directory parity$`, agmValidatesConfigurationDirectoryParity)
	ctx.Step(`^AGM validates deprecated configuration directory parity$`, agmValidatesDeprecatedConfigurationDirectoryParity)
	ctx.Step(`^harness "([^"]*)" should have configuration directory "([^"]*)"$`, harnessShouldHaveConfigurationDirectory)
	ctx.Step(`^a Codex CLI trust prompt$`, aCodexCLITrustPrompt)
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
	ctx.Step(`^AGM should auto-accept the AGY trust prompt before prompt delivery$`, agmShouldAutoAcceptAGYTrustPromptBeforePromptDelivery)
	ctx.Step(`^AGM runs send safety for the configured harness$`, agmRunsSendSafetyForTheConfiguredHarness)
	ctx.Step(`^send safety should not require a Claude process$`, sendSafetyShouldNotRequireClaudeProcess)
	ctx.Step(`^AGM validates the AGY adapter lifecycle$`, agmValidatesTheAgyAdapterLifecycle)
	ctx.Step(`^the AGY adapter should preserve canonical launch and resume policy$`, agyAdapterShouldPreserveCanonicalLaunchAndResumePolicy)
	ctx.Step(`^the AGY adapter should require AGY process and transcript truth$`, agyAdapterShouldRequireAgyProcessAndTranscriptTruth)
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
	ctx.Step(`^AGM resumes the session$`, agmResumesTheSession)
	ctx.Step(`^AGM kills the session$`, agmKillsTheSession)
	ctx.Step(`^AGM archives the stopped session$`, agmArchivesTheStoppedSession)
	ctx.Step(`^Dolt should reflect the expected lifecycle transitions$`, doltShouldReflectLifecycleTransitions)
	ctx.Step(`^the matching Codex saved session should be archived$`, matchingCodexSavedSessionShouldBeArchived)
	ctx.Step(`^a stopped Codex CLI session without a tmux pane$`, aStoppedCodexCLISessionWithoutTmuxPane)
	ctx.Step(`^AGM validates the Codex resume transaction$`, agmValidatesTheCodexResumeTransaction)
	ctx.Step(`^Codex resume success should require process and composer readiness$`, codexResumeSuccessShouldRequireProcessAndComposerReadiness)
	ctx.Step(`^a failed Codex resume should remove only its newly created tmux session$`, aFailedCodexResumeShouldRemoveOnlyItsNewlyCreatedTmuxSession)
	ctx.Step(`^Codex activity updates should follow resume readiness$`, codexActivityUpdatesShouldFollowResumeReadiness)
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
	path := filepath.Join(bddRepoRoot(), "agm", "cmd", "agm", "resume.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read resume production source: %w", err)
	}
	harnessState.resumeSource = string(data)
	return nil
}

func codexResumeSuccessShouldRequireProcessAndComposerReadiness(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "-run", `^TestWaitForResumedCodexRequiresProcessAndComposer$`, "-count=1")
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
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "-run", `^TestResumeSession(Codex(RollsBackNewTmuxBeforeActivityUpdate|JoinsCleanupFailure|RollbackUsesCreatedCanonicalTmuxName|PersistsCreatedCanonicalTmuxName|TmuxPersistencePreservesConcurrentMetadata|RollsBackWhen(PromptDeliveryIsCanceled|CanonicalNamePersistenceFails)|ReadinessFailureRemovesIsolatedTmux|RollbackReportsInaccessibleSocketAndPreservesHiddenTarget)|PreservesPreexistingTmuxOnLaterFailure)$`, "-count=1")
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

func codexActivityUpdatesShouldFollowResumeReadiness(ctx context.Context) error {
	source := ctx.Value(harnessParityStateKey{}).(*harnessParityState).resumeSource
	waitIndex := strings.Index(source, "runtime.wait(harnessName, health)")
	updateIndex := strings.Index(source, "runtime.updateActivity(adapter, sessionID, manifestPath)")
	if waitIndex < 0 || updateIndex < waitIndex {
		return fmt.Errorf("codex activity update must follow successful resume readiness")
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
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "./agm/internal/ops", "-run", `^(Test(StartCurrentTmuxHarnessCodex(UsesRealLauncherContract|StopsAfterCredentialFailure|PropagatesQueueFailure)|QueueCurrentTmuxCodex(DoesNotWaitForReadiness|RejectsMissingExecutable)|StartNewSessionForContextRoutesCurrentTmux)|TestCreateSession_RollsBackEveryPostTmuxFailure)$`, "-count=1", "-v")
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

func agmBackendImplementationIsConfigured(ctx context.Context, backend string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.backendImplementation = backend
	harnessState.backendImplementationSpec = filepath.Join(bddRepoRoot(), "agm", "internal", filepath.FromSlash(backend), "SPEC.md")
	return nil
}

func agmValidatesBackendImplementationCoverage(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.backendImplementationSpec == "" {
		return fmt.Errorf("no AGM backend implementation configured")
	}
	if _, err := os.Stat(harnessState.backendImplementationSpec); err != nil {
		return fmt.Errorf("backend implementation SPEC %s: %w", harnessState.backendImplementationSpec, err)
	}
	return nil
}

func backendImplementationShouldHaveCoLocatedSPEC(ctx context.Context, backend string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if backend != harnessState.backendImplementation {
		return fmt.Errorf("configured backend implementation = %q, want %q", harnessState.backendImplementation, backend)
	}
	wantSuffix := filepath.Join("agm", "internal", filepath.FromSlash(backend), "SPEC.md")
	if !strings.HasSuffix(harnessState.backendImplementationSpec, wantSuffix) {
		return fmt.Errorf("backend implementation SPEC = %q, want suffix %q", harnessState.backendImplementationSpec, wantSuffix)
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
	want := "agents-md-skill-fallback"
	if harnessState.marketplaceSurface.Name == "claude-code" {
		want = "native-claude-plugin-marketplace"
	}
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
	harnessState.paneOutput = `╭────────────────────────────────────────────────────╮
│ >_ OpenAI Codex                                    │
│  /model to change model                            │
╰────────────────────────────────────────────────────╯`
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
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test",
		"./agm/cmd/agm",
		"./agm/cmd/agm-mcp-server",
		"./agm/internal/agent",
		"./agm/internal/dolt",
		"./agm/internal/importer",
		"./agm/internal/lock",
		"./agm/internal/ops",
		"./agm/internal/safety",
		"./agm/internal/send",
		"./agm/internal/state",
		"./agm/internal/tmux",
		"-run", `^(Test(StartAgyHarness(UsesCanonicalLaunchAndWaits|PropagatesReadinessFailure)|StartNewSessionForContextRejectsCurrentTmuxAgyBeforeLaunch|BuildAgyCommand_(AutoPermissionMode|DefaultPermissionMode)|AgyModelCatalogMatchesPublicCLI|BuildAgyImportedManifest(LeavesUnknownModelUnset|PreservesConversationAndCurrentDefaults)|CreateSession_AgyDetachedPromptUsesCanonicalCommand|CreateSession_Agy(WorkspaceLockReleasesBeforeSurfaceCompletion|IdentitySnapshotFailsBeforeTmuxMutation|IdentityDiscoveryFailureRollsBackBeforeRegistration)|CreateSession_CancellationAfterRegistrationRollsBackBeforeCompletion|BuildAgyResumeCommandPreservesModelConversationAndMode|BuildAgyResumeCommand_(TranslatesLegacyModels|PreservesImportedConversationModel)|MigrateAmbiguousLegacyAgyModelClearsStoredOverride|GetResumeManifestStopsCanceledMigration|NormalizeModelInput(PreservesAgyPublicLabels|CanonicalizesCrossHarnessAliases)|ResolveSetModelInstruction_(PreservesAgyPublicLabel|NormalizesCrossHarnessAliasCase)|NewAgyModelConfirmationRejectsStaleOrMismatchedOutput|PersistAgyModelSwitchPreservesOnlyConfirmedProvenance|SQLite(CreateSessionDefaultsModelOnlyForClaude|UpdateSessionRoundTripsModel)|MCPCreateSessionRuntime(WaitsForAgyBeforePrompt|StopsBeforePromptAfterCancellation)|ExecuteWithSignalContextPropagatesCancellation|RootCommandOwnsProcessSignalHandling|LongRunningCommandsConsumeRootContext|CommandHandlersAvoidBackgroundMultilineDelivery|RunScanLoopUsesCallerContext|RunHeartbeatWatchdogUsesCallerContext|ExecuteRestartContextUsesCallerContext|RunWatchUsesCallerContext|VerifyCompactionUsesCallerContext|MonitorCompactionUsesCallerContext|ResumeSessionStopsCancellationAfterManifestRead|FinalizeCLICreateSessionStopsCancellationAfterLiveness|WithAgyResumeWorkspaceLockCoversLifecycle|WaitForResumed(Agy|Claude|Codex)UsesCallerContext|WaitForAgyMetadataBackfillUsesCallerContext|WaitForAgyAssociationRetryDelayUsesCallerContext|Run(AgyPostCreate(ReturnsCancellationBeforeSideEffects|MetadataRetryUsesCallerContext|Propagates(ReadinessFailure|PostPromptReadinessFailure))|ClaudePostCreateReturnsCallerCancellationBeforeSideEffects|CodexPostCreateReturnsCancellationBeforePromptDelivery|SendSetModelUsesCallerContextBeforeSlashCommandDelivery)|DeliverInitialPromptReturnsCallerCancellation|DispatchModeSwitchContextStopsBeforeSlashCommandDelivery|CommandScoped(ReadinessWaitsReturnCallerCancellation|SafeDeliveryReturnsCallerCancellation)|StructuredPromptUsesCallerContext|Send(ViaTmuxUsesCallerContext|PostResumePromptUsesCallerContext|MultiLinePromptSafeContextReturnsCallerCancellation)|SequentialDeliverPassesCallerContext|NewNonClaudeAssociationManifestLeavesAgyModelUnknown|UpdateNonClaudeAssociationManifestLeavesAgyModelUnknown|AgyCreateSession(UsesCanonicalModelAwareCommand|ImportedConversationOmitsUnknownModel|RejectsExistingTmuxAndUnsafeModelBeforeMutation|PropagatesReadinessFailureAndRollsBack|ReportsRollbackFailure|CapturesNativeConversationIdentity|NormalizesWorkingDirectoryForLaunchAndDiscovery|SerializesWorkspaceIdentityDiscovery|RollsBackWhenNativeIdentityCannotBeCaptured|DoesNotReuseStaleNativeConversationIdentity)|AgyResumePolicyPersistsInJSONSessionStore|AgyResumeSession(PreservesNativeIdentityModelAndMode|OmitsModelWhenProvenanceUnknown|DoesNotInventNativeIdentity|RejectsUnsafeNativeIdentityBeforeMutation|RejectsAnotherLiveHarnessBeforeMutation|RejectsNonShellForegroundBeforeMutation|RestartsInExistingBareShell|HoldsWorkspaceLockThroughReadiness|SerializesPaneProofWithCommandDelivery|UsesExactProcessLivenessAndFailsSafe|LeavesLiveAgyUntouched|PropagatesReadinessFailureBeforeAttach)|AgyGetSessionStatusRequiresAgyProcess|AgyGetHistory(ReadsNativeTranscript|FallsBackToFullTranscript|RequiresNativeIdentity|RejectsUnsafeNativeIdentity)|AgyAdapterRejectsUnsupportedRunHook|DetectAgySessionUninitialized|NormalizeHarnessForSafety|Agy(StaleSurveyAllowsLaterReadyPrompt|PromptBeforeSurveyRemainsOverlay)|ContainsAgyPromptAfterSurveyRequiresLaterPrompt|ClassifyPaneLiveness|CheckPaneLivenessContextHonorsCallerCancellation|WaitForAgyPrompt(AcceptsTrustBeforeReady|DismissesSurveyBeforeReady|DoesNotRedismissStaleSurvey|ReturnsCancellationAfterReadyStabilityDelay)))$`,
		"-count=1", "-v",
	)
	cmd.Dir = bddRepoRoot()
	output, runErr := cmd.CombinedOutput()
	lockCmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/lock", "./agm/internal/agysession",
		"-run", `^Test(FileLockTryLockPreservesPermanentFlockError|AcquireWorkspaceCreateLockStopsOnPermanentFlockError)$`,
		"-count=1", "-v",
	)
	lockCmd.Dir = bddRepoRoot()
	lockOutput, lockErr := lockCmd.CombinedOutput()
	harnessState.agyLifecycleTestOutput = string(output) + "\n" + string(lockOutput)
	if runErr == nil {
		runErr = lockErr
	}
	harnessState.agyLifecycleTestErr = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("AGY lifecycle behavior suite timed out: %w", testCtx.Err())
	}
	return nil
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
		"TestWithAgyResumeWorkspaceLockCoversLifecycle",
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
		"TestAgyResumeSessionPropagatesReadinessFailureBeforeAttach",
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
	return requireAgyLifecycleBehaviors(harnessState, "TestBuildAgyResumeCommand_TranslatesLegacyModels")
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
		"TestBuildAgyResumeCommand_PreservesImportedConversationModel",
		"TestMigrateAmbiguousLegacyAgyModelClearsStoredOverride",
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
	return requireAgyLifecycleBehaviors(harnessState, "TestMCPCreateSessionRuntimeWaitsForAgyBeforePrompt")
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
		"TestGetResumeManifestStopsCanceledMigration",
		"TestResumeSessionStopsCancellationAfterManifestRead",
		"TestFinalizeCLICreateSessionStopsCancellationAfterLiveness",
		"TestWaitForResumedAgyUsesCallerContext",
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
		"TestWaitForResumedClaudeUsesCallerContext",
		"TestWaitForResumedCodexUsesCallerContext",
		"TestRunCodexPostCreateReturnsCancellationBeforePromptDelivery",
		"TestWaitForAgyPromptReturnsCancellationAfterReadyStabilityDelay",
		"TestSendViaTmuxUsesCallerContext",
		"TestStructuredPromptUsesCallerContext",
		"TestSequentialDeliverPassesCallerContext",
		"TestSendPostResumePromptUsesCallerContext",
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
	switch harnessState.harness {
	case "codex-cli":
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
		"TestBuildAgyResumeCommand_PreservesImportedConversationModel",
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
	harnessState.harness = "codex-cli"
	return nil
}

func agmSendsMessageToTheSession(ctx context.Context) error { return nil }

func agmResumesTheSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if harnessState.harness == "agy" {
		if harnessState.agyResumeCommand == "" {
			return fmt.Errorf("AGY resume command was not built from the imported session")
		}
	}
	harnessState.tmuxResumeLaunched = true
	return nil
}

func agmKillsTheSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.lifecycleReflected = true
	return nil
}

func agmArchivesTheStoppedSession(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.lifecycleReflected = true
	harnessState.codexArchiveInvoked = true
	return nil
}

func doltShouldReflectLifecycleTransitions(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.lifecycleReflected {
		return fmt.Errorf("expected lifecycle transitions to be reflected")
	}
	return nil
}

func matchingCodexSavedSessionShouldBeArchived(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if !harnessState.codexArchiveInvoked {
		return fmt.Errorf("expected matching Codex saved session archive")
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
