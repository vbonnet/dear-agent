package tmux

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForAgyPromptRejectsFirstRunOnboardingWithoutInput(t *testing.T) {
	tests := map[string]string{
		"theme":         "Welcome to Antigravity CLI!\nChoose your color scheme:\n> terminal",
		"terms":         "Terms of Service & Data Use\n[ ] Yes, I agree to help improve Antigravity CLI\nPrevious  Done",
		"wrapped terms": "Terms of Service & Data\nUse\n[ ] Yes, I agree to help improve Antigravity\nCLI\nPrevious  Done",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			captures := 0
			sends := 0
			runtime := agyPromptRuntime{
				capture: func(context.Context, string) ([]byte, error) {
					captures++
					return []byte(content), nil
				},
				sendKeys: func(string, string) error {
					sends++
					return nil
				},
				sleep: func(context.Context, time.Duration) {},
			}

			err := waitForAgyPromptWithRuntime(t.Context(), "agy-onboarding", time.Second, runtime)
			if !errors.Is(err, ErrAgyOnboardingRequired) {
				t.Fatalf("error = %v, want ErrAgyOnboardingRequired", err)
			}
			if !strings.Contains(err.Error(), "will not accept legal or data-use choices automatically") {
				t.Fatalf("error lacks non-consent guidance: %v", err)
			}
			if captures != 1 || sends != 0 {
				t.Fatalf("onboarding I/O = %d capture(s), %d send(s); want one capture and no input", captures, sends)
			}
		})
	}
}

func TestAgyOnboardingDetectionRequiresActiveScreen(t *testing.T) {
	tests := map[string]struct {
		content string
		want    bool
	}{
		"active theme": {
			content: "Welcome to Antigravity CLI!\nChoose your color scheme:\n> terminal",
			want:    true,
		},
		"active terms": {
			content: "Terms of Service & Data Use\n[x] Yes, I agree to help improve Antigravity CLI\nPrevious  Done",
			want:    true,
		},
		"active wrapped terms": {
			content: "Terms of Service & Data\nUse\n[x] Yes, I agree to help improve Antigravity\nCLI\nPrevious  Done",
			want:    true,
		},
		"stale theme before composer": {
			content: "Welcome to Antigravity CLI!\nChoose your color scheme:\n> terminal\nsetup complete\n>",
		},
		"stale terms before composer": {
			content: "Terms of Service & Data Use\n[x] Yes, I agree to help improve Antigravity CLI\nPrevious  Done\nsetup complete\n>",
		},
		"conversational marker": {
			content: "I can explain Terms of Service & Data Use.\n>",
		},
		"incomplete active marker": {
			content: "Terms of Service & Data Use",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := containsAgyOnboardingPrompt(test.content); got != test.want {
				t.Fatalf("containsAgyOnboardingPrompt() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWaitForAgyPromptAfterInputIgnoresQuotedOnboarding(t *testing.T) {
	captures := 0
	sends := 0
	runtime := agyPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			captures++
			return []byte("previous composer\n>\n> you: quote this screen\nWelcome to Antigravity CLI!\nChoose your color scheme:\n> terminal\nresponse complete\n>"), nil
		},
		sendKeys: func(string, string) error {
			sends++
			return nil
		},
		sleep: func(context.Context, time.Duration) {},
	}

	if err := waitForAgyPromptAfterInputWithRuntime(t.Context(), "agy-transcript", time.Second, runtime); err != nil {
		t.Fatalf("post-input wait rejected quoted onboarding text: %v", err)
	}
	if captures != 1 || sends != 0 {
		t.Fatalf("post-input I/O = %d capture(s), %d send(s); want one capture and no input", captures, sends)
	}
}

func TestWaitForAgyPromptOnResumeIgnoresTransientQuotedOnboarding(t *testing.T) {
	outputs := [][]byte{
		[]byte("previous composer\n>\n> you: quote this screen\nWelcome to Antigravity CLI!\nChoose your color scheme:\n> terminal"),
		[]byte("previous composer\n>\n> you: quote this screen\nWelcome to Antigravity CLI!\nChoose your color scheme:\n> terminal\nresume complete\n>"),
	}
	captureIndex := 0
	captures := 0
	sends := 0
	runtime := agyPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			output := outputs[captureIndex]
			captures++
			if captureIndex < len(outputs)-1 {
				captureIndex++
			}
			return output, nil
		},
		sendKeys: func(string, string) error {
			sends++
			return nil
		},
		sleep: func(context.Context, time.Duration) {},
	}

	if err := waitForAgyPromptOnResumeWithRuntime(t.Context(), "agy-resume-transcript", time.Second, runtime); err != nil {
		t.Fatalf("resume wait rejected transient quoted onboarding text: %v", err)
	}
	if captures != len(outputs) || sends != 0 {
		t.Fatalf("resume transcript I/O = %d capture(s), %d send(s); want %d captures and no input", captures, sends, len(outputs))
	}
}

func TestWaitForAgyPromptOnResumeConfirmsPersistentOnboardingWithoutInput(t *testing.T) {
	captures := 0
	sends := 0
	runtime := agyPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			captures++
			return []byte("Welcome to Antigravity CLI!\nChoose your color scheme:\n> terminal"), nil
		},
		sendKeys: func(string, string) error {
			sends++
			return nil
		},
		sleep: func(context.Context, time.Duration) {},
	}

	err := waitForAgyPromptOnResumeWithRuntime(t.Context(), "agy-resume-onboarding", time.Second, runtime)
	if !errors.Is(err, ErrAgyOnboardingRequired) {
		t.Fatalf("resume wait error = %v, want ErrAgyOnboardingRequired", err)
	}
	if captures != agyResumeOnboardingConfirmationChecks || sends != 0 {
		t.Fatalf("persistent onboarding I/O = %d capture(s), %d send(s); want %d captures and no input", captures, sends, agyResumeOnboardingConfirmationChecks)
	}
}

func TestAgySurveyOverridesReadyPrompt(t *testing.T) {
	content := "Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"
	if !ContainsAgySurveyPrompt(content) {
		t.Fatal("AGY survey footer was not detected")
	}
	if containsAgyReadyPattern(content) {
		t.Fatal("survey footer must override the bare ready prompt")
	}
}

func TestAgyStaleSurveyAllowsLaterReadyPrompt(t *testing.T) {
	survey := "How's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"
	if !containsAgyReadyPattern(survey + "\nTask complete\n>") {
		t.Fatal("prompt after stale survey history should be ready")
	}
	if containsAgyReadyPattern("Task complete\n>\n" + survey) {
		t.Fatal("prompt preceding a live survey should not be ready")
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
