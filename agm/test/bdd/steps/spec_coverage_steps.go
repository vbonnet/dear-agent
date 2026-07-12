package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	ctx.Step(`^every parity SPEC should pass strict EARS lint$`, everyParitySPECShouldPassStrictEARSLint)
	ctx.Step(`^every parity SPEC should reference its executable BDD feature$`, everyParitySPECShouldReferenceExecutableBDDFeature)
	ctx.Step(`^every parity BDD feature should reference its governing SPEC\.md$`, everyParityBDDFeatureShouldReferenceGoverningSPEC)
	ctx.Step(`^every parity SPEC should have a completed audit marker$`, everyParitySPECShouldHaveCompletedAuditMarker)
	ctx.Step(`^every parity BDD feature should be registered in the coverage matrix$`, everyParityBDDFeatureShouldBeRegistered)
	ctx.Step(`^every executable BDD feature should be listed in the BDD catalog$`, everyExecutableBDDFeatureShouldBeListedInBDDCatalog)
	ctx.Step(`^every BDD catalog feature reference should exist$`, everyBDDCatalogFeatureReferenceShouldExist)
	ctx.Step(`^every executable BDD feature should reference a governing SPEC\.md$`, everyExecutableBDDFeatureShouldReferenceGoverningSPEC)
	ctx.Step(`^every governing SPEC\.md should reference its executable BDD feature$`, everyGoverningSPECShouldReferenceExecutableBDDFeature)
	ctx.Step(`^AGM validates changed Go package SPEC coverage$`, agmValidatesChangedGoPackageSPECCoverage)
	ctx.Step(`^changed production Go packages should have co-located SPEC.md files$`, changedProductionGoPackagesShouldHaveCoLocatedSPECFiles)
	ctx.Step(`^changed production Go package SPEC.md files should pass strict EARS lint$`, changedProductionGoPackageSPECFilesShouldPassStrictEARSLint)
	ctx.Step(`^AGM validates repository-wide implementation SPEC and BDD coverage$`, agmValidatesRepositoryImplementationCoverage)
	ctx.Step(`^every implementation directory should have strict co-located SPEC and reciprocal BDD coverage$`, everyImplementationDirectoryShouldHaveStrictCoverage)
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

func everyParitySPECShouldPassStrictEARSLint(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "invalid EARS syntax")
}

func everyParitySPECShouldReferenceExecutableBDDFeature(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "does not reference its executable BDD feature")
}

func everyParityBDDFeatureShouldReferenceGoverningSPEC(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "does not reference its governing SPEC.md")
}

func everyParitySPECShouldHaveCompletedAuditMarker(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "audit marker")
}

func everyParityBDDFeatureShouldBeRegistered(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "not registered")
}

func everyExecutableBDDFeatureShouldBeListedInBDDCatalog(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "not listed in agm/docs/BDD-CATALOG.md")
}

func everyBDDCatalogFeatureReferenceShouldExist(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "references a missing feature file")
}

func everyExecutableBDDFeatureShouldReferenceGoverningSPEC(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "does not declare governing SPEC.md")
}

func everyGoverningSPECShouldReferenceExecutableBDDFeature(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "governing SPEC.md does not reference executable BDD feature")
}

func agmValidatesChangedGoPackageSPECCoverage(ctx context.Context) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	diffCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	findings, err := speccoverage.ValidateChangedGoPackageSpecs(diffCtx, specCoverageRepoRoot(), "origin/main")
	if err != nil {
		return err
	}
	state.findings = findings
	return nil
}

func changedProductionGoPackagesShouldHaveCoLocatedSPECFiles(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "co-located SPEC.md")
}

func changedProductionGoPackageSPECFilesShouldPassStrictEARSLint(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, "invalid EARS syntax")
}

func agmValidatesRepositoryImplementationCoverage(ctx context.Context) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	findings, err := speccoverage.ValidateAllImplementationSpecs(specCoverageRepoRoot())
	if err != nil {
		return err
	}
	state.findings = findings
	return nil
}

func everyImplementationDirectoryShouldHaveStrictCoverage(ctx context.Context) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	if len(state.findings) == 0 {
		return nil
	}
	messages := make([]string, 0, len(state.findings))
	for _, finding := range state.findings {
		messages = append(messages, finding.Error())
	}
	return fmt.Errorf("%s", strings.Join(messages, "\n"))
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
	return packageSpecBDDRepoRoot()
}
