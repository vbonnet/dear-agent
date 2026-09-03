package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/internal/agenticreview"
)

type agenticReviewGateState struct {
	cfg     agenticreview.Config
	input   agenticreview.Input
	verdict agenticreview.Verdict
}

type agenticReviewGateStateKey struct{}

// gateReadyAt anchors every scenario clock. Scenarios that mean "still in
// flight" evaluate shortly after it; there is no scenario here that needs a
// deadline to have expired, because the down states these scenarios exercise
// are the explicit ones.
var gateReadyAt = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)

// RegisterAgenticReviewGateSteps registers the per-family review gate guardrails.
func RegisterAgenticReviewGateSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, agenticReviewGateStateKey{}, &agenticReviewGateState{}), nil
	})
	ctx.Step(`^the reviewer families claude, codex and gemini$`, theReviewerFamiliesClaudeCodexAndGemini)
	ctx.Step(`^claude approves, codex approves and gemini approves$`, allThreeFamiliesApprove)
	ctx.Step(`^claude approves, codex requests changes and gemini approves$`, codexRequestsChangesWhileOthersApprove)
	ctx.Step(`^claude approves, codex reports an error and gemini approves$`, codexReportsErrorWhileOthersApprove)
	ctx.Step(`^claude approves, codex requests changes and gemini reports an error$`, codexRequestsChangesAndGeminiErrors)
	ctx.Step(`^no family has started reviewing$`, noFamilyHasStartedReviewing)
	ctx.Step(`^claude approves, gemini approves and codex has only started$`, codexHasOnlyStarted)
	ctx.Step(`^the agentic review gate should permit the merge$`, gateShouldPermitTheMerge)
	ctx.Step(`^the agentic review gate should refuse the merge$`, gateShouldRefuseTheMerge)
	ctx.Step(`^the refusal should name (.+)$`, refusalShouldNameFamily)
}

func theReviewerFamiliesClaudeCodexAndGemini(ctx context.Context) error {
	state, err := getAgenticReviewGateState(ctx)
	if err != nil {
		return err
	}
	state.cfg = agenticreview.Config{
		Families:        agenticreview.DefaultFamilies,
		Quorum:          2,
		VerdictTimeout:  45 * time.Minute,
		DispatchTimeout: 30 * time.Minute,
	}
	state.input = agenticreview.Input{
		AppliedAt: map[string]time.Time{},
		ReadyAt:   gateReadyAt,
		Now:       gateReadyAt.Add(5 * time.Minute),
	}
	return nil
}

// publish records a family walking its lifecycle up to the given phase.
func publish(state *agenticReviewGateState, family agenticreview.Family, phases ...agenticreview.Phase) {
	for _, phase := range phases {
		label := agenticreview.Label(family, phase)
		state.input.Labels = append(state.input.Labels, label)
		state.input.AppliedAt[label] = gateReadyAt.Add(time.Minute)
	}
}

func evaluateGate(ctx context.Context, record func(*agenticReviewGateState)) error {
	state, err := getAgenticReviewGateState(ctx)
	if err != nil {
		return err
	}
	record(state)
	state.verdict, err = state.cfg.Evaluate(state.input)
	return err
}

func allThreeFamiliesApprove(ctx context.Context) error {
	return evaluateGate(ctx, func(state *agenticReviewGateState) {
		for _, family := range agenticreview.DefaultFamilies {
			publish(state, family, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		}
	})
}

func codexRequestsChangesWhileOthersApprove(ctx context.Context) error {
	return evaluateGate(ctx, func(state *agenticReviewGateState) {
		publish(state, agenticreview.FamilyClaude, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyGemini, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyCodex, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseChangesRequested)
	})
}

func codexReportsErrorWhileOthersApprove(ctx context.Context) error {
	return evaluateGate(ctx, func(state *agenticReviewGateState) {
		publish(state, agenticreview.FamilyClaude, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyGemini, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyCodex, agenticreview.PhaseStarted, agenticreview.PhaseError)
	})
}

func codexRequestsChangesAndGeminiErrors(ctx context.Context) error {
	return evaluateGate(ctx, func(state *agenticReviewGateState) {
		publish(state, agenticreview.FamilyClaude, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyCodex, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseChangesRequested)
		publish(state, agenticreview.FamilyGemini, agenticreview.PhaseStarted, agenticreview.PhaseError)
	})
}

func noFamilyHasStartedReviewing(ctx context.Context) error {
	return evaluateGate(ctx, func(*agenticReviewGateState) {})
}

func codexHasOnlyStarted(ctx context.Context) error {
	return evaluateGate(ctx, func(state *agenticReviewGateState) {
		publish(state, agenticreview.FamilyClaude, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyGemini, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)
		publish(state, agenticreview.FamilyCodex, agenticreview.PhaseStarted)
	})
}

func gateShouldPermitTheMerge(ctx context.Context) error {
	state, err := getAgenticReviewGateState(ctx)
	if err != nil {
		return err
	}
	if !state.verdict.Mergeable() {
		return fmt.Errorf("gate returned %s (%s), want a permitted merge",
			state.verdict.Decision, state.verdict.Reason)
	}
	return nil
}

func gateShouldRefuseTheMerge(ctx context.Context) error {
	state, err := getAgenticReviewGateState(ctx)
	if err != nil {
		return err
	}
	if state.verdict.Mergeable() {
		return fmt.Errorf("gate permitted the merge; want a refusal (%s)", state.verdict.Reason)
	}
	return nil
}

func refusalShouldNameFamily(ctx context.Context, family string) error {
	state, err := getAgenticReviewGateState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.verdict.Reason, family) {
		return fmt.Errorf("refusal reason %q does not name %q", state.verdict.Reason, family)
	}
	return nil
}

func getAgenticReviewGateState(ctx context.Context) (*agenticReviewGateState, error) {
	state, ok := ctx.Value(agenticReviewGateStateKey{}).(*agenticReviewGateState)
	if !ok || state == nil {
		return nil, fmt.Errorf("agentic review gate state not initialized")
	}
	return state, nil
}
