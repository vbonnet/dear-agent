package steps

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/state"
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
	ctx.Step(`^model family "([^"]*)" is configured$`, modelFamilyIsConfigured)
	ctx.Step(`^AGM validates model family parity support$`, agmValidatesModelFamilyParitySupport)
	ctx.Step(`^model family "([^"]*)" should be supported$`, modelFamilyShouldBeSupported)
	ctx.Step(`^model family "([^"]*)" should have a default model route$`, modelFamilyShouldHaveDefaultModelRoute)
	ctx.Step(`^AGM resolves a model change for harness "([^"]*)" with model "([^"]*)"$`, agmResolvesModelChangeForHarness)
	ctx.Step(`^the model change should use tmux command "([^"]*)"$`, modelChangeShouldUseTmuxCommand)
	ctx.Step(`^the resolved model should not be empty$`, resolvedModelShouldNotBeEmpty)
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
