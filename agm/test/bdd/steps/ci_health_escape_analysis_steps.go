package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

type ciHealthState struct {
	classifyOutput string
	classifyErr    error
	roiOutput      string
	roiErr         error
}

type ciHealthStateKey struct{}

// RegisterCIHealthEscapeAnalysisSteps registers BDD steps that bind the CI
// escape-analysis feature to the executable pkg/cihealth regressions.
func RegisterCIHealthEscapeAnalysisSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, ciHealthStateKey{}, &ciHealthState{}), nil
	})
	ctx.Step(`^AGM runs the CI escape classification regressions$`, agmRunsCIEscapeClassificationRegressions)
	ctx.Step(`^each escape class should name the mechanism that fixes it$`, eachEscapeClassShouldNameItsMechanism)
	ctx.Step(`^AGM runs the CI escape ROI regressions$`, agmRunsCIEscapeROIRegressions)
	ctx.Step(`^unmeasured prevention cost should not produce a placement verdict$`, unmeasuredPreventionShouldNotProduceVerdict)
}

func getCIHealthState(ctx context.Context) (*ciHealthState, error) {
	state, ok := ctx.Value(ciHealthStateKey{}).(*ciHealthState)
	if !ok || state == nil {
		return nil, fmt.Errorf("CI health scenario state is not initialized")
	}
	return state, nil
}

func agmRunsCIEscapeClassificationRegressions(ctx context.Context) error {
	state, err := getCIHealthState(ctx)
	if err != nil {
		return err
	}
	state.classifyOutput, state.classifyErr = runLocalGuardrailNamedGoTests(ctx,
		"./pkg/cihealth",
		"TestClassify",
		"TestFilterRefinableOnlyForSelectionClasses",
		"TestScheduleOnlyWorkflowIsNotAnEscape",
		"TestNonSuccessConclusionIsNotTreatedAsAPass",
		"TestUnknownRequiredContextsDoNotAssertAdvisory",
		"TestScheduledDetectionIsNotPinnedOnTheHeadCommit",
		"TestRetroNamesOnlyMechanismsThatExist",
		"TestPostMergeOnlyRetroDoesNotPricePlacement",
		"TestMergeSkewDeclinesPlacementWithoutCallingItANonEscape",
		"TestPlacementIsPricedOnlyWhereItIsTheDecision",
		"TestRequiredContextMatchesOnProducingApp",
		"TestBriefDoesNotClaimToBeACompletedRetro",
	)
	return nil
}

func eachEscapeClassShouldNameItsMechanism(ctx context.Context) error {
	state, err := getCIHealthState(ctx)
	if err != nil {
		return err
	}
	if state.classifyErr != nil {
		return fmt.Errorf("CI escape classification regressions: %w: %s", state.classifyErr, state.classifyOutput)
	}
	return nil
}

func agmRunsCIEscapeROIRegressions(ctx context.Context) error {
	state, err := getCIHealthState(ctx)
	if err != nil {
		return err
	}
	state.roiOutput, state.roiErr = runLocalGuardrailNamedGoTests(ctx,
		"./pkg/cihealth",
		"TestROIRatioAndVerdict",
		"TestROIFreePreventionIsAlwaysWorthIt",
		"TestROIFreePreventionWithNoEscapesIsNoSignal",
		"TestROIExplainShowsItsWork",
		"TestUnmeasuredPreventionIsNotFreePrevention",
		"TestAssumedCureCostYieldsAProvisionalVerdict",
		"TestTruncatedPreventionIsReportedAsALowerBound",
		"TestFullyMeasuredROIIsNotProvisional",
	)
	return nil
}

func unmeasuredPreventionShouldNotProduceVerdict(ctx context.Context) error {
	state, err := getCIHealthState(ctx)
	if err != nil {
		return err
	}
	if state.roiErr != nil {
		return fmt.Errorf("CI escape ROI regressions: %w: %s", state.roiErr, state.roiOutput)
	}
	return nil
}
