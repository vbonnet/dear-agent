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

// ErrRepeatedRunState identifies a run-state filter supplied more than once.
var ErrRepeatedRunState = errors.New("workflow: repeated state filter")

// ParseRunStateFilterValues validates a run-state filter carried by a
// transport that can repeat a key, such as a URL query. Reading only the
// first value would make validation order-dependent: "?state=running&
// state=typo" would silently ignore the unknown spelling while the reverse
// order rejected it. A repeated filter is ambiguous rather than additive, so
// it is refused outright.
func ParseRunStateFilterValues(values []string) (RunState, error) {
	switch len(values) {
	case 0:
		return "", nil
	case 1:
		return ParseRunStateFilter(values[0])
	default:
		return "", fmt.Errorf("%w: %d values", ErrRepeatedRunState, len(values))
	}
}
