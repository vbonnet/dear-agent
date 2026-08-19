//go:build darwin || linux

package steps

import (
	"context"

	"github.com/cucumber/godog"
)

// registerSpecGovernanceRenderingSteps keeps the cohesive HTML-rendering
// checks out of the general spec-governance step module.
func registerSpecGovernanceRenderingSteps(ctx *godog.ScenarioContext) {
	ctx.Step(specGovernanceOfflineRenderingStep, exerciseBoundedOfflineSPECAuditRendering)
	ctx.Step(specGovernanceFindingFilterStep, exerciseSPECAuditFindingCardFiltering)
	ctx.Step(specGovernanceReadOnlyBoundaryStep, exerciseReadOnlySPECAuditBoundary)
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
