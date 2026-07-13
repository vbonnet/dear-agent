package tmux

import "testing"

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
