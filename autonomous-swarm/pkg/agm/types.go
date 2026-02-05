package csm

import "time"

// SessionState represents the state of a CSM session
type SessionState string

const (
	SessionActive   SessionState = "active"
	SessionStopped  SessionState = "stopped"
	SessionArchived SessionState = "archived"
)

// SessionMetadata contains metadata about a CSM session
type SessionMetadata struct {
	Name      string       `json:"name"`
	UUID      string       `json:"uuid"`
	State     SessionState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	BeadID    string       `json:"bead_id"`
}
