package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestValidateMaxDispatchRejectsNegative pins the fail-closed half of the cap: a
// misconfigured negative value must be an error, never a silent "unlimited".
func TestValidateMaxDispatchRejectsNegative(t *testing.T) {
	for _, n := range []int{-1, -100} {
		err := validateMaxDispatch(n)
		if err == nil {
			t.Errorf("validateMaxDispatch(%d) = nil, want an error (a negative cap must not read as unlimited)", n)
			continue
		}
		if !strings.Contains(err.Error(), "-max-dispatch must be >= 0") {
			t.Errorf("validateMaxDispatch(%d) error = %q, want it to name the flag and its valid range", n, err)
		}
	}
}

func TestValidateMaxDispatchAcceptsZeroAndPositive(t *testing.T) {
	for _, n := range []int{0, 1, 42} {
		if err := validateMaxDispatch(n); err != nil {
			t.Errorf("validateMaxDispatch(%d) = %v, want nil", n, err)
		}
	}
}

func TestDispatchCapReached(t *testing.T) {
	tests := []struct {
		name                               string
		maxDispatch, dispatched, remaining int
		want                               bool
		wantNotice                         string
	}{
		{name: "unlimited never reached", maxDispatch: 0, dispatched: 99, remaining: 5},
		{name: "negative never reached", maxDispatch: -1, dispatched: 99, remaining: 5},
		{name: "under budget", maxDispatch: 3, dispatched: 2, remaining: 4},
		{
			name: "budget spent", maxDispatch: 2, dispatched: 2, remaining: 18,
			want: true, wantNotice: "-max-dispatch=2 reached, deferring 18 remaining eligible bead(s)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if got := dispatchCapReached(tt.maxDispatch, tt.dispatched, tt.remaining, &errOut); got != tt.want {
				t.Errorf("dispatchCapReached(%d, %d, %d) = %v, want %v",
					tt.maxDispatch, tt.dispatched, tt.remaining, got, tt.want)
			}
			if tt.wantNotice == "" {
				if errOut.Len() != 0 {
					t.Errorf("expected no deferral notice, got %q", errOut.String())
				}
				return
			}
			if !strings.Contains(errOut.String(), tt.wantNotice) {
				t.Errorf("deferral notice = %q, want it to contain %q", errOut.String(), tt.wantNotice)
			}
		})
	}
}
