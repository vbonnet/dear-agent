package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/internal/hookparity"
)

type hookParityState struct {
	harness                  string
	hooks                    map[string][]bddHookGroup
	specContractReview       bool
	unsupportedOmitted       bool
	postMergeHook            string
	companionOutput          string
	companionRegression      error
	openCodeOutput           string
	openCodeRegression       error
	piTerminalOutput         string
	piTerminalRegression     error
	sharedReminderOutput     string
	sharedReminderRegression error
	boundedFailureOutput     string
	boundedFailureRegression error
	helperReproOutput        string
	helperReproRegression    error
}

type bddHookGroup struct {
	Hooks []bddHookEntry `json:"hooks"`
}

type bddHookEntry struct {
	Command string `json:"command"`
}

type bddHookSettings struct {
	Hooks map[string][]bddHookGroup `json:"hooks"`
}

type bddNamedHookSettings map[string]map[string][]bddHookEntry

type hookParityStateKey struct{}

// RegisterHookParitySteps registers BDD steps for hook harness parity.
func RegisterHookParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, hookParityStateKey{}, &hookParityState{}), nil
	})

	ctx.Step(`^hook harness "([^"]*)" is configured$`, hookHarnessIsConfigured)
	ctx.Step(`^AGM validates hook parity for that harness$`, agmValidatesHookParityForThatHarness)
	ctx.Step(`^hook harness "([^"]*)" should include guardrail hook "([^"]*)"$`, hookHarnessShouldIncludeGuardrailHook)
	ctx.Step(`^hook harness "([^"]*)" should include Beads lifecycle hook "([^"]*)"$`, hookHarnessShouldIncludeBeadsLifecycleHook)
	ctx.Step(`^hook harness "([^"]*)" should expose bounded SPEC contract review$`, hookHarnessShouldExposeBoundedSPECContractReview)
	ctx.Step(`^hook harness "([^"]*)" should omit unsupported legacy hook projections$`, hookHarnessShouldOmitUnsupportedLegacyHookProjections)
	ctx.Step(`^staged SPEC contract feedback is configured$`, stagedSPECContractFeedbackIsConfigured)
	ctx.Step(`^AGM exercises the shared reminder across all projected harness adapters$`, agmExercisesSharedSPECReminder)
	ctx.Step(`^every reminder should route to the canonical authoring page and single-source skill$`, sharedSPECReminderUsesCanonicalAuthoringRoute)
	ctx.Step(`^terminal SPEC feedback identity is configured$`, terminalSPECFeedbackIdentityIsConfigured)
	ctx.Step(`^AGM exercises sibling continuations and repeated SPEC identities across native terminal adapters$`, agmExercisesBoundedSPECFeedbackIdentities)
	ctx.Step(`^fresh SPEC identities should block once while repeats yield without claiming compliance$`, nativeTerminalAdaptersBoundFreshSPECFeedback)
	ctx.Step(`^installed SPEC helper status is configured$`, installedSPECStatusIsConfigured)
	ctx.Step(`^AGM rebuilds the expected helper with distinct wall-clock inputs$`, agmRebuildsExpectedSPECHelper)
	ctx.Step(`^the expected helper bytes should remain identical for unchanged source and provenance$`, expectedSPECHelperBytesRemainIdentical)
	ctx.Step(`^OpenCode idle-session SPEC feedback is configured$`, openCodeIdleSessionSPECFeedbackIsConfigured)
	ctx.Step(`^AGM exercises repeated, synthetic, message-capacity, session-capacity, and deleted-session events$`, agmExercisesOpenCodeIdleSessionEvents)
	ctx.Step(`^each real turn and the global session table should remain bounded while tracked deletion admits yielded sessions$`, openCodeIdleSessionEventsRemainBounded)
	ctx.Step(`^Pi terminal hook aggregation is configured$`, piTerminalHookAggregationIsConfigured)
	ctx.Step(`^AGM exercises Pi terminal handler count, runtime, deadline, and output bounds$`, agmExercisesPiTerminalHookBounds)
	ctx.Step(`^Pi should fail closed within its budgets while preserving multi-handler aggregation$`, piTerminalHookBoundsRemainFailClosed)
	ctx.Step(`^the repository post-merge hook is configured$`, repositoryPostMergeHookIsConfigured)
	ctx.Step(`^AGM validates repository post-merge hook coverage$`, agmValidatesRepositoryPostMergeHookCoverage)
	ctx.Step(`^the repository post-merge hook should include lifecycle safeguard "([^"]*)"$`, repositoryPostMergeHookShouldIncludeLifecycleSafeguard)
	ctx.Step(`^AGM runs detached archive companion startup regressions$`, agmRunsDetachedArchiveCompanionStartupRegressions)
	ctx.Step(`^a mixed revision or missing startup acknowledgement should fail before async success$`, mixedRevisionOrMissingStartupAcknowledgementShouldFailBeforeAsyncSuccess)
	ctx.Step(`^AGM renders the canonical AGM companion install plan$`, agmRendersCanonicalAGMCompanionInstallPlan)
	ctx.Step(`^the root AGM install plan should build and install the companion pair$`, rootAGMInstallPlanShouldBuildAndInstallCompanionPair)
}

