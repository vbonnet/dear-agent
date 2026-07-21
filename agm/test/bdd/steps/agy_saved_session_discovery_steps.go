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

type agySavedSessionDiscoveryStateKey struct{}

type agySavedSessionDiscoveryState struct {
	output string
	err    error
}

// RegisterAgySavedSessionDiscoverySteps registers production-backed AGY log
// discovery behavior steps.
func RegisterAgySavedSessionDiscoverySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, agySavedSessionDiscoveryStateKey{}, &agySavedSessionDiscoveryState{}), nil
	})
	ctx.Step(`^AGY saved-session metadata requires log fallback$`, agySavedSessionMetadataRequiresLogFallback)
	ctx.Step(`^AGM validates bounded AGY log discovery$`, agmValidatesBoundedAgyLogDiscovery)
	ctx.Step(`^AGY cache hits should bypass log discovery$`, agyCacheHitsShouldBypassLogDiscovery)
	ctx.Step(`^AGY log fallback should prefer the newest modification time$`, agyLogFallbackShouldPreferNewestModificationTime)
	ctx.Step(`^AGY log fallback should enforce its candidate-file budget$`, agyLogFallbackShouldEnforceCandidateFileBudget)
	ctx.Step(`^AGY log fallback should enforce its per-file byte budget$`, agyLogFallbackShouldEnforcePerFileByteBudget)
	ctx.Step(`^AGY oversized log lines should fail explicitly$`, agyOversizedLogLinesShouldFailExplicitly)
}

func agySavedSessionMetadataRequiresLogFallback() error {
	specPath := filepath.Join(packageSpecBDDRepoRoot(), "agm", "internal", "agysession", "SPEC.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("read AGY saved-session SPEC: %w", err)
	}
	if !strings.Contains(string(content), "**AGYS-04**") {
		return fmt.Errorf("AGY saved-session SPEC does not declare AGYS-04")
	}
	return nil
}

func agmValidatesBoundedAgyLogDiscovery(ctx context.Context) error {
	state, err := getAgySavedSessionDiscoveryState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/internal/agysession", "-run",
		`^(TestFindByID_CacheHitDoesNotReadInvalidLogDirectory|TestWorkspaceFromLogs(PrefersNewestModificationTime|ReportsCandidateBudgetExhaustion|ReportsPerFileByteBudgetExhaustion|RejectsOversizedLine|ReturnsMatchInsideTruncatedFile)|TestLatestConversationForWorkspaceRejects(TruncatedPrefixMatch|OlderMatchAfterTruncatedNewerLog)|TestLogHasUnreadTailDetectsGrowthAfterBoundedScan)$`,
		"-count=1", "-v")
	cmd.Dir = packageSpecBDDRepoRoot()
	output, runErr := cmd.CombinedOutput()
	state.output = string(output)
	state.err = runErr
	if testCtx.Err() != nil {
		return fmt.Errorf("bounded AGY discovery behavior suite timed out: %w", testCtx.Err())
	}
	return nil
}

func agyCacheHitsShouldBypassLogDiscovery(ctx context.Context) error {
	return requireAgySavedSessionBehavior(ctx, "TestFindByID_CacheHitDoesNotReadInvalidLogDirectory")
}

func agyLogFallbackShouldPreferNewestModificationTime(ctx context.Context) error {
	return requireAgySavedSessionBehavior(ctx, "TestWorkspaceFromLogsPrefersNewestModificationTime")
}

func agyLogFallbackShouldEnforceCandidateFileBudget(ctx context.Context) error {
	return requireAgySavedSessionBehavior(ctx, "TestWorkspaceFromLogsReportsCandidateBudgetExhaustion")
}

func agyLogFallbackShouldEnforcePerFileByteBudget(ctx context.Context) error {
	return requireAgySavedSessionBehavior(ctx,
		"TestWorkspaceFromLogsReportsPerFileByteBudgetExhaustion",
		"TestWorkspaceFromLogsReturnsMatchInsideTruncatedFile",
		"TestLatestConversationForWorkspaceRejectsTruncatedPrefixMatch",
		"TestLatestConversationForWorkspaceRejectsOlderMatchAfterTruncatedNewerLog",
		"TestLogHasUnreadTailDetectsGrowthAfterBoundedScan",
	)
}

func agyOversizedLogLinesShouldFailExplicitly(ctx context.Context) error {
	return requireAgySavedSessionBehavior(ctx, "TestWorkspaceFromLogsRejectsOversizedLine")
}

func requireAgySavedSessionBehavior(ctx context.Context, behaviors ...string) error {
	state, err := getAgySavedSessionDiscoveryState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("bounded AGY discovery behavior suite failed: %w\n%s", state.err, state.output)
	}
	for _, behavior := range behaviors {
		if !strings.Contains(state.output, "--- PASS: "+behavior) {
			return fmt.Errorf("bounded AGY discovery behavior %s did not pass:\n%s", behavior, state.output)
		}
	}
	return nil
}

func getAgySavedSessionDiscoveryState(ctx context.Context) (*agySavedSessionDiscoveryState, error) {
	state, ok := ctx.Value(agySavedSessionDiscoveryStateKey{}).(*agySavedSessionDiscoveryState)
	if !ok || state == nil {
		return nil, fmt.Errorf("AGY saved-session discovery state not initialized")
	}
	return state, nil
}
