package steps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

// trustTestState holds per-scenario trust test state.
type trustTestState struct {
	trustDir    string
	environment map[string]trustEnvironmentValue
	lastErr     error
	lastScore   int
}

type trustEnvironmentValue struct {
	value string
	set   bool
}

type trustTestStateKey struct{}

const trustProtocolFeatureName = "trust_protocol.feature"

const trustGoEnvTimeout = 10 * time.Second

// RegisterTrustProtocolSteps registers step definitions for trust protocol features.
func RegisterTrustProtocolSteps(ctx *godog.ScenarioContext) {
	goCache, goModCache, cacheErr := resolveTrustGoCaches()
	ctx.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		if !isTrustProtocolScenario(scenario) {
			return ctx, nil
		}
		if cacheErr != nil {
			return ctx, cacheErr
		}
		dir, err := os.MkdirTemp("", "bdd-trust-*")
		if err != nil {
			return ctx, err
		}
		state := &trustTestState{
			trustDir:    dir,
			environment: snapshotTrustEnvironment(),
		}
		if err := configureTrustEnvironment(dir, goCache, goModCache); err != nil {
			return ctx, errors.Join(err, restoreTrustEnvironment(state.environment), removeTrustDir(dir))
		}
		contracts.ResetForTesting()
		return context.WithValue(ctx, trustTestStateKey{}, state), nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		state, ok := ctx.Value(trustTestStateKey{}).(*trustTestState)
		if !ok || state == nil {
			return ctx, scenarioErr
		}
		return ctx, errors.Join(
			scenarioErr,
			restoreTrustEnvironment(state.environment),
			removeTrustDir(state.trustDir),
		)
	})

	ctx.Step(`^a session "([^"]*)" with no trust history$`, aSessionWithNoTrustHistory)
	ctx.Step(`^I record (\d+) "([^"]*)" events for "([^"]*)"$`, iRecordNEventsFor)
	ctx.Step(`^the trust score for "([^"]*)" should be (\d+)$`, trustScoreShouldBe)
	ctx.Step(`^the score should never be negative$`, scoreShouldNeverBeNegative)
	ctx.Step(`^I record a "([^"]*)" event for "([^"]*)"$`, iRecordAnEventFor)
	ctx.Step(`^the trust history for "([^"]*)" should have (\d+) events$`, trustHistoryShouldHaveNEvents)
	ctx.Step(`^the events should be in chronological order$`, eventsShouldBeChronological)
	ctx.Step(`^I attempt to record a trust event with empty session name$`, iAttemptRecordWithEmptySessionName)
	ctx.Step(`^an invalid input error should be returned$`, anInvalidInputErrorShouldBeReturned)
	ctx.Step(`^I attempt to record a trust event with type "([^"]*)" for session "([^"]*)"$`, iAttemptRecordWithInvalidType)
	ctx.Step(`^the error should list valid event types$`, errorShouldListValidTypes)
}

func isTrustProtocolScenario(scenario *godog.Scenario) bool {
	return scenario != nil && filepath.Base(scenario.Uri) == trustProtocolFeatureName
}

func aSessionWithNoTrustHistory(ctx context.Context, _ string) error {
	_, err := getTrustTestState(ctx)
	return err
}

func iRecordNEventsFor(_ context.Context, count int, eventType, session string) error {
	for i := 0; i < count; i++ {
		_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: session,
			EventType:   eventType,
			Detail:      fmt.Sprintf("event %d", i),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func trustScoreShouldBe(ctx context.Context, session string, expected int) error {
	result, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: session})
	if err != nil {
		return err
	}
	state, err := getTrustTestState(ctx)
	if err != nil {
		return err
	}
	state.lastScore = result.Score
	if result.Score != expected {
		return fmt.Errorf("expected trust score %d, got %d", expected, result.Score)
	}
	return nil
}

func scoreShouldNeverBeNegative(ctx context.Context) error {
	state, err := getTrustTestState(ctx)
	if err != nil {
		return err
	}
	if state.lastScore < 0 {
		return fmt.Errorf("trust score is negative: %d", state.lastScore)
	}
	return nil
}

func iRecordAnEventFor(_ context.Context, eventType, session string) error {
	_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
		SessionName: session,
		EventType:   eventType,
	})
	return err
}

func trustHistoryShouldHaveNEvents(_ context.Context, session string, expected int) error {
	result, err := ops.TrustHistory(nil, &ops.TrustHistoryRequest{SessionName: session})
	if err != nil {
		return err
	}
	if result.Total != expected {
		return fmt.Errorf("expected %d events, got %d", expected, result.Total)
	}
	return nil
}

func eventsShouldBeChronological(ctx context.Context) error {
	// Events are append-only with time.Now() so they are inherently ordered
	return nil
}

