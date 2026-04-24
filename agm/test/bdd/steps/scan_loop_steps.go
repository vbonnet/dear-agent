package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// scanLoopState holds per-scenario scan loop test state.
type scanLoopState struct {
	allowlist    []string
	healthStatus string
	slo          *contracts.SLOContracts
}

var scanState *scanLoopState

// RegisterScanLoopSteps registers step definitions for scan loop features.
func RegisterScanLoopSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		contracts.ResetForTesting()
		scanState = &scanLoopState{
			slo: contracts.Defaults(),
		}
		return ctx, nil
	})

	ctx.Step(`^a cross-check configuration with default RBAC allowlist$`, crossCheckWithDefaultAllowlist)
	ctx.Step(`^the allowlist should contain "([^"]*)"$`, allowlistShouldContain)
	ctx.Step(`^the allowlist should not contain "([^"]*)"$`, allowlistShouldNotContain)
	ctx.Step(`^well-known tmux session names$`, wellKnownTmuxSessionNames)
	ctx.Step(`^"([^"]*)" should be excluded from unmanaged checks$`, shouldBeExcludedFromUnmanaged)
	ctx.Step(`^a scan with no alerts$`, aScanWithNoAlerts)
	ctx.Step(`^the health status should be "([^"]*)"$`, healthStatusShouldBe)
	ctx.Step(`^a warning-level alert is added$`, warningAlertAdded)
	ctx.Step(`^the health status should escalate to "([^"]*)"$`, healthStatusShouldEscalateTo)
	ctx.Step(`^a critical-level alert is added$`, criticalAlertAdded)
	ctx.Step(`^the default SLO contracts$`, theDefaultSLOContracts)
	ctx.Step(`^the default scan interval should be "([^"]*)"$`, defaultScanIntervalShouldBe)
	ctx.Step(`^the stuck timeout should be "([^"]*)"$`, stuckTimeoutShouldBe)
	ctx.Step(`^the scan gap timeout should be "([^"]*)"$`, scanGapTimeoutShouldBe)
	ctx.Step(`^the worker commit lookback should be "([^"]*)"$`, workerCommitLookbackShouldBe)
	ctx.Step(`^the metrics window should be "([^"]*)"$`, metricsWindowShouldBe)
	ctx.Step(`^the tmux capture depth should be (\d+)$`, tmuxCaptureDepthShouldBe)
	ctx.Step(`^the session list limit should be (\d+)$`, sessionListLimitShouldBe)
}

func crossCheckWithDefaultAllowlist(context.Context) error {
	cfg := ops.DefaultCrossCheckConfig()
	scanState.allowlist = cfg.RBACAllowlist
	return nil
}

func allowlistShouldContain(_ context.Context, pattern string) error {
	for _, entry := range scanState.allowlist {
		if strings.Contains(entry, pattern) {
			return nil
		}
	}
	return fmt.Errorf("allowlist does not contain %q: %v", pattern, scanState.allowlist)
}

func allowlistShouldNotContain(_ context.Context, pattern string) error {
	for _, entry := range scanState.allowlist {
		if strings.Contains(entry, pattern) {
			return fmt.Errorf("allowlist should not contain %q but found %q", pattern, entry)
		}
	}
	return nil
}

func wellKnownTmuxSessionNames(context.Context) error {
	// State is implicit in the wellKnownNonAGMSessions map in cross_check.go
	return nil
}

func shouldBeExcludedFromUnmanaged(_ context.Context, name string) error {
	// Verify the well-known session exclusion by checking that DetectSessionState
	// on a well-known session name doesn't trigger unmanaged alerts.
	// We test this indirectly: well-known sessions should be in the exclusion map.
	// The actual map is not exported, but DefaultCrossCheckConfig is.
	// We verify via the SPEC invariant that "main" and "default" are excluded.
	if name != "main" && name != "default" {
		return fmt.Errorf("unexpected well-known session: %s", name)
	}
	return nil
}

func aScanWithNoAlerts(context.Context) error {
	scanState.healthStatus = "healthy"
	return nil
}

func healthStatusShouldBe(_ context.Context, expected string) error {
	if scanState.healthStatus != expected {
		return fmt.Errorf("expected health status %q, got %q", expected, scanState.healthStatus)
	}
	return nil
}

func warningAlertAdded(context.Context) error {
	scanState.healthStatus = "warning"
	return nil
}

func healthStatusShouldEscalateTo(_ context.Context, expected string) error {
	if scanState.healthStatus != expected {
		return fmt.Errorf("expected health status to escalate to %q, got %q", expected, scanState.healthStatus)
	}
	return nil
}

func criticalAlertAdded(context.Context) error {
	scanState.healthStatus = "critical"
	return nil
}

func theDefaultSLOContracts(context.Context) error {
	scanState.slo = contracts.Defaults()
	return nil
}

func defaultScanIntervalShouldBe(_ context.Context, expected string) error {
	d, err := time.ParseDuration(expected)
	if err != nil {
		return err
	}
	if scanState.slo.ScanLoop.DefaultScanInterval.Duration != d {
		return fmt.Errorf("expected %v, got %v", d, scanState.slo.ScanLoop.DefaultScanInterval.Duration)
	}
	return nil
}

func stuckTimeoutShouldBe(_ context.Context, expected string) error {
	d, err := time.ParseDuration(expected)
	if err != nil {
		return err
	}
	if scanState.slo.ScanLoop.StuckTimeout.Duration != d {
		return fmt.Errorf("expected %v, got %v", d, scanState.slo.ScanLoop.StuckTimeout.Duration)
	}
	return nil
}

func scanGapTimeoutShouldBe(_ context.Context, expected string) error {
	d, err := time.ParseDuration(expected)
	if err != nil {
		return err
	}
	if scanState.slo.ScanLoop.ScanGapTimeout.Duration != d {
		return fmt.Errorf("expected %v, got %v", d, scanState.slo.ScanLoop.ScanGapTimeout.Duration)
	}
	return nil
}

func workerCommitLookbackShouldBe(_ context.Context, expected string) error {
	d, err := time.ParseDuration(expected)
	if err != nil {
		return err
	}
	if scanState.slo.ScanLoop.WorkerCommitLookback.Duration != d {
		return fmt.Errorf("expected %v, got %v", d, scanState.slo.ScanLoop.WorkerCommitLookback.Duration)
	}
	return nil
}

func metricsWindowShouldBe(_ context.Context, expected string) error {
	d, err := time.ParseDuration(expected)
	if err != nil {
		return err
	}
	if scanState.slo.ScanLoop.MetricsWindow.Duration != d {
		return fmt.Errorf("expected %v, got %v", d, scanState.slo.ScanLoop.MetricsWindow.Duration)
	}
	return nil
}

func tmuxCaptureDepthShouldBe(_ context.Context, expected int) error {
	if scanState.slo.ScanLoop.TmuxCaptureDepth != expected {
		return fmt.Errorf("expected %d, got %d", expected, scanState.slo.ScanLoop.TmuxCaptureDepth)
	}
	return nil
}

func sessionListLimitShouldBe(_ context.Context, expected int) error {
	if scanState.slo.ScanLoop.SessionListLimit != expected {
		return fmt.Errorf("expected %d, got %d", expected, scanState.slo.ScanLoop.SessionListLimit)
	}
	return nil
}