func hookHarnessIsConfigured(ctx context.Context, harness string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = agent.NormalizeHarnessName(harness)
	return nil
}

func agmValidatesHookParityForThatHarness(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" {
		return fmt.Errorf("no hook harness configured")
	}
	root := hookBDDRepoRoot()
	state.hooks = nil
	state.specContractReview = false
	state.unsupportedOmitted = false
	switch state.harness {
	case "agy":
		path := filepath.Join(root, ".agents", "hooks.json")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read hook manifest %s: %w", path, err)
		}
		var settings bddNamedHookSettings
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse named hook manifest %s: %w", path, err)
		}
		entries := settings["spec-contract-guard"]["Stop"]
		state.hooks = map[string][]bddHookGroup{"Stop": {{Hooks: entries}}}
		state.specContractReview = len(entries) == 1 && entries[0].Command == "/usr/local/libexec/dear-agent-spec-contract-hook --root-from-workspace-stdin --provider antigravity --event Stop"
		state.unsupportedOmitted = len(settings) == 1 && len(settings["spec-contract-guard"]) == 1
	case "opencode-cli":
		path := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.mjs")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read hook plugin %s: %w", path, err)
		}
		plugin := string(data)
		state.specContractReview = strings.Contains(plugin, "session.idle") && strings.Contains(plugin, "cmd/spec-contract-hook") && strings.Contains(plugin, `"opencode"`) && strings.Contains(plugin, "promptAsync")
		state.unsupportedOmitted, err = hookparity.OpenCodeLegacyProjectionIsInactive(root)
		if err != nil {
			return err
		}
	default:
		path, ok := hookManifestPath(state.harness)
		if !ok {
			return fmt.Errorf("harness %q has no hook configuration surface", state.harness)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read hook manifest %s: %w", path, err)
		}
		var settings bddHookSettings
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse hook manifest %s: %w", path, err)
		}
		state.hooks = settings.Hooks
		state.specContractReview = hookCommandsContain(state.hooks, "cmd/spec-contract-hook")
	}
	if len(state.hooks) == 0 && !state.specContractReview {
		return fmt.Errorf("harness %q has no validated hook capability", state.harness)
	}
	return nil
}

func hookHarnessShouldIncludeGuardrailHook(ctx context.Context, harness, guardrail string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if agent.NormalizeHarnessName(harness) != state.harness {
		return fmt.Errorf("configured hook harness = %q, want %q", state.harness, harness)
	}
	if hookCommandsContain(state.hooks, guardrail) {
		return nil
	}
	return fmt.Errorf("harness %q missing guardrail hook %q", state.harness, guardrail)
}

