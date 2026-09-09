package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

type reviewThreadReplySafetyState struct {
	output string
	err    error
}

type reviewThreadReplySafetyStateKey struct{}

// RegisterReviewThreadReplySafetySteps registers the data-only review reply behavior.
func RegisterReviewThreadReplySafetySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, reviewThreadReplySafetyStateKey{}, &reviewThreadReplySafetyState{}), nil
	})

	ctx.Step(`^AGM runs the review reply data-only regressions$`, agmRunsReviewReplyDataOnlyRegressions)
	ctx.Step(`^reply-resolve should require a body-file source$`, replyResolveShouldRequireBodyFileSource)
	ctx.Step(
		`^invalid body sources should fail before GitHub mutation$`,
		invalidBodySourcesShouldFailBeforeGitHubMutation,
	)
	ctx.Step(
		`^oversized and endless body sources should be bounded before GitHub mutation$`,
		oversizedAndEndlessBodySourcesShouldBeBoundedBeforeGitHubMutation,
	)
	ctx.Step(
		`^a replaced named source should fail without blocking$`,
		replacedNamedSourceShouldFailWithoutBlocking,
	)
	ctx.Step(
		`^GitHub should receive the exact reply bytes without shell evaluation$`,
		githubShouldReceiveExactReplyBytesWithoutShellEvaluation,
	)
	ctx.Step(
		`^failed provider diagnostics should not echo the reply body$`,
		failedProviderDiagnosticsShouldNotEchoReplyBody,
	)
	ctx.Step(
		`^body-free provider diagnostics should remain available$`,
		bodyFreeProviderDiagnosticsShouldRemainAvailable,
	)
	ctx.Step(
		`^retry guidance should reuse the body-file source without rendering its content$`,
		retryGuidanceShouldReuseBodyFileWithoutRenderingContent,
	)
}

func agmRunsReviewReplyDataOnlyRegressions(ctx context.Context) error {
	state, err := getReviewThreadReplySafetyState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx,
		"./cmd/resolve-review-threads",
		"TestReplyResolveRequiresBodyFile",
		"TestReplyResolveRejectsInvalidBodySourcesBeforeProviderMutation",
		"TestLoadReplyBodyPreservesExactBytes",
		"TestLoadReplyBodyRejectsOversizedOrEndlessSources",
		"TestLoadReplyBodyRejectsReplacedFIFOWithoutBlocking",
		"TestLoadReplyBodyUsesNonblockingOpenerWithoutPathStat",
		"TestGHGraphQLSendsQueryAndVariablesAsJSONStdin",
		"TestGHGraphQLSuppressesReplyBodyEchoedByChildStderr",
		"TestGHGraphQLPreservesBodyFreeChildStderr",
		"TestRetryAdviceDoesNotRenderReplyBody",
	)
	return nil
}

func replyResolveShouldRequireBodyFileSource(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func invalidBodySourcesShouldFailBeforeGitHubMutation(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func oversizedAndEndlessBodySourcesShouldBeBoundedBeforeGitHubMutation(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func replacedNamedSourceShouldFailWithoutBlocking(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func githubShouldReceiveExactReplyBytesWithoutShellEvaluation(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func failedProviderDiagnosticsShouldNotEchoReplyBody(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func bodyFreeProviderDiagnosticsShouldRemainAvailable(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func retryGuidanceShouldReuseBodyFileWithoutRenderingContent(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func requireReviewThreadReplySafetyRegressions(ctx context.Context) error {
	state, err := getReviewThreadReplySafetyState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("review reply data-only regressions: %w\n%s", state.err, state.output)
	}
	return nil
}

func getReviewThreadReplySafetyState(ctx context.Context) (*reviewThreadReplySafetyState, error) {
	state, ok := ctx.Value(reviewThreadReplySafetyStateKey{}).(*reviewThreadReplySafetyState)
	if !ok || state == nil {
		return nil, fmt.Errorf("review thread reply safety state not initialized")
	}
	return state, nil
}
