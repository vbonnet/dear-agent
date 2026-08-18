package state

import (
	"testing"
	"time"
)

const agySurveyPane = "Task complete\n>\nHow's the CLI experience so far? [1] Good [2] Fine [3] Bad [0] Skip"

func TestAgySurveyBlocksReadyPrompt(t *testing.T) {
	detector := NewDetector()
	result := detector.DetectState(agySurveyPane, time.Now())
	if result.State != StateBackgroundTasksView {
		t.Fatalf("DetectState() = %q, want dismissible overlay", result.State)
	}
	if got := detector.CheckCanReceive(agySurveyPane); got != CanReceiveOverlay {
		t.Fatalf("CheckCanReceive() = %q, want %q", got, CanReceiveOverlay)
	}
}

func TestAgyStaleSurveyAllowsLaterReadyPrompt(t *testing.T) {
	detector := NewDetector()
	content := agySurveyPane + "\nTask complete\n>"
	result := detector.DetectState(content, time.Now())
	if result.State != StateReady {
		t.Fatalf("DetectState() = %q, want ready after stale survey", result.State)
	}
	if got := detector.CheckCanReceive(content); got != CanReceiveYes {
		t.Fatalf("CheckCanReceive() = %q, want %q", got, CanReceiveYes)
	}
}

func TestAgyPromptBeforeSurveyRemainsOverlay(t *testing.T) {
	detector := NewDetector()
	content := "Task complete\n>\n" + agySurveyPane
	if got := detector.CheckCanReceive(content); got != CanReceiveOverlay {
		t.Fatalf("CheckCanReceive() = %q, want %q", got, CanReceiveOverlay)
	}
}
