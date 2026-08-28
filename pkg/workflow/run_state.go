package workflow

import (
	"errors"
	"fmt"
)

// ErrInvalidRunState identifies text outside the persisted run-state domain.
var ErrInvalidRunState = errors.New("workflow: invalid run state")

// Valid reports whether state is one of the values accepted by runs.state.
func (state RunState) Valid() bool {
	switch state {
	case RunStatePending,
		RunStateRunning,
		RunStateAwaitingHITL,
		RunStateSucceeded,
		RunStateFailed,
		RunStateCancelled:
		return true
	default:
		return false
	}
}

// ParseRunState validates one persisted run-state spelling.
func ParseRunState(value string) (RunState, error) {
	state := RunState(value)
	if !state.Valid() {
		return "", fmt.Errorf("%w %q", ErrInvalidRunState, value)
	}
	return state, nil
}

// ParseRunStateFilter preserves the empty any-state filter and otherwise
// delegates to the persisted run-state parser.
func ParseRunStateFilter(value string) (RunState, error) {
	if value == "" {
		return "", nil
	}
	return ParseRunState(value)
}
