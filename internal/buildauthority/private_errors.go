package buildauthority

import (
	"errors"
	"fmt"
)

// privateFailure carries raw diagnostic context only inside the package. Stage
// reduces it to its fixed cause codes and never returns or unwraps it.
type privateFailure struct {
	causes []CauseCode
	raw    error
}

func (failure *privateFailure) Error() string {
	if failure == nil || failure.raw == nil {
		return "buildauthority private failure"
	}
	return failure.raw.Error()
}

func (failure *privateFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.raw
}

func fail(cause CauseCode, message string) error {
	return &privateFailure{
		causes: []CauseCode{cause},
		raw:    errors.New(message),
	}
}

func failWith(err error, cause CauseCode, message string) error {
	raw := errors.New(message)
	if err != nil {
		raw = fmt.Errorf("%s: %w", message, err)
	}
	return &privateFailure{causes: []CauseCode{cause}, raw: raw}
}

func privateCauses(err error, fallback CauseCode) []CauseCode {
	causes := make([]CauseCode, 0, 1)
	var collect func(error)
	collect = func(current error) {
		if current == nil {
			return
		}
		// Inspect this exact node: errors.As would stop at the first branch of an
		// errors.Join tree and lose later sanitized causes.
		if failure, ok := current.(*privateFailure); ok { //nolint:errorlint // Exact-node inspection prevents errors.Join from hiding later branches.
			causes = append(causes, failure.causes...)
			return
		}
		// Walk each exact unwrap edge ourselves for the same multi-error reason.
		switch wrapped := current.(type) { //nolint:errorlint // Manual traversal visits every errors.Join branch in deterministic order.
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				collect(child)
			}
		case interface{ Unwrap() error }:
			collect(wrapped.Unwrap())
		}
	}
	collect(err)
	if len(causes) != 0 {
		return causes
	}
	return []CauseCode{fallback}
}
