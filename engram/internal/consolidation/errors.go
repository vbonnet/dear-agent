package consolidation

import "errors"

var (
	// ErrNotFound is returned when a requested memory/session/artifact does not exist.
	ErrNotFound = errors.New("not found")

	// ErrInvalidNamespace is returned for malformed namespace paths.
	// Namespaces must not be empty, contain empty parts, or include path traversal (..).
	ErrInvalidNamespace = errors.New("invalid namespace")

	// ErrInvalidConfig is returned when provider configuration is invalid or incomplete.
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrProviderNotFound is returned when requested provider type is not registered.
	ErrProviderNotFound = errors.New("provider not found")

	// ErrSessionActive is returned when trying to persist an active session.
	// Sessions must be completed before persistence.
	ErrSessionActive = errors.New("session is still active")

	// ErrAccessDenied is returned when user lacks permission for namespace.
	// Currently used as a placeholder for future multi-user support (v1.0.0+).
	ErrAccessDenied = errors.New("access denied")
)
