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

const agmControlSurfaceFeaturePath = "agm/test/bdd/features/agm_control_surface_guardrails.feature"

type agmControlSurfaceGuardrailStateKey struct{}
type agmCobraIsolationStateKey struct{}

type agmCobraIsolationState struct {
	output string
	err    error
}

// RegisterAGMControlSurfaceGuardrailSteps registers BDD steps for AGM control-plane packages.
func RegisterAGMControlSurfaceGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmControlSurfaceGuardrailStateKey{},
		label:             "AGM control surface package",
		featurePath:       agmControlSurfaceFeaturePath,
		configuredPattern: `^AGM control surface package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates control surface package coverage$`,
		colocatedPattern:  `^AGM control surface package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, agmCobraIsolationStateKey{}, &agmCobraIsolationState{}), nil
	})
	ctx.Step(`^AGM Cobra commands with mutable execution state$`, agmCobraCommandsWithMutableExecutionState)
	ctx.Step(`^AGM audits Cobra command test isolation$`, agmAuditsCobraCommandTestIsolation)
	ctx.Step(`^mutable command flags should belong to fresh command instances$`, mutableCommandFlagsShouldBelongToFreshCommandInstances)
	ctx.Step(`^command validation tests should exercise repeatable execution orders$`, commandValidationTestsShouldExerciseRepeatableExecutionOrders)
}

func agmCobraCommandsWithMutableExecutionState() error {
	path := filepath.Join(packageSpecBDDRepoRoot(), "agm", "cmd", "agm")
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("locate AGM command package: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("AGM command package path %s is not a directory", path)
	}
	return nil
}

func agmAuditsCobraCommandTestIsolation(ctx context.Context) error {
	state, err := getAGMCobraIsolationState(ctx)
	if err != nil {
		return err
	}
	// Cold CI runners may compile the command package while the integration
	// graph is already saturating the host. Keep the behavioral gate bounded,
	// but allow enough time for that first build instead of turning contention
	// into a false contract failure.
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/cmd/agm", "-run", `^Test(CobraCommandValidationIsOrderIndependent|CobraCommandFactoriesIsolateFlagValues|RestoreCommandTreeFlagsForTestPreservesStringSliceParseState)$`, "-count=1", "-v")
	cmd.Dir = packageSpecBDDRepoRoot()
	output, runErr := cmd.CombinedOutput()
	state.output = string(output)
	state.err = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("cobra isolation behavior suite timed out: %w", testCtx.Err())
	}
	return nil
}

func mutableCommandFlagsShouldBelongToFreshCommandInstances(ctx context.Context) error {
	return requireAGMCobraIsolationBehavior(ctx)
}

func commandValidationTestsShouldExerciseRepeatableExecutionOrders(ctx context.Context) error {
	if err := requireAGMCobraIsolationBehavior(ctx); err != nil {
		return err
	}

	root := packageSpecBDDRepoRoot()
	spec, err := os.ReadFile(filepath.Join(root, "agm", "cmd", "agm", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("read AGM CLI SPEC: %w", err)
	}
	if !strings.Contains(string(spec), "**CLI-24** When AGM command tests execute Cobra commands or mutate command flags") {
		return fmt.Errorf("AGM CLI SPEC does not require command-state isolation")
	}
	return nil
}

func requireAGMCobraIsolationBehavior(ctx context.Context) error {
	state, err := getAGMCobraIsolationState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("cobra isolation behavior suite failed: %w\n%s", state.err, state.output)
	}
	for _, behavior := range []string{
		"TestCobraCommandValidationIsOrderIndependent",
		"TestCobraCommandFactoriesIsolateFlagValues",
		"TestRestoreCommandTreeFlagsForTestPreservesStringSliceParseState",
	} {
		if !strings.Contains(state.output, "--- PASS: "+behavior) {
			return fmt.Errorf("cobra isolation behavior %s did not pass:\n%s", behavior, state.output)
		}
	}
	return nil
}

func getAGMCobraIsolationState(ctx context.Context) (*agmCobraIsolationState, error) {
	state, ok := ctx.Value(agmCobraIsolationStateKey{}).(*agmCobraIsolationState)
	if !ok || state == nil {
		return nil, fmt.Errorf("cobra isolation behavior state not initialized")
	}
	return state, nil
}