func hookHarnessShouldIncludeBeadsLifecycleHook(ctx context.Context, harness, event string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if agent.NormalizeHarnessName(harness) != state.harness {
		return fmt.Errorf("configured hook harness = %q, want %q", state.harness, harness)
	}
	prefix, ok := map[string]string{
		"codex-cli":    "codex",
		"agy":          "antigravity",
		"opencode-cli": "opencode",
		"pi-cli":       "codex",
	}[state.harness]
	if !ok {
		return fmt.Errorf("harness %q is not expected to have Beads lifecycle hooks", state.harness)
	}
	want := "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on " + prefix + "-hook " + event
	for _, group := range state.hooks[event] {
		for _, hook := range group.Hooks {
			if strings.Contains(hook.Command, want) {
				return nil
			}
		}
	}
	return fmt.Errorf("harness %q missing Beads lifecycle hook %q", state.harness, want)
}

func hookHarnessShouldExposeBoundedSPECContractReview(ctx context.Context, harness string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if agent.NormalizeHarnessName(harness) != state.harness {
		return fmt.Errorf("configured hook harness = %q, want %q", state.harness, harness)
	}
	if !state.specContractReview {
		return fmt.Errorf("harness %q has no bounded SPEC contract review transport", state.harness)
	}
	return nil
}

func hookHarnessShouldOmitUnsupportedLegacyHookProjections(ctx context.Context, harness string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if agent.NormalizeHarnessName(harness) != state.harness {
		return fmt.Errorf("configured hook harness = %q, want %q", state.harness, harness)
	}
	if !state.unsupportedOmitted {
		return fmt.Errorf("harness %q retains an unsupported legacy hook projection", state.harness)
	}
	return nil
}

func openCodeIdleSessionSPECFeedbackIsConfigured(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = "opencode-cli"
	return agmValidatesHookParityForThatHarness(ctx)
}

func stagedSPECContractFeedbackIsConfigured(ctx context.Context) error {
	_, err := getHookParityState(ctx)
	return err
}

func agmExercisesSharedSPECReminder(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.sharedReminderOutput, state.sharedReminderRegression = runLocalGuardrailGoTest(ctx,
		`^TestRunProvidesCooperativeTerminalReminderForValidStagedContract$`,
		"./cmd/spec-contract-hook",
	)
	return nil
}

func sharedSPECReminderUsesCanonicalAuthoringRoute(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.sharedReminderRegression != nil {
		return fmt.Errorf("shared staged-SPEC reminder regression: %w: %s", state.sharedReminderRegression, state.sharedReminderOutput)
	}
	if !strings.Contains(state.sharedReminderOutput, "--- PASS: TestRunProvidesCooperativeTerminalReminderForValidStagedContract") {
		return fmt.Errorf("shared staged-SPEC reminder output omitted its passing regression: %s", state.sharedReminderOutput)
	}
	return nil
}

func terminalSPECFeedbackIdentityIsConfigured(ctx context.Context) error {
	_, err := getHookParityState(ctx)
	return err
}

func agmExercisesBoundedSPECFeedbackIdentities(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	adapterOutput, adapterRegression := runLocalGuardrailNamedGoTests(ctx,
		"./cmd/spec-contract-hook",
		"TestTerminalFeedbackClaimDistinguishesSiblingContinuationSnapshotAndTurn",
		"TestTerminalFeedbackClaimBoundsDeterministicValidationFailures",
		"TestPiAdapterReturnsStableFeedbackIdentityForOuterLoop",
		"TestAntigravityDeterministicBlockSupportsZeroBasedExecutionSequence",
		"TestAntigravityFailurePathsWithoutStableIdentityAllowTermination",
		"TestAntigravityWorkspaceRootFailuresContinueOncePerConversation",
		"TestAntigravityWorkspaceRootFailureAllowsWhenOneShotStateIsUnavailable",
		"TestRunYieldsInvalidHookInvocationWithoutAStableRetrySignal",
		"TestRunYieldsOversizedHookInputWithoutAStableRetrySignal",
		"TestRunYieldsMalformedBoundedHookInputWithoutAStableRetrySignal",
	)
	piOutput, piRegression := runLocalGuardrailNamedGoTests(ctx,
		"./agm/internal/permissionparity",
		"TestEmbeddedPiExtensionDecisionParity",
	)
	state.boundedFailureOutput = adapterOutput + "\n" + piOutput
	state.boundedFailureRegression = adapterRegression
	if state.boundedFailureRegression == nil {
		state.boundedFailureRegression = piRegression
	}
	return nil
}

