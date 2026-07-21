package tmux

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAgySurveyOverridesReadyPrompt(t *testing.T) {
	content := "Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"
	if !ContainsAgySurveyPrompt(content) {
		t.Fatal("AGY survey footer was not detected")
	}
	if containsAgyReadyPattern(content) {
		t.Fatal("survey footer must override the bare ready prompt")
	}
}

func TestAgyOrdinaryPromptIsNotSurvey(t *testing.T) {
	content := "Task complete\n>"
	if ContainsAgySurveyPrompt(content) {
		t.Fatal("ordinary AGY prompt was classified as survey")
	}
	if !containsAgyReadyPattern(content) {
		t.Fatal("ordinary AGY prompt should remain ready")
	}
}

func TestContainsAgyPromptAfterSurveyRequiresLaterPrompt(t *testing.T) {
	survey := "How's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"
	if containsAgyPromptAfterSurvey("Task complete\n>\n" + survey) {
		t.Fatal("prompt preceding the survey was treated as the post-dismissal composer")
	}
	if !containsAgyPromptAfterSurvey(survey + "\nAGY ready\n>") {
		t.Fatal("prompt following stale survey history was not treated as the composer")
	}
}

func TestWaitForAgyPromptAcceptsTrustBeforeReady(t *testing.T) {
	outputs := [][]byte{
		[]byte("Do you trust the contents of this project?\n> Yes"),
		[]byte("AGY ready\n> "),
	}
	captureIndex := 0
	var sent []string
	runtime := agyPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			output := outputs[captureIndex]
			if captureIndex < len(outputs)-1 {
				captureIndex++
			}
			return output, nil
		},
		sendKeys: func(sessionName, keys string) error {
			if sessionName != "agy-trust" {
				t.Fatalf("session = %q, want agy-trust", sessionName)
			}
			sent = append(sent, keys)
			return nil
		},
		sleep: func(context.Context, time.Duration) {},
	}
	if err := waitForAgyPromptWithRuntime(context.Background(), "agy-trust", time.Second, runtime); err != nil {
		t.Fatalf("waitForAgyPromptWithRuntime: %v", err)
	}
	if len(sent) != 1 || sent[0] != "Enter" {
		t.Fatalf("trust keys = %v, want [Enter]", sent)
	}
}

func assertAgySurveySequence(t *testing.T, sessionName string, outputs [][]byte) {
	t.Helper()
	captureIndex := 0
	var sent []string
	runtime := agyPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			output := outputs[captureIndex]
			if captureIndex < len(outputs)-1 {
				captureIndex++
			}
			return output, nil
		},
		sendKeys: func(_ string, keys string) error {
			sent = append(sent, keys)
			return nil
		},
		sleep: func(context.Context, time.Duration) {},
	}
	if err := waitForAgyPromptWithRuntime(context.Background(), sessionName, time.Second, runtime); err != nil {
		t.Fatalf("waitForAgyPromptWithRuntime: %v", err)
	}
	if len(sent) != 1 || sent[0] != "0" {
		t.Fatalf("survey keys = %v, want one dismissal", sent)
	}
}

func TestWaitForAgyPromptDismissesSurveyBeforeReady(t *testing.T) {
	assertAgySurveySequence(t, "agy-survey", [][]byte{
		[]byte("Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"),
		[]byte("AGY ready\n> "),
	})
}

func TestWaitForAgyPromptDoesNotRedismissStaleSurvey(t *testing.T) {
	assertAgySurveySequence(t, "agy-stale-survey", [][]byte{
		[]byte("Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"),
		[]byte("How's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip\nAGY ready\n> "),
	})
}

func TestWaitForAgyPromptHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	captured := false
	runtime := agyPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			captured = true
			return nil, nil
		},
		sendKeys: func(string, string) error { return nil },
		sleep:    func(context.Context, time.Duration) {},
	}

	err := waitForAgyPromptWithRuntime(ctx, "agy-cancelled", time.Second, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if captured {
		t.Fatal("pane capture ran after caller cancellation")
	}
}

func TestWaitForAgyPromptReturnsCancellationAfterReadyStabilityDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runtime := agyPromptRuntime{
		capture:  func(context.Context, string) ([]byte, error) { return []byte("AGY ready\n> "), nil },
		sendKeys: func(string, string) error { return nil },
		sleep: func(_ context.Context, _ time.Duration) {
			cancel()
		},
	}

	err := waitForAgyPromptWithRuntime(ctx, "agy-ready-cancelled", time.Second, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
