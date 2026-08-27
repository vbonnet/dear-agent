package dolt

import (
	"errors"
	"fmt"
)

// ErrSessionNotFound is the storage-layer sentinel for an exact session ID
// miss. Callers must use errors.Is rather than interpreting arbitrary backend
// failures or matching error text as absence.
var ErrSessionNotFound = errors.New("session not found")

func sessionNotFoundError(identifier string) error {
	if identifier == "" {
		return ErrSessionNotFound
	}
	return fmt.Errorf("%w: %s", ErrSessionNotFound, identifier)
}