func nativeTerminalAdaptersBoundFreshSPECFeedback(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.boundedFailureRegression != nil {
		return fmt.Errorf("bounded terminal SPEC failure regressions: %w: %s", state.boundedFailureRegression, state.boundedFailureOutput)
	}
	return nil
}

func installedSPECStatusIsConfigured(ctx context.Context) error {
	_, err := getHookParityState(ctx)
	return err
}

func agmRebuildsExpectedSPECHelper(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.helperReproOutput, state.helperReproRegression = runLocalGuardrailNamedGoTests(ctx,
		"./tests/buildstamp",
		"TestSpecContractHookExpectedArtifactIsReproducible",
		"TestSpecContractHookStatusArtifactPreservesDirectExitContract",
	)
	return nil
}

func expectedSPECHelperBytesRemainIdentical(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.helperReproRegression != nil {
		return fmt.Errorf("SPEC helper reproducibility regression: %w: %s", state.helperReproRegression, state.helperReproOutput)
	}
	return nil
}

func agmExercisesOpenCodeIdleSessionEvents(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.openCodeOutput, state.openCodeRegression = runLocalGuardrailGoTest(ctx,
		`^TestOpenCodeSPECContractPlugin(UsesIdleEventAndConservativeTransport|TerminatesProcessGroup)$`,
		"./internal/hookparity",
	)
	return nil
}

func openCodeIdleSessionEventsRemainBounded(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.openCodeRegression != nil {
		return fmt.Errorf("OpenCode idle-session transport regression: %w: %s", state.openCodeRegression, state.openCodeOutput)
	}
	if !strings.Contains(state.openCodeOutput, "--- PASS: TestOpenCodeSPECContractPluginUsesIdleEventAndConservativeTransport") {
		return fmt.Errorf("OpenCode idle-session transport output omitted its passing regression: %s", state.openCodeOutput)
	}
	if !strings.Contains(state.openCodeOutput, "--- PASS: TestOpenCodeSPECContractPluginTerminatesProcessGroup") {
		return fmt.Errorf("OpenCode process-group transport output omitted its passing regression: %s", state.openCodeOutput)
	}
	return nil
}

func piTerminalHookAggregationIsConfigured(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = "pi-cli"
	return agmValidatesHookParityForThatHarness(ctx)
}

func agmExercisesPiTerminalHookBounds(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.piTerminalOutput, state.piTerminalRegression = runLocalGuardrailGoTest(ctx,
		`^(TestPiProductionTerminalTimeoutBudgetsAreHonored|TestEmbeddedPiExtensionDecisionParity)$`,
		"./agm/internal/permissionparity",
	)
	return nil
}

func piTerminalHookBoundsRemainFailClosed(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.piTerminalRegression != nil {
		return fmt.Errorf("pi terminal hook bound regression: %w: %s", state.piTerminalRegression, state.piTerminalOutput)
	}
	if !strings.Contains(state.piTerminalOutput, "--- PASS: TestPiProductionTerminalTimeoutBudgetsAreHonored") ||
		!strings.Contains(state.piTerminalOutput, "--- PASS: TestEmbeddedPiExtensionDecisionParity") {
		return fmt.Errorf("pi terminal hook bound output omitted its passing regression: %s", state.piTerminalOutput)
	}
	return nil
}

