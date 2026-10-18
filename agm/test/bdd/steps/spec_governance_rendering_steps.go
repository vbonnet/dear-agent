//go:build darwin || linux

package steps

import (
	"context"

	"github.com/cucumber/godog"
)

// registerSpecGovernanceRenderingSteps keeps the focused supplemental checks
// out of the general spec-governance step module.
func registerSpecGovernanceRenderingSteps(ctx *godog.ScenarioContext) {
	ctx.Step(specGovernanceOfflineRenderingStep, exerciseBoundedOfflineSPECAuditRendering)
	ctx.Step(specGovernanceFindingFilterStep, exerciseSPECAuditFindingCardFiltering)
	ctx.Step(specGovernanceReadOnlyBoundaryStep, exerciseReadOnlySPECAuditBoundary)
	ctx.Step(specGovernancePortablePackageStep, exercisePortableSpecGovernancePackage)
	ctx.Step(specGovernanceOverlappingPackageStep, exerciseOverlappingSpecGovernancePackage)
	ctx.Step(specGovernancePortableCommandStep, exercisePortableSpecGovernanceCommandBoundary)
}

func exerciseBoundedOfflineSPECAuditRendering(ctx context.Context) error {
	return runSpecAuditGoTests(ctx,
		"TestRenderIsOfflineAndEscapesEvidence",
		"TestReportInputsAndArtifactsAreBounded",
		"TestEscapedHTMLStopsAtArtifactLimit",
	)
}

func exerciseSPECAuditFindingCardFiltering(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestRenderFiltersCandidateAndBoundaryFindingCards")
}

func exerciseReadOnlySPECAuditBoundary(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryValidateRenderPreserveTargetRepositoryState")
}

func exercisePortableSpecGovernancePackage(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestPortableSpecGovernancePackageRunsFromUnrelatedWorkingDirectory")
}

func exerciseOverlappingSpecGovernancePackage(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestPortableSpecGovernancePackageRejectsSourceOverlapBeforeAllocation")
}

func exercisePortableSpecGovernanceCommandBoundary(ctx context.Context) error {
	return runSpecAuditGoTests(ctx,
		"TestPortableSpecGovernancePackageRejectsUnapprovedExecutableReferencesAndRetainsPrivateRoot",
		"TestReproductionRepositoryArgumentCannotChangeCommandShape",
		"TestInventoryRejectsUnquotableRepositoryLabelsBeforeGit",
	)
}
