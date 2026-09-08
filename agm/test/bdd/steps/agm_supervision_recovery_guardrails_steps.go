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

func agmValidatesNoCheckProviderCompleteness(ctx context.Context) error {
	state, err := getNoChecksProviderCompletenessState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "-v", "-count=1", "-timeout=90s",
		"-run", `^Test(RequiredCheckNamesForBranch|FetchRequiredChecks|CheckRunNamesForRef|RunPRScanNoChecksPolicyErrorStopsBeforeTrigger).*$`,
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

func policyFailuresShouldPreventTriggerCalls(ctx context.Context) error {
	if err := requireNoChecksProviderCompletenessBehavior(ctx); err != nil {
		return err
	}
	return requireNoChecksTestOutput(ctx, "TestRunPRScanNoChecksPolicyErrorStopsBeforeTrigger")
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
