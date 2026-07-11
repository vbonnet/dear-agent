package steps

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

const wayfinderInternalFeaturePath = "agm/test/bdd/features/wayfinder_internal_package_guardrails.feature"

type wayfinderInternalPackageStateKey struct{}
type wayfinderInternalParityStateKey struct{}

type wayfinderInternalParityState struct {
	harness string
	family  string
}

// RegisterWayfinderInternalPackageGuardrailSteps registers Wayfinder package and parity steps.
func RegisterWayfinderInternalPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          wayfinderInternalPackageStateKey{},
		label:             "Wayfinder internal package",
		featurePath:       wayfinderInternalFeaturePath,
		configuredPattern: `^Wayfinder internal package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Wayfinder internal package coverage$`,
		colocatedPattern:  `^Wayfinder internal package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, wayfinderInternalParityStateKey{}, &wayfinderInternalParityState{}), nil
	})
	ctx.Step(`^canonical Wayfinder is driven by harness "([^"]*)" and model family "([^"]*)"$`, canonicalWayfinderIsDriven)
	ctx.Step(`^AGM validates the Wayfinder parity route$`, agmValidatesWayfinderParityRoute)
	ctx.Step(`^Wayfinder should preserve the same nine-phase contract$`, wayfinderPreservesNinePhaseContract)
}

func canonicalWayfinderIsDriven(ctx context.Context, harness, family string) error {
	state, err := getWayfinderInternalParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = harness
	state.family = family
	return nil
}

func agmValidatesWayfinderParityRoute(ctx context.Context) error {
	state, err := getWayfinderInternalParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(agent.ActiveHarnesses(), state.harness) {
		return fmt.Errorf("wayfinder harness %q is not active", state.harness)
	}
	if !agent.IsSupportedModelFamily(state.family) {
		return fmt.Errorf("wayfinder model family %q is not supported", state.family)
	}
	return nil
}

func wayfinderPreservesNinePhaseContract(ctx context.Context) error {
	if _, err := getWayfinderInternalParityState(ctx); err != nil {
		return err
	}
	path := filepath.Join(packageSpecBDDRepoRoot(), "wayfinder/cmd/wayfinder-session/internal/status/types_v2.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse canonical Wayfinder status source: %w", err)
	}
	want := []string{
		"WaypointV2Charter", "WaypointV2Problem", "WaypointV2Research",
		"WaypointV2Design", "WaypointV2Spec", "WaypointV2Plan",
		"WaypointV2Setup", "WaypointV2Build", "WaypointV2Retro",
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "AllWaypointsV2Schema" || fn.Body == nil {
			continue
		}
		var got []string
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			ret, ok := node.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, result := range ret.Results {
				literal, ok := result.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, element := range literal.Elts {
					if identifier, ok := element.(*ast.Ident); ok {
						got = append(got, identifier.Name)
					}
				}
			}
			return false
		})
		if !slices.Equal(got, want) {
			return fmt.Errorf("canonical Wayfinder phases = %v, want %v", got, want)
		}
		return nil
	}
	return fmt.Errorf("canonical Wayfinder phase enumerator not found")
}

func getWayfinderInternalParityState(ctx context.Context) (*wayfinderInternalParityState, error) {
	state, ok := ctx.Value(wayfinderInternalParityStateKey{}).(*wayfinderInternalParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("wayfinder internal parity state not initialized")
	}
	return state, nil
}
