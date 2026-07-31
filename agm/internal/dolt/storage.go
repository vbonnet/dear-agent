package dolt

import (
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/artifacts"
	"github.com/vbonnet/dear-agent/agm/internal/logs"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Storage is the interface for session storage backends.
// It embeds manifest.Store and retains legacy session methods.
// Both Adapter (MySQL/Dolt) and MockAdapter (in-memory) implement this interface.
type Storage interface {
	manifest.Store

	// CreateSession inserts a new session (legacy — delegates to Create).
	CreateSession(session *manifest.Manifest) error

	// GetSession retrieves a session by ID (legacy — delegates to Get).
	GetSession(sessionID string) (*manifest.Manifest, error)

	// UpdateSession updates an existing session (legacy — delegates to Update).
	UpdateSession(session *manifest.Manifest) error

	// DeleteSession removes a session (legacy — delegates to Delete).
	DeleteSession(sessionID string) error

	// ListSessions returns sessions matching the SessionFilter.
	ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error)

	// GetSessionByUUID returns the session whose Claude conversation UUID matches
	// conversationUUID, or (nil, nil) if no session tracks that UUID. Used by
	// agm session get to resolve Claude session UUIDs passed from Stop hooks.
	GetSessionByUUID(conversationUUID string) (*manifest.Manifest, error)

	// RecordHarnessSwitch appends a harness-switch event for a session.
	RecordHarnessSwitch(sessionID, fromHarness, toHarness string, switchedAt time.Time) error

	// GetHarnessHistory returns all harness-switch events for a session in chronological order.
	GetHarnessHistory(sessionID string) ([]manifest.HarnessSwitch, error)
}

// SessionNameReservationStore serializes active session-name claims across
// processes before any harness launch side effect occurs.
type SessionNameReservationStore interface {
	ReserveSessionName(sessionID, name string) error
	RenewSessionNameReservation(sessionID, name string) error
	ReleaseSessionNameReservation(sessionID string) error
}

// Verify that Adapter implements manifest.Store at compile time.
var _ manifest.Store = (*Adapter)(nil)

// Verify that MockAdapter implements manifest.Store at compile time.
var _ manifest.Store = (*MockAdapter)(nil)

// Verify that both types implement the full Storage interface.
var _ Storage = (*Adapter)(nil)
var _ Storage = (*MockAdapter)(nil)
var _ SessionNameReservationStore = (*Adapter)(nil)
var _ SessionNameReservationStore = (*MockAdapter)(nil)

// Verify that LogAdapter implements logs.Store at compile time.
var _ logs.Store = (*LogAdapter)(nil)

// Verify that ArtifactAdapter implements artifacts.Store at compile time.
var _ artifacts.Store = (*ArtifactAdapter)(nil)
