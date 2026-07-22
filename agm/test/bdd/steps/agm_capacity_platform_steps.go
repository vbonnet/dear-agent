package steps

import (
	"context"
	"fmt"
	"runtime"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/capacity"
)

type agmCapacityPlatformState struct {
	info capacity.SystemInfo
}

type agmCapacityPlatformStateKey struct{}

// RegisterAGMCapacityPlatformSteps registers native capacity detector coverage.
func RegisterAGMCapacityPlatformSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, agmCapacityPlatformStateKey{}, &agmCapacityPlatformState{}), nil
	})
	ctx.Step(`^AGM is running on a supported capacity platform$`, agmIsRunningOnSupportedCapacityPlatform)
	ctx.Step(`^AGM detects the host capacity resources$`, agmDetectsHostCapacityResources)
	ctx.Step(`^the capacity detector should report bounded memory$`, capacityDetectorShouldReportBoundedMemory)
}

func agmIsRunningOnSupportedCapacityPlatform() error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("capacity detector does not support %s", runtime.GOOS)
	}
	return nil
}

func agmDetectsHostCapacityResources(ctx context.Context) error {
	state, err := getAGMCapacityPlatformState(ctx)
	if err != nil {
		return err
	}
	state.info, err = capacity.NewDetector().Detect()
	return err
}

func capacityDetectorShouldReportBoundedMemory(ctx context.Context) error {
	state, err := getAGMCapacityPlatformState(ctx)
	if err != nil {
		return err
	}
	if state.info.TotalRAMBytes == 0 {
		return fmt.Errorf("total RAM must be positive")
	}
	if state.info.AvailableRAMBytes > state.info.TotalRAMBytes {
		return fmt.Errorf("available RAM %d exceeds total RAM %d", state.info.AvailableRAMBytes, state.info.TotalRAMBytes)
	}
	return nil
}

func getAGMCapacityPlatformState(ctx context.Context) (*agmCapacityPlatformState, error) {
	state, ok := ctx.Value(agmCapacityPlatformStateKey{}).(*agmCapacityPlatformState)
	if !ok || state == nil {
		return nil, fmt.Errorf("AGM capacity platform state not initialized")
	}
	return state, nil
}
