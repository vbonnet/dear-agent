//go:build !darwin && !linux

package steps

import (
	"context"
	"fmt"
	"runtime"

	"github.com/cucumber/godog"
)

// RegisterSpecGovernanceToolingSteps keeps the scenario surface explicit on
// unsupported platforms while failing closed before any child process starts.
func RegisterSpecGovernanceToolingSteps(ctx *godog.ScenarioContext) {
	for _, expression := range []string{
		specGovernancePinnedInventoryStep,
		specGovernanceNonVerdictLeadStep,
		specGovernanceReciprocalDiagnosticStep,
		specGovernanceFindingValidationStep,
		specGovernanceOfflineRenderingStep,
		specGovernanceReadOnlyBoundaryStep,
		specGovernanceResultStep,
	} {
		ctx.Step(expression, unsupportedSpecGovernanceToolingStep)
	}
}

func unsupportedSpecGovernanceToolingStep(context.Context) error {
	return fmt.Errorf("focused SPEC audit tooling is unsupported on %s/%s", runtime.GOOS, runtime.GOARCH)
}
