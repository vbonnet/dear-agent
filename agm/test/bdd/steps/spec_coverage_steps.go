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
	ctx.Step(`^every implementation directory including canonical Dockerfile and Makefile directories should have strict co-located SPEC and reciprocal BDD coverage$`, everyImplementationDirectoryShouldHaveStrictCoverage)
	ctx.Step(`^every repository SPEC should have strict EARS and reciprocal executable BDD coverage$`, everyImplementationDirectoryShouldHaveStrictCoverage)
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
	return specCoverageShouldHaveNoFindings(ctx,
		speccoverage.FindingKindMissingSpecPath,
		speccoverage.FindingKindSpecRead,
	)
}

func everyParitySurfaceShouldHaveExecutableBDDFeature(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx,
		speccoverage.FindingKindMissingFeaturePath,
		speccoverage.FindingKindFeatureRead,
		speccoverage.FindingKindFeatureMissingDeclaration,
	)
}

func everyParitySPECShouldDeclareEARSRequirements(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindMissingEARS)
}

func everyParitySPECShouldPassStrictEARSLint(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindInvalidEARS)
}

func everyParitySPECShouldReferenceExecutableBDDFeature(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindSpecMissingBDDRef)
}

func everyParityBDDFeatureShouldReferenceGoverningSPEC(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindFeatureMissingSpecRef)
}

func everyParitySPECShouldHaveCompletedAuditMarker(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindMissingAudit)
}

func everyParityBDDFeatureShouldBeRegistered(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindUnregisteredParityFeature)
}

func everyExecutableBDDFeatureShouldBeListedInBDDCatalog(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx,
		speccoverage.FindingKindCatalogRead,
		speccoverage.FindingKindFeatureDiscovery,
		speccoverage.FindingKindCatalogUnlistedFeature,
	)
}

func everyBDDCatalogFeatureReferenceShouldExist(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindCatalogMissingFeature)
}

func everyExecutableBDDFeatureShouldReferenceGoverningSPEC(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx,
		speccoverage.FindingKindFeatureDiscovery,
		speccoverage.FindingKindFeatureRead,
		speccoverage.FindingKindFeatureMissingSpecRef,
		speccoverage.FindingKindFeatureMissingSpec,
	)
}

func everyGoverningSPECShouldReferenceExecutableBDDFeature(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx,
		speccoverage.FindingKindFeatureMissingSpec,
		speccoverage.FindingKindSpecMissingFeatureRef,
	)
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
	return specCoverageShouldHaveNoFindings(ctx,
		speccoverage.FindingKindMissingCoLocatedSpec,
		speccoverage.FindingKindSpecRead,
	)
}

func changedProductionGoPackageSPECFilesShouldPassStrictEARSLint(ctx context.Context) error {
	return specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindInvalidEARS)
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
	repositorySpecFindings, err := speccoverage.ValidateAllRepositorySpecs(specCoverageRepoRoot())
	if err != nil {
		return err
	}
	state.findings = findings
	state.findings = append(state.findings, repositorySpecFindings...)
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

func specCoverageShouldHaveNoFindings(ctx context.Context, kinds ...speccoverage.FindingKind) error {
	state, err := getSpecCoverageState(ctx)
	if err != nil {
		return err
	}
	selected := make(map[speccoverage.FindingKind]struct{}, len(kinds))
	for _, kind := range kinds {
		selected[kind] = struct{}{}
	}
	var matches []string
	for _, finding := range state.findings {
		if _, ok := selected[finding.Kind]; ok {
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
