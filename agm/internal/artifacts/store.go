// Package artifacts provides artifacts-related functionality.
package artifacts

import "time"

// Store is the interface for session artifact persistence.
type Store interface {
	// Store saves artifact metadata.
	Store(artifact *Artifact) error

	// Get retrieves an artifact by ID.
	Get(id string) (*Artifact, error)

	// ListBySession retrieves all artifacts for a session.
	ListBySession(sessionID string) ([]*Artifact, error)

	// Delete removes an artifact by ID.
	Delete(id string) error
}

// Artifact represents a file or document produced by a session.
type Artifact struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"session_id"`
	Type      string                 `json:"type"`       // "research-report", "code-review", etc.
	Path      string                 `json:"path"`       // filesystem path
	Size      int64                  `json:"size"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}
