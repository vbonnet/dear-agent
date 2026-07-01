package steps

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/internal/speccoverage"
)

type specCoverageState struct {
	findings []speccoverage.Finding
}

type specCoverageStateKey struct{}

// RegisterSpecCoverageSteps registers BDD steps for SPEC/BDD coverage parity.
func RegisterSpecCoverageSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, specCoverageStateKey{}, &specCoverageState{}), nil
	})

	ctx.Step(`^AGM parity coverage requirements$`, agmParityCoverageRequirements)
	ctx.Step(`^AGM validates parity SPEC and BDD coverage$`, agmValidatesParitySpecAndBDDCoverage)
	ctx.Step(`^every parity surface should have a SPEC.md$`, everyParitySurfaceShouldHaveSPEC)
	ctx.Step(`^every parity surface should have an executable BDD feature$`, everyParitySurfaceShouldHaveExecutableBDDFeature)
	ctx.Step(`^every parity SPEC should declare EARS requirements$`, everyParitySPECShouldDeclareEARSRequirements)
	ctx.Step(`^every parity BDD feature should be registered in the coverage matrix$`, everyParityBDDFeatureShouldBeRegistered)
	ctx.Step(`^AGM validates changed Go package SPEC coverage$`, agmValidatesChangedGoPackageSPECCoverage)
	ctx.Step(`^changed production Go packages should have co-located SPEC.md files$`, changedProductionGoPackagesShouldHaveCoLocatedSPECFiles)
}

func agmParityCoverageRequirements(ctx context.Context) error {
	if len(speccoverage.ParitySurfaces()) == 0 {
		return fmt.Errorf("parity coverage matrix is empty")
	}
	return nil
}

func agmValidatesParitySpecAndBDDCoverage(ctx context.Context) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	findings, err := speccoverage.Validate(specCoverageRepoRoot())
	if err != nil {
		return err
	}
	state.findings = findings
	return nil
}

func everyParitySurfaceShouldHaveSPEC(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "SPEC.md")
}

func everyParitySurfaceShouldHaveExecutableBDDFeature(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "BDD feature")
}

func everyParitySPECShouldDeclareEARSRequirements(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "EARS requirements")
}

func everyParityBDDFeatureShouldBeRegistered(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "not registered")
}

func agmValidatesChangedGoPackageSPECCoverage(ctx context.Context) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	findings, err := speccoverage.ValidateChangedGoPackageSpecs(specCoverageRepoRoot(), "origin/main")
	if err != nil {
		return err
	}
	state.findings = findings
	return nil
}

func changedProductionGoPackagesShouldHaveCoLocatedSPECFiles(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "co-located SPEC.md")
}

func specCoverageShouldHaveNoFindings(ctx context.Context, phrase string) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	var matches []string
	for _, finding := range state.findings {
		if strings.Contains(finding.Message, phrase) || strings.Contains(finding.Path, phrase) {
			matches = append(matches, finding.Error())
		}
	}
	if len(matches) > 0 {
		return fmt.Errorf("%s", strings.Join(matches, "\n"))
	}
	return nil
}

func getSpecCoverageState(ctx context.Context) (*specCoverageState, error) {
	state, ok := ctx.Value(specCoverageStateKey{}).(*specCoverageState)
	if !ok || state == nil {
		return nil, fmt.Errorf("spec coverage state not initialized")
	}
	return state, nil
}

func specCoverageRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
