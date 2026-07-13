package steps

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/cucumber/godog"
)

type bddRootPortabilityState struct {
	start string
	root  string
	found bool
}

type bddRootPortabilityStateKey struct{}

// RegisterBDDRootPortabilitySteps registers checkout discovery guardrails.
func RegisterBDDRootPortabilitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, bddRootPortabilityStateKey{}, &bddRootPortabilityState{}), nil
	})
	ctx.Step(`^BDD tests execute from a nested package directory$`, bddTestsExecuteFromNestedPackageDirectory)
	ctx.Step(`^AGM resolves the BDD repository root$`, agmResolvesBDDRepositoryRoot)
	ctx.Step(`^the resolver should find the checkout without compiler source paths$`, resolverShouldFindCheckoutWithoutCompilerSourcePaths)
}

func bddTestsExecuteFromNestedPackageDirectory(ctx context.Context) error {
	state, err := getBDDRootPortabilityState(ctx)
	if err != nil {
		return err
	}
	state.start = filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "bdd", "steps")
	return nil
}

func agmResolvesBDDRepositoryRoot(ctx context.Context) error {
	state, err := getBDDRootPortabilityState(ctx)
	if err != nil {
		return err
	}
	state.root, state.found = findBDDRepoRoot(state.start)
	return nil
}

func resolverShouldFindCheckoutWithoutCompilerSourcePaths(ctx context.Context) error {
	state, err := getBDDRootPortabilityState(ctx)
	if err != nil {
		return err
	}
	if !state.found {
		return fmt.Errorf("BDD repository root not found from %s", state.start)
	}
	if state.root != packageSpecBDDRepoRoot() {
		return fmt.Errorf("BDD repository root = %q, want %q", state.root, packageSpecBDDRepoRoot())
	}
	return nil
}

func getBDDRootPortabilityState(ctx context.Context) (*bddRootPortabilityState, error) {
	state, ok := ctx.Value(bddRootPortabilityStateKey{}).(*bddRootPortabilityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("BDD root portability state not initialized")
	}
	return state, nil
}
