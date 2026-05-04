package ui

import (
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Session wraps a manifest with computed status for UI display
type Session struct {
	*manifest.Manifest
	Status    string    // "active", "stopped", or "archived"
	UpdatedAt time.Time // Cached from manifest
}

// SessionSelector abstracts session selection for testability
type SessionSelector interface {
	SelectSession(sessions []*Session) (*Session, error)
}

// NewSessionFormData holds fields collected from the new-session form.
type NewSessionFormData struct {
	Name    string
	Project string
	Purpose string
	Tags    []string
}

// FormProvider abstracts form interactions
type FormProvider interface {
	NewSessionForm() (*NewSessionFormData, error)
}

// Confirmer abstracts confirmation prompts
type Confirmer interface {
	ConfirmCreate(name, project string) (bool, error)
	ConfirmCleanup(toArchive, toDelete []string) (bool, error)
}

// CleanupResult represents multi-select cleanup results
type CleanupResult struct {
	ToArchive []*Session
	ToDelete  []*Session
}
