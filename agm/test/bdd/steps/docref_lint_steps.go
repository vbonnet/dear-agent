package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

type docrefLintState struct {
	classifierOutput string
	classifierErr    error
	scanOutput       string
	scanErr          error
}

type docrefLintStateKey struct{}

// RegisterDocRefLintSteps registers BDD steps that bind the living-document
// reference feature to the executable tools/docref-lint regressions.
func RegisterDocRefLintSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, docrefLintStateKey{}, &docrefLintState{}), nil
	})
	ctx.Step(`^AGM runs the docref-lint classifier regressions$`, agmRunsDocRefLintClassifierRegressions)
	ctx.Step(`^only backticked known-prefix paths should be treated as claims$`, onlyBacktickedKnownPrefixPathsShouldBeClaims)
	ctx.Step(`^AGM runs the docref-lint scan regressions$`, agmRunsDocRefLintScanRegressions)
	ctx.Step(`^a reference to a missing artifact should be reported as a finding$`, missingArtifactReferenceShouldBeReported)
}

func getDocRefLintState(ctx context.Context) (*docrefLintState, error) {
	state, ok := ctx.Value(docrefLintStateKey{}).(*docrefLintState)
	if !ok || state == nil {
		return nil, fmt.Errorf("docref-lint scenario state is not initialized")
	}
	return state, nil
}

func agmRunsDocRefLintClassifierRegressions(ctx context.Context) error {
	state, err := getDocRefLintState(ctx)
	if err != nil {
		return err
	}
	state.classifierOutput, state.classifierErr = runLocalGuardrailNamedGoTests(ctx,
		"./tools/docref-lint",
		"TestHasKnownPrefix",
		"TestRefPatternMatchesOnlyBacktickedPaths",
		"TestResolvesAgainstRootThenAncestors",
	)
	return nil
}

func onlyBacktickedKnownPrefixPathsShouldBeClaims(ctx context.Context) error {
	state, err := getDocRefLintState(ctx)
	if err != nil {
		return err
	}
	if state.classifierErr != nil {
		return fmt.Errorf("docref-lint classifier regressions: %w: %s", state.classifierErr, state.classifierOutput)
	}
	return nil
}

func agmRunsDocRefLintScanRegressions(ctx context.Context) error {
	state, err := getDocRefLintState(ctx)
	if err != nil {
		return err
	}
	state.scanOutput, state.scanErr = runLocalGuardrailNamedGoTests(ctx,
		"./tools/docref-lint",
		"TestScanDocReportsOnlyMissingKnownPrefixRefs",
		"TestScanDocSurfacesUnreadableDocumentAsError",
	)
	return nil
}

func missingArtifactReferenceShouldBeReported(ctx context.Context) error {
	state, err := getDocRefLintState(ctx)
	if err != nil {
		return err
	}
	if state.scanErr != nil {
		return fmt.Errorf("docref-lint scan regressions: %w: %s", state.scanErr, state.scanOutput)
	}
	return nil
}
