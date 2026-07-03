package tmux

import "testing"

func TestAutonomousModeToggle(t *testing.T) {
	t.Cleanup(func() { SetAutonomousMode(false) })

	if AutonomousMode() {
		t.Fatal("autonomous mode should default to false")
	}
	SetAutonomousMode(true)
	if !AutonomousMode() {
		t.Fatal("autonomous mode should be true after SetAutonomousMode(true)")
	}
	SetAutonomousMode(false)
	if AutonomousMode() {
		t.Fatal("autonomous mode should be false after SetAutonomousMode(false)")
	}
}
