package tmux

import "testing"

func TestForceDeliveryToggle(t *testing.T) {
	t.Cleanup(func() { SetForceDelivery(false) })

	if ForceDelivery() {
		t.Fatal("force delivery should default to false")
	}
	SetForceDelivery(true)
	if !ForceDelivery() {
		t.Fatal("force delivery should be true after SetForceDelivery(true)")
	}
	SetForceDelivery(false)
	if ForceDelivery() {
		t.Fatal("force delivery should be false after SetForceDelivery(false)")
	}
}

// TestSkipPostSubmitGuard pins the gate that decides whether the post-submit
// human-typing cooldown abort is skipped. The ce-5sow fix is the `force` column:
// before it, a forced send still ran the cooldown check and re-aborted.
func TestSkipPostSubmitGuard(t *testing.T) {
	tests := []struct {
		name            string
		shouldInterrupt bool
		autonomous      bool
		force           bool
		want            bool
	}{
		{
			name: "attended normal send runs the guard",
			want: false,
		},
		{
			name:            "interrupting send skips the guard",
			shouldInterrupt: true,
			want:            true,
		},
		{
			name:       "autonomous send skips the guard",
			autonomous: true,
			want:       true,
		},
		{
			name:  "forced send skips the guard (ce-5sow)",
			force: true,
			want:  true,
		},
		{
			name:       "force wins even when not interrupting and attended",
			autonomous: false,
			force:      true,
			want:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipPostSubmitGuard(tc.shouldInterrupt, tc.autonomous, tc.force); got != tc.want {
				t.Errorf("skipPostSubmitGuard(%v, %v, %v) = %v, want %v",
					tc.shouldInterrupt, tc.autonomous, tc.force, got, tc.want)
			}
		})
	}
}
