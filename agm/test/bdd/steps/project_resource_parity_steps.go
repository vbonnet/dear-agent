package steps

import (
	"context"
	"fmt"
	"slices"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/projectresourceparity"
)

type projectResourceParityState struct {
	harness        string
	surface        projectresourceparity.Surface
	validateErr    error
	instructionErr error
}

type projectResourceParityStateKey struct{}

// RegisterProjectResourceParitySteps registers project resource governance steps.
func RegisterProjectResourceParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, projectResourceParityStateKey{}, &projectResourceParityState{}), nil
	})
	ctx.Step(`^project resource harness "([^"]*)" is active$`, projectResourceHarnessIsActive)
	ctx.Step(`^AGM validates project resource surface coverage$`, agmValidatesProjectResourceSurfaceCoverage)
	ctx.Step(`^harness "([^"]*)" should declare instruction "([^"]*)", skill "([^"]*)", configuration "([^"]*)", and hook "([^"]*)" dispositions$`, harnessShouldDeclareProjectResourceDispositions)
	ctx.Step(`^the repository Pi instruction surface$`, repositoryPiInstructionSurface)
	ctx.Step(`^AGM validates the Pi instruction projection$`, agmValidatesPiInstructionProjection)
	ctx.Step(`^the repository should expose root AGENTS\.md without a divergent Pi-only copy$`, repositoryShouldExposeSharedInstructions)
}

func projectResourceHarnessIsActive(ctx context.Context, harness string) error {
	state, err := getProjectResourceParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(agent.ActiveHarnesses(), harness) {
		return fmt.Errorf("harness %q is not active", harness)
	}
	state.harness = harness
	return nil
}

func agmValidatesProjectResourceSurfaceCoverage(ctx context.Context) error {
	state, err := getProjectResourceParityState(ctx)
	if err != nil {
		return err
	}
	state.validateErr = projectresourceparity.ValidateActiveHarnessSurfaces()
	if state.validateErr != nil {
		return state.validateErr
	}
	state.surface, _ = projectresourceparity.SurfaceForHarness(state.harness)
	return nil
}

func harnessShouldDeclareProjectResourceDispositions(ctx context.Context, harness, instructions, skills, configuration, hooks string) error {
	state, err := getProjectResourceParityState(ctx)
	if err != nil {
		return err
	}
	if state.validateErr != nil {
		return state.validateErr
	}
	if state.harness != harness || state.surface.Harness != harness {
		return fmt.Errorf("project resource surface = %+v, want harness %q", state.surface, harness)
	}
	got := []string{
		string(state.surface.Instructions.Disposition),
		string(state.surface.Skills.Disposition),
		string(state.surface.Configuration.Disposition),
		string(state.surface.Hooks.Disposition),
	}
	want := []string{instructions, skills, configuration, hooks}
	if !slices.Equal(got, want) {
		return fmt.Errorf("project resource dispositions for %q = %v, want %v", harness, got, want)
	}
	return nil
}

func repositoryPiInstructionSurface(ctx context.Context) error {
	state, err := getProjectResourceParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = "pi-cli"
	return nil
}

func agmValidatesPiInstructionProjection(ctx context.Context) error {
	state, err := getProjectResourceParityState(ctx)
	if err != nil {
		return err
	}
	state.instructionErr = projectresourceparity.ValidatePiInstructionSurface(packageSpecBDDRepoRoot())
	return nil
}

func repositoryShouldExposeSharedInstructions(ctx context.Context) error {
	state, err := getProjectResourceParityState(ctx)
	if err != nil {
		return err
	}
	return state.instructionErr
}

func getProjectResourceParityState(ctx context.Context) (*projectResourceParityState, error) {
	state, ok := ctx.Value(projectResourceParityStateKey{}).(*projectResourceParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("project resource parity state not initialized")
	}
	return state, nil
}
