package steps

import "testing"

func TestRealTmuxReadinessBehaviorSatisfiedRequiresPassOrConfiguredSkip(t *testing.T) {
	const behavior = "TestRealTmuxReadinessDetectsManagedPiComposer"

	if !realTmuxReadinessBehaviorSatisfied("--- PASS: "+behavior+" (0.10s)", behavior) {
		t.Fatal("passing real-tmux behavior was rejected")
	}

	t.Setenv("CI_SKIP_TMUX", "false")
	if realTmuxReadinessBehaviorSatisfied("--- SKIP: "+behavior+" (0.00s)", behavior) {
		t.Fatal("unconfigured real-tmux skip was accepted")
	}
	managedSkip := "=== RUN   " + behavior + "\n" +
		"tmux_real_readiness_test.go:20: process-table inspection is unavailable: operation not permitted\n" +
		"--- SKIP: " + behavior + " (0.00s)"
	if !realTmuxReadinessBehaviorSatisfied(managedSkip, behavior) {
		t.Fatal("self-diagnosed managed process-table skip was rejected")
	}
	if realTmuxReadinessBehaviorSatisfied("tmux is not available\n--- SKIP: "+behavior+" (0.00s)", behavior) {
		t.Fatal("an unrelated unconfigured capability skip was accepted")
	}

	t.Setenv("CI_SKIP_TMUX", "true")
	if !realTmuxReadinessBehaviorSatisfied("--- SKIP: "+behavior+" (0.00s)", behavior) {
		t.Fatal("configured real-tmux skip was rejected")
	}
	if realTmuxReadinessBehaviorSatisfied("--- SKIP: TestDifferentBehavior (0.00s)", behavior) {
		t.Fatal("a different skipped behavior satisfied the assertion")
	}
}
