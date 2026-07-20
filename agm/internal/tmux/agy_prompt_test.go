package tmux

import (
	"context"
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
		sleep: func(time.Duration) {},
	}
	if err := waitForAgyPromptWithRuntime(context.Background(), "agy-trust", time.Second, runtime); err != nil {
		t.Fatalf("waitForAgyPromptWithRuntime: %v", err)
	}
	if len(sent) != 1 || sent[0] != "Enter" {
		t.Fatalf("trust keys = %v, want [Enter]", sent)
	}
}

func TestWaitForAgyPromptDismissesSurveyBeforeReady(t *testing.T) {
	outputs := [][]byte{
		[]byte("Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"),
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
		sendKeys: func(_ string, keys string) error {
			sent = append(sent, keys)
			return nil
		},
		sleep: func(time.Duration) {},
	}
	if err := waitForAgyPromptWithRuntime(context.Background(), "agy-survey", time.Second, runtime); err != nil {
		t.Fatalf("waitForAgyPromptWithRuntime: %v", err)
	}
	if len(sent) != 1 || sent[0] != "0" {
		t.Fatalf("survey keys = %v, want [0]", sent)
	}
}