func iAttemptRecordWithEmptySessionName(ctx context.Context) error {
	state, stateErr := getTrustTestState(ctx)
	if stateErr != nil {
		return stateErr
	}
	_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
		SessionName: "",
		EventType:   "success",
	})
	state.lastErr = err
	return nil
}

func anInvalidInputErrorShouldBeReturned(ctx context.Context) error {
	state, err := getTrustTestState(ctx)
	if err != nil {
		return err
	}
	if state.lastErr == nil {
		return fmt.Errorf("expected an error but got nil")
	}
	return nil
}

func iAttemptRecordWithInvalidType(ctx context.Context, eventType, session string) error {
	state, stateErr := getTrustTestState(ctx)
	if stateErr != nil {
		return stateErr
	}
	_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
		SessionName: session,
		EventType:   eventType,
	})
	state.lastErr = err
	return nil
}

func errorShouldListValidTypes(ctx context.Context) error {
	state, err := getTrustTestState(ctx)
	if err != nil {
		return err
	}
	if state.lastErr == nil {
		return fmt.Errorf("expected error to list valid types")
	}
	msg := state.lastErr.Error()
	for _, t := range ops.ValidTrustEventTypes() {
		if !strings.Contains(msg, string(t)) {
			return fmt.Errorf("error message should list %q but got: %s", t, msg)
		}
	}
	return nil
}

func getTrustTestState(ctx context.Context) (*trustTestState, error) {
	state, ok := ctx.Value(trustTestStateKey{}).(*trustTestState)
	if !ok || state == nil {
		return nil, errors.New("trust protocol scenario state is not initialized")
	}
	return state, nil
}

func resolveTrustGoCaches() (string, string, error) {
	goCache, goCacheSet := os.LookupEnv("GOCACHE")
	goModCache, goModCacheSet := os.LookupEnv("GOMODCACHE")
	if goCacheSet && goCache != "" && goModCacheSet && goModCache != "" {
		return goCache, goModCache, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), trustGoEnvTimeout)
	defer cancel()
	output, err := newTrustGoEnvCommand(ctx).Output()
	if err != nil {
		return "", "", fmt.Errorf("resolve shared Go caches for trust BDD: %w", err)
	}
	lines := parseTrustGoEnv(output)
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", "", fmt.Errorf("resolve shared Go caches for trust BDD: unexpected go env output %q", output)
	}
	if !goCacheSet || goCache == "" {
		goCache = lines[0]
	}
	if !goModCacheSet || goModCache == "" {
		goModCache = lines[1]
	}
	return goCache, goModCache, nil
}

func newTrustGoEnvCommand(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", "env", "GOCACHE", "GOMODCACHE")
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd
}

func parseTrustGoEnv(output []byte) []string {
	normalized := strings.ReplaceAll(string(output), "\r", "")
	return slices.Collect(strings.SplitSeq(strings.TrimSpace(normalized), "\n"))
}

func snapshotTrustEnvironment() map[string]trustEnvironmentValue {
	snapshot := make(map[string]trustEnvironmentValue, 3)
	for _, name := range []string{"HOME", "GOCACHE", "GOMODCACHE"} {
		value, set := os.LookupEnv(name)
		snapshot[name] = trustEnvironmentValue{value: value, set: set}
	}
	return snapshot
}

func configureTrustEnvironment(home, goCache, goModCache string) error {
	for _, variable := range []struct {
		name  string
		value string
	}{
		{name: "HOME", value: home},
		{name: "GOCACHE", value: goCache},
		{name: "GOMODCACHE", value: goModCache},
	} {
		if err := os.Setenv(variable.name, variable.value); err != nil {
			return fmt.Errorf("set trust BDD environment %s: %w", variable.name, err)
		}
	}
	return nil
}

func restoreTrustEnvironment(snapshot map[string]trustEnvironmentValue) error {
	var restoreErr error
	for _, name := range []string{"HOME", "GOCACHE", "GOMODCACHE"} {
		previous := snapshot[name]
		if previous.set {
			restoreErr = errors.Join(restoreErr, os.Setenv(name, previous.value))
		} else {
			restoreErr = errors.Join(restoreErr, os.Unsetenv(name))
		}
	}
	if restoreErr != nil {
		return fmt.Errorf("restore trust BDD environment: %w", restoreErr)
	}
	return nil
}

func removeTrustDir(path string) error {
	if !strings.HasPrefix(filepath.Base(path), "bdd-trust-") {
		return fmt.Errorf("refuse to remove non-trust BDD directory %q", path)
	}
	if err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			// #nosec G302,G122 -- the exact owned tree needs owner traversal for cleanup.
			return os.Chmod(current, 0o700)
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("make trust BDD directory removable: %w", err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove trust BDD directory: %w", err)
	}
	return nil
}
