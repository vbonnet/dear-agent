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

func TestShouldClearStaleInput(t *testing.T) {
	tests := []struct {
		name    string
		ansi    string
		want    bool
		comment string
	}{
		{
			name:    "stale agm command in input box",
			ansi:    "some output\n❯ merge PR 527\n",
			want:    true,
			comment: "un-submitted AGM text must be stashed, not treated as human typing",
		},
		{
			name:    "another stale command",
			ansi:    "❯ pkill -x gopls\n",
			want:    true,
			comment: "the classic ghost-text deadlock string",
		},
		{
			name:    "empty input box",
			ansi:    "previous response\n❯ \n",
			want:    false,
			comment: "nothing to clear",
		},
		{
			name:    "no prompt at all",
			ansi:    "just some output\n",
			want:    false,
			comment: "no input line to clear",
		},
		{
			name:    "ghost/placeholder text is left alone",
			ansi:    "❯ \x1b[2mstart the loop\x1b[0m\n",
			want:    false,
			comment: "dim ghost text has nothing real to stash and does not block submit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldClearStaleInput(tc.ansi); got != tc.want {
				t.Errorf("shouldClearStaleInput() = %v, want %v (%s)", got, tc.want, tc.comment)
			}
		})
	}
}

// TestClearStaleInputLockedGatedOnAutonomous verifies the stash is a no-op
// unless autonomous mode is active, even when stale text is present — attended
// sessions keep the original human-protecting behavior.
func TestClearStaleInputLockedGatedOnAutonomous(t *testing.T) {
	t.Cleanup(func() { SetAutonomousMode(false) })
	staleInput := "❯ merge PR 527\n"

	SetAutonomousMode(false)
	if clearStaleInputLocked("/nonexistent.sock", "no-such-session", staleInput) {
		t.Error("clearStaleInputLocked must be a no-op when autonomous mode is off")
	}

	// With autonomous on and stashable content, it attempts the C-s send. The
	// target socket/session does not exist, so the exec fails and it returns
	// false — but the important contract (no-op when off) is covered above, and
	// the pure decision is covered by TestShouldClearStaleInput.
	SetAutonomousMode(true)
	if shouldClearStaleInput(staleInput) != true {
		t.Error("guard precondition: stale input should be deemed stashable")
	}
}
