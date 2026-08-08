package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/internal/specguard"
)

type specGuardStateKey struct{}

type specGuardState struct {
	request            specguard.Request
	result             specguard.Result
	deletionOutput     string
	deletionRegression error
}

// RegisterSpecGuardSteps registers the provider-neutral guard interface steps.
func RegisterSpecGuardSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, specGuardStateKey{}, &specGuardState{}), nil
	})
	ctx.Step(`^malformed provider-neutral SPEC guard case "([^"]+)"$`, malformedSpecGuardCase)
	ctx.Step(`^the shared SPEC guard interface evaluates the request$`, evaluateSharedSpecGuard)
	ctx.Step(`^the SPEC guard result should block and disclose its source, cooperative hook, and repository identity boundaries$`, specGuardShouldBlockWithBoundedEvidence)
	ctx.Step(`^governed contract deletion validation is configured$`, governedContractDeletionValidationIsConfigured)
	ctx.Step(`^AGM exercises dangling deletion, live owner deletion, complete retirement, and same-change relocation$`, agmExercisesGovernedContractDeletion)
	ctx.Step(`^only structurally owned replacement, complete retirement, or relocation should reach semantic review$`, completeRetirementOrRelocationReachesSemanticReview)
}

func malformedSpecGuardCase(ctx context.Context, testCase string) error {
	state, err := getSpecGuardState(ctx)
	if err != nil {
		return err
	}
	switch testCase {
	case "unknown-mode":
		state.request = specguard.Request{Repository: ".", Mode: "unknown"}
	case "staged-with-base":
		state.request = specguard.Request{Repository: ".", Mode: specguard.ModeStaged, Base: "main"}
	case "committed-no-base":
		state.request = specguard.Request{Repository: ".", Mode: specguard.ModeCommitted}
	default:
		return fmt.Errorf("unknown malformed SPEC guard case %q", testCase)
	}
	return nil
}

func evaluateSharedSpecGuard(ctx context.Context) error {
	state, err := getSpecGuardState(ctx)
	if err != nil {
		return err
	}
	state.result = specguard.Evaluate(ctx, state.request)
	return nil
}

func specGuardShouldBlockWithBoundedEvidence(ctx context.Context) error {
	state, err := getSpecGuardState(ctx)
	if err != nil {
		return err
	}
	if state.result.Decision != specguard.DecisionBlock {
		return fmt.Errorf("SPEC guard decision = %q, want block", state.result.Decision)
	}
	if state.result.Scope != specguard.GuardScope {
		return fmt.Errorf("SPEC guard scope = %q", state.result.Scope)
	}
	claim := strings.ToLower(state.result.EvidenceClaim)
	for _, phrase := range []string{
		"semantic validation is limited to local git index or commit-tree objects",
		"path/status metadata",
		"mutable working-tree bodies are not parsed",
		"no provider",
		"runtime state",
	} {
		if !strings.Contains(claim, phrase) {
			return fmt.Errorf("SPEC guard evidence claim %q omits %q", state.result.EvidenceClaim, phrase)
		}
	}
	trust := strings.ToLower(state.result.TrustBoundary)
	for _, phrase := range []string{
		"checkpoint-revalidates",
		"worktree root",
		"git directory",
		"git common directory",
		"their ancestors",
		"before and after each git command",
		"filesystem behavior between checkpoints",
		"cooperative feedback only",
		"not tamper-resistant",
		"mandatory immutable enforcement must come from a separately reviewed changed-spec ci and provider rollout",
		"does not attest that such enforcement is deployed, has run for a change, or is provider-required",
	} {
		if !strings.Contains(trust, phrase) {
			return fmt.Errorf("SPEC guard trust boundary %q omits %q", state.result.TrustBoundary, phrase)
		}
	}
	return nil
}

func governedContractDeletionValidationIsConfigured(ctx context.Context) error {
	_, err := getSpecGuardState(ctx)
	return err
}

func agmExercisesGovernedContractDeletion(ctx context.Context) error {
	state, err := getSpecGuardState(ctx)
	if err != nil {
		return err
	}
	state.deletionOutput, state.deletionRegression = runLocalGuardrailNamedGoTests(ctx,
		"./internal/specguard",
		"TestGovernedDeletionValidatesSurvivingGraphAndAllowsReviewedRetirement",
	)
	return nil
}

func completeRetirementOrRelocationReachesSemanticReview(ctx context.Context) error {
	state, err := getSpecGuardState(ctx)
	if err != nil {
		return err
	}
	if state.deletionRegression != nil {
		return fmt.Errorf("governed contract deletion regression: %w: %s", state.deletionRegression, state.deletionOutput)
	}
	return nil
}

func getSpecGuardState(ctx context.Context) (*specGuardState, error) {
	state, ok := ctx.Value(specGuardStateKey{}).(*specGuardState)
	if !ok || state == nil {
		return nil, fmt.Errorf("SPEC guard scenario state is not initialized")
	}
	return state, nil
}