func hookCommandsContain(hooks map[string][]bddHookGroup, substr string) bool {
	for _, groups := range hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if strings.Contains(hook.Command, substr) {
					return true
				}
			}
		}
	}
	return false
}

func getHookParityState(ctx context.Context) (*hookParityState, error) {
	state, ok := ctx.Value(hookParityStateKey{}).(*hookParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("hook parity state not initialized")
	}
	return state, nil
}

func repositoryPostMergeHookIsConfigured(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(hookBDDRepoRoot(), "scripts", "git-hooks", "post-merge")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read repository post-merge hook %s: %w", path, err)
	}
	state.postMergeHook = string(data)
	return nil
}

func agmValidatesRepositoryPostMergeHookCoverage(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(state.postMergeHook) == "" {
		return fmt.Errorf("repository post-merge hook was not loaded")
	}
	for _, want := range []string{
		"rebuild_changed_binaries",
		"deploy_host_artifacts",
		"verify_deployment_after_rebuild",
		"transition_merged_beads",
		"sweep_merged_worktrees",
		"exit 0",
	} {
		if !strings.Contains(state.postMergeHook, want) {
			return fmt.Errorf("repository post-merge hook missing %q", want)
		}
	}
	return nil
}

func repositoryPostMergeHookShouldIncludeLifecycleSafeguard(ctx context.Context, safeguard string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	needles := postMergeSafeguardNeedles(safeguard)
	if len(needles) == 0 {
		return fmt.Errorf("unknown post-merge safeguard %q", safeguard)
	}
	for _, want := range needles {
		if !strings.Contains(state.postMergeHook, want) {
			return fmt.Errorf("repository post-merge hook missing safeguard %q marker %q", safeguard, want)
		}
	}
	return nil
}

func agmRunsDetachedArchiveCompanionStartupRegressions(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.companionOutput, state.companionRegression = runLocalGuardrailGoTest(ctx,
		`^(TestValidateRevision|TestAcknowledgeStartup.*|TestAwaitReaperStartup.*)$`,
		"./agm/cmd/agm", "./agm/cmd/agm-reaper",
	)
	return nil
}

func mixedRevisionOrMissingStartupAcknowledgementShouldFailBeforeAsyncSuccess(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.companionRegression != nil {
		return fmt.Errorf("detached archive companion regressions: %w: %s", state.companionRegression, state.companionOutput)
	}
	return nil
}

func agmRendersCanonicalAGMCompanionInstallPlan(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx,
		"make", "--no-print-directory", "-n", "install-agm",
		"GOFLAGS=", "GOENV=off", "GOWORK=off", "EXTRA_GO_LDFLAGS=",
		"VERSION=bdd-version", "GIT_COMMIT=bdd-commit", "BUILD_DATE=2026-08-03T00:00:00Z",
		"HOME=.bdd-install-plan",
	)
	cmd.Dir = hookBDDRepoRoot()
	cmd.Env = canonicalAGMInstallPlanEnvironment()
	output, runErr := cmd.CombinedOutput()
	state.companionOutput = string(output)
	state.companionRegression = runErr
	return nil
}

func rootAGMInstallPlanShouldBuildAndInstallCompanionPair(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.companionRegression != nil {
		return fmt.Errorf("render canonical AGM companion install plan: %w: %s", state.companionRegression, state.companionOutput)
	}
	return validateCanonicalAGMInstallPlan(state.companionOutput)
}

