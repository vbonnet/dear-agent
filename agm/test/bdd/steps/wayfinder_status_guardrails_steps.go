package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
)

type wayfinderStatusGuardrailState struct {
	pkg      string
	spec     string
	repoRoot string
}

type wayfinderStatusGuardrailStateKey struct{}

// RegisterWayfinderStatusGuardrailSteps registers BDD steps for Wayfinder status packages.
func RegisterWayfinderStatusGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, wayfinderStatusGuardrailStateKey{}, &wayfinderStatusGuardrailState{
			repoRoot: wayfinderStatusBDDRepoRoot(),
		}), nil
	})

	ctx.Step(`^Wayfinder core package "([^"]*)" is configured$`, wayfinderCorePackageIsConfigured)
	ctx.Step(`^AGM validates Wayfinder core package coverage$`, agmValidatesWayfinderCorePackageCoverage)
	ctx.Step(`^Wayfinder core package "([^"]*)" should have a co-located SPEC$`, wayfinderCorePackageShouldHaveCoLocatedSPEC)
}

func wayfinderCorePackageIsConfigured(ctx context.Context, pkg string) error {
	state, err := getWayfinderStatusGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.pkg = pkg
	state.spec = filepath.Join(state.repoRoot, filepath.FromSlash(pkg), "SPEC.md")
	return nil
}

func agmValidatesWayfinderCorePackageCoverage(ctx context.Context) error {
	state, err := getWayfinderStatusGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.spec == "" {
		return fmt.Errorf("no Wayfinder core package configured")
	}
	if _, err := os.Stat(state.spec); err != nil {
		return fmt.Errorf("wayfinder core package SPEC %s: %w", state.spec, err)
	}
	return nil
}

func wayfinderCorePackageShouldHaveCoLocatedSPEC(ctx context.Context, pkg string) error {
	state, err := getWayfinderStatusGuardrailState(ctx)
	if err != nil {
		return err
	}
	if pkg != state.pkg {
		return fmt.Errorf("configured Wayfinder core package = %q, want %q", state.pkg, pkg)
	}
	wantSuffix := filepath.Join(filepath.FromSlash(pkg), "SPEC.md")
	if !strings.HasSuffix(state.spec, wantSuffix) {
		return fmt.Errorf("wayfinder core package SPEC = %q, want suffix %q", state.spec, wantSuffix)
	}
	return nil
}

func getWayfinderStatusGuardrailState(ctx context.Context) (*wayfinderStatusGuardrailState, error) {
	state, ok := ctx.Value(wayfinderStatusGuardrailStateKey{}).(*wayfinderStatusGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("wayfinder status guardrail state not initialized")
	}
	return state, nil
}

func wayfinderStatusBDDRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
