package steps

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type agmSupervisionRecoveryGuardrailStateKey struct{}
type sentinelTmuxIsolationStateKey struct{}

type sentinelTmuxIsolationState struct {
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
		return context.WithValue(ctx, sentinelTmuxIsolationStateKey{}, &sentinelTmuxIsolationState{}), nil
	})
	ctx.Step(`^sentinel monitoring owns an explicit tmux socket$`, sentinelMonitoringOwnsAnExplicitTmuxSocket)
	ctx.Step(`^AGM validates sentinel tmux isolation$`, agmValidatesSentinelTmuxIsolation)
	ctx.Step(`^sentinel discovery should use only the configured socket$`, sentinelDiscoveryShouldUseOnlyTheConfiguredSocket)
	ctx.Step(`^nested AGM recovery commands should inherit the configured socket$`, nestedAGMRecoveryCommandsShouldInheritTheConfiguredSocket)
	ctx.Step(`^sentinel lifecycle tests should not inspect ambient tmux sessions$`, sentinelLifecycleTestsShouldNotInspectAmbientTmuxSessions)
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