func validateCanonicalAGMInstallPlan(output string) error {
	stampFragments := []string{
		"-ldflags",
		"-X github.com/vbonnet/dear-agent/pkg/version.Version=${_BUILD_STAMP_VERSION}",
		"-X github.com/vbonnet/dear-agent/pkg/version.GitCommit=${_BUILD_STAMP_GIT_COMMIT}",
		"-X github.com/vbonnet/dear-agent/pkg/version.BuildDate=${_BUILD_STAMP_DATE}",
		"-X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=makefile",
	}
	checks := []struct {
		label     string
		marker    string
		fragments []string
	}{
		{label: "build-stamp guard", marker: "go run ./internal/buildstamp"},
		{label: "AGM stamped build", marker: "-o bin/agm ./agm/cmd/agm/", fragments: stampFragments},
		{label: "reaper stamped build", marker: "-o bin/agm-reaper ./agm/cmd/agm-reaper/", fragments: stampFragments},
		{
			label:  "AGM atomic install",
			marker: `dest="$dir/agm"`,
			fragments: []string{
				"cp 'bin/agm' \"$stage\"",
				"mv -f \"$stage\" \"$dest\"",
				"echo \"Installed: $dest\"",
			},
		},
		{
			label:  "reaper atomic install",
			marker: `dest="$dir/agm-reaper"`,
			fragments: []string{
				"cp 'bin/agm-reaper' \"$stage\"",
				"mv -f \"$stage\" \"$dest\"",
				"echo \"Installed: $dest\"",
			},
		},
	}
	for _, check := range checks {
		if err := requireInstallPlanLine(output, check.label, check.marker, check.fragments...); err != nil {
			return err
		}
	}
	return nil
}

func requireInstallPlanLine(output, label, marker string, fragments ...string) error {
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		for _, fragment := range fragments {
			if !strings.Contains(line, fragment) {
				return fmt.Errorf("canonical AGM companion install plan %s missing %q: %s", label, fragment, line)
			}
		}
		return nil
	}
	return fmt.Errorf("canonical AGM companion install plan missing %s marker %q: %s", label, marker, output)
}

func canonicalAGMInstallPlanEnvironment() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=.bdd-install-plan",
		"GOFLAGS=",
		"GOENV=off",
		"GOWORK=off",
		"MAKEFLAGS=",
		"MFLAGS=",
		"MAKEOVERRIDES=",
		"MAKEFILES=",
	}
}

func postMergeSafeguardNeedles(safeguard string) []string {
	switch safeguard {
	case "atomic-binary-install":
		return []string{"go build -o", "mv -f", "(atomic)"}
	case "trunk-build-context":
		return []string{"fetch_trunk_commit", "ensure_build_dir", "origin/${default_branch}"}
	case "agm-companion-coherence":
		return []string{"maybe_rebuild_agm_pair", ".agm-pair-install.lock", "run_install_lock", "installed pair unchanged", "_pair_ldflags", "agm/internal/"}
	case "wayfinder-runtime-deploy":
		return []string{"maybe_rebuild_wayfinder", ".wayfinder-install.lock", "run_install_lock", "lockf -t", "fetch_trunk_commit", "wayfinder/ pkg/ internal/ go.mod go.sum"}
	case "host-artifact-deploy":
		return []string{"deploy_host_artifacts", "make dear-deploy-sync"}
	case "deployment-verification":
		return []string{"verify_deployment_after_rebuild", "agm admin verify-deployment"}
	case "bead-transition":
		return []string{"transition_merged_beads", "bd --db", "close"}
	case "worktree-sweep":
		return []string{"sweep_merged_worktrees", "agm worktree sweep --execute"}
	case "fail-safe-exit":
		return []string{"NEVER blocks or fails the git operation", "exit 0"}
	default:
		return nil
	}
}

func hookManifestPath(harness string) (string, bool) {
	root := hookBDDRepoRoot()
	switch harness {
	case "claude-code":
		return filepath.Join(root, ".claude", "settings.json"), true
	case "codex-cli":
		return filepath.Join(root, ".codex", "hooks.json"), true
	case "agy":
		return "", false
	case "opencode-cli":
		return "", false
	case "pi-cli":
		return filepath.Join(root, ".pi", "hooks.json"), true
	default:
		return "", false
	}
}

func hookBDDRepoRoot() string {
	return packageSpecBDDRepoRoot()
}
