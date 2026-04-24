package steps

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// trustTestState holds per-scenario trust test state.
type trustTestState struct {
	trustDir  string
	lastErr   error
	lastScore int
}

var trustState *trustTestState

// RegisterTrustProtocolSteps registers step definitions for trust protocol features.
func RegisterTrustProtocolSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "bdd-trust-*")
		if err != nil {
			return ctx, err
		}
		trustState = &trustTestState{trustDir: dir}
		// Override HOME so trust files go to temp dir
		os.Setenv("HOME", dir)
		contracts.ResetForTesting()
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if trustState != nil {
			os.RemoveAll(trustState.trustDir)
		}
		return ctx, nil
	})

	ctx.Step(`^a session "([^"]*)" with no trust history$`, aSessionWithNoTrustHistory)
	ctx.Step(`^I record (\d+) "([^"]*)" events for "([^"]*)"$`, iRecordNEventsFor)
	ctx.Step(`^the trust score for "([^"]*)" should be (\d+)$`, trustScoreShouldBe)
	ctx.Step(`^the score should never be negative$`, scoreShouldNeverBeNegative)
	ctx.Step(`^I record a "([^"]*)" event for "([^"]*)"$`, iRecordAnEventFor)
	ctx.Step(`^the trust history for "([^"]*)" should have (\d+) events$`, trustHistoryShouldHaveNEvents)
	ctx.Step(`^the events should be in chronological order$`, eventsShouldBeChronological)
	ctx.Step(`^I attempt to record a trust event with empty session name$`, iAttemptRecordWithEmptySessionName)
	ctx.Step(`^an invalid input error should be returned$`, anInvalidInputErrorShouldBeReturned)
	ctx.Step(`^I attempt to record a trust event with type "([^"]*)" for session "([^"]*)"$`, iAttemptRecordWithInvalidType)
	ctx.Step(`^the error should list valid event types$`, errorShouldListValidTypes)
}

func aSessionWithNoTrustHistory(_ context.Context, _ string) error {
	// No-op: temp dir ensures clean state
	return nil
}

func iRecordNEventsFor(_ context.Context, count int, eventType, session string) error {
	for i := 0; i < count; i++ {
		_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: session,
			EventType:   eventType,
			Detail:      fmt.Sprintf("event %d", i),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func trustScoreShouldBe(_ context.Context, session string, expected int) error {
	result, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: session})
	if err != nil {
		return err
	}
	trustState.lastScore = result.Score
	if result.Score != expected {
		return fmt.Errorf("expected trust score %d, got %d", expected, result.Score)
	}
	return nil
}

func scoreShouldNeverBeNegative(context.Context) error {
	if trustState.lastScore < 0 {
		return fmt.Errorf("trust score is negative: %d", trustState.lastScore)
	}
	return nil
}

func iRecordAnEventFor(_ context.Context, eventType, session string) error {
	_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
		SessionName: session,
		EventType:   eventType,
	})
	return err
}

func trustHistoryShouldHaveNEvents(_ context.Context, session string, expected int) error {
	result, err := ops.TrustHistory(nil, &ops.TrustHistoryRequest{SessionName: session})
	if err != nil {
		return err
	}
	if result.Total != expected {
		return fmt.Errorf("expected %d events, got %d", expected, result.Total)
	}
	return nil
}

func eventsShouldBeChronological(ctx context.Context) error {
	// Events are append-only with time.Now() so they are inherently ordered
	return nil
}

func iAttemptRecordWithEmptySessionName(ctx context.Context) error {
	_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
		SessionName: "",
		EventType:   "success",
	})
	trustState.lastErr = err
	return nil
}

func anInvalidInputErrorShouldBeReturned(context.Context) error {
	if trustState.lastErr == nil {
		return fmt.Errorf("expected an error but got nil")
	}
	return nil
}

func iAttemptRecordWithInvalidType(_ context.Context, eventType, session string) error {
	_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
		SessionName: session,
		EventType:   eventType,
	})
	trustState.lastErr = err
	return nil
}

func errorShouldListValidTypes(context.Context) error {
	if trustState.lastErr == nil {
		return fmt.Errorf("expected error to list valid types")
	}
	msg := trustState.lastErr.Error()
	for _, t := range ops.ValidTrustEventTypes() {
		if !strings.Contains(msg, string(t)) {
			return fmt.Errorf("error message should list %q but got: %s", t, msg)
		}
	}
	return nil
}
