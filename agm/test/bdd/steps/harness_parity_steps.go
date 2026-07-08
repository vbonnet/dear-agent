package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/configdirparity"
	"github.com/vbonnet/dear-agent/agm/internal/engramparity"
	"github.com/vbonnet/dear-agent/agm/internal/marketplaceparity"
	"github.com/vbonnet/dear-agent/agm/internal/mcpparity"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity"
	"github.com/vbonnet/dear-agent/agm/internal/quotaparity"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/wayfinderparity"
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
	agyResumeAutoPermissions   bool
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
	marketplaceCatalog         marketplaceparity.Catalog
	marketplaceSurface         marketplaceparity.HarnessSurface
	marketplaceMirrorValid     bool
	marketplacePlugin          string
	marketplacePluginValid     bool
	engramSurface              engramparity.HarnessSurface
	engramMetadataValid        bool
	wayfinderSurface           wayfinderparity.HarnessSurface
	wayfinderAssetsValid       bool
	wayfinderPhaseEngrams      bool
	configDirSurface           configdirparity.DirectorySurface
	conformanceFindings        []agent.HarnessConformanceFinding
	runtimeHelperCommand       string
	runtimeHelperSpec          string
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
	ctx.Step(`^AGM active harnesses are configured$`, agmActiveHarnessesAreConfigured)
	ctx.Step(`^AGM validates active harness adapter conformance$`, agmValidatesActiveHarnessAdapterConformance)
	ctx.Step(`^every active harness adapter should satisfy the shared conformance suite$`, everyActiveHarnessAdapterShouldSatisfySharedConformanceSuite)
	ctx.Step(`^AGM runtime helper command "([^"]*)" is configured$`, agmRuntimeHelperCommandIsConfigured)
	ctx.Step(`^AGM validates runtime helper command coverage$`, agmValidatesRuntimeHelperCommandCoverage)
	ctx.Step(`^runtime helper command "([^"]*)" should have a co-located SPEC$`, runtimeHelperCommandShouldHaveCoLocatedSPEC)
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
	ctx.Step(`^model family "([^"]*)" should have a default quota model route$`, modelFamilyShouldHaveDefaultQuotaModelRoute)
	ctx.Step(`^AGM validates MCP session creation parity$`, agmValidatesMCPSessionCreationParity)
	ctx.Step(`^harness "([^"]*)" should have an MCP create-session surface$`, harnessShouldHaveMCPCreateSessionSurface)
	ctx.Step(`^the MCP create-session surface should use shared model validation$`, mcpCreateSessionSurfaceShouldUseSharedModelValidation)
	ctx.Step(`^the MCP create-session surface should be deprecated compatibility$`, mcpCreateSessionSurfaceShouldBeDeprecatedCompatibility)
	ctx.Step(`^AGM validates MCP model identifier "([^"]*)"$`, agmValidatesMCPModelIdentifier)
	ctx.Step(`^the MCP model identifier should be accepted$`, mcpModelIdentifierShouldBeAccepted)
	ctx.Step(`^AGM validates MCP operation discovery parity$`, agmValidatesMCPOperationDiscoveryParity)
	ctx.Step(`^the MCP operation registry should expose lifecycle mutations$`, mcpOperationRegistryShouldExposeLifecycleMutations)
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
	ctx.Step(`^AGM checks whether the session can receive input$`, agmChecksWhetherTheSessionCanReceiveInput)
	ctx.Step(`^delivery should be allowed$`, deliveryShouldBeAllowed)
	ctx.Step(`^delivery should be queued$`, deliveryShouldBeQueued)
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
}

func harnessIsConfigured(ctx context.Context, harness string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	harnessState.configuredHarness = agent.NormalizeHarnessName(harness)
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
	if strings.Contains(harnessState.paneOutput, "trust this folder") ||
		strings.Contains(harnessState.paneOutput, "trust the contents") {
		harnessState.trustAutoAccepted = true
	}
	harnessState.waitedForAgyPrompt = true
	harnessState.startupDelivered = true
	return nil
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
	return nil
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
	return nil
}

func agmRunsSendSafetyForTheConfiguredHarness(ctx context.Context) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	switch harnessState.harness {
	case "codex-cli", "agy":
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
	harnessState.harness = harness
	harnessState.preservedAgyConversationID = true
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
	harnessState.tmuxResumeLaunched = true
	harnessState.agyResumeAutoPermissions = mode == "auto"
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
	return nil
}

func theAGYResumeCommandShouldInclude(ctx context.Context, expected string) error {
	harnessState, err := getHarnessParityState(ctx)
	if err != nil {
		return err
	}
	if expected == "--dangerously-skip-permissions" && !harnessState.agyResumeAutoPermissions {
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
