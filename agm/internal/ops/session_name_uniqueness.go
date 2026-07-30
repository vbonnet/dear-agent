package ops

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// SessionNameExistsError reports a durable non-archived session-name collision.
// Its message preserves the CLI duplicate-name UX while giving ops callers a
// typed error they can convert into their structured problem response.
type SessionNameExistsError struct {
	Name string
}

func (e *SessionNameExistsError) Error() string {
	return sessionNameExistsMessage(e.Name)
}

func sessionNameExistsMessage(name string) string {
	return fmt.Sprintf("session '%s' already exists. Use a different name or archive the existing session with: agm session archive %s", name, name)
}

// EnsureNonArchivedSessionNameAvailable rejects a session name already held by
// a non-archived Dolt session record.
func EnsureNonArchivedSessionNameAvailable(store dolt.Storage, sessionName string) error {
	if store == nil || sessionName == "" {
		return nil
	}
	sessions, err := store.ListSessions(&dolt.SessionFilter{ExcludeArchived: true})
	if err != nil {
		return fmt.Errorf("list non-archived sessions: %w", err)
	}
	for _, s := range sessions {
		if s != nil && s.Name == sessionName {
			return &SessionNameExistsError{Name: sessionName}
		}
	}
	return nil
}
