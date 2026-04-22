package reservation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tempStorePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "reservations.json")
}

func TestReserve(t *testing.T) {
	store := NewStore(tempStorePath(t))

	created, err := store.Reserve("session-1", []string{"pkg/dag/*.go", "pkg/dag/types.go"}, DefaultTTL)
	require.NoError(t, err)
	assert.Len(t, created, 2)
	assert.Equal(t, "session-1", created[0].SessionID)
	assert.Equal(t, "pkg/dag/*.go", created[0].Pattern)
	assert.Equal(t, "pkg/dag/types.go", created[1].Pattern)
	assert.False(t, created[0].IsExpired())
}

func TestReserveMultipleSessions(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("session-1", []string{"pkg/dag/*.go"}, DefaultTTL)
	require.NoError(t, err)

	_, err = store.Reserve("session-2", []string{"pkg/api/*.go"}, DefaultTTL)
	require.NoError(t, err)

	all, err := store.List("")
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestCheckReservation(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("session-1", []string{"pkg/dag/*.go"}, DefaultTTL)
	require.NoError(t, err)

	// Check from a different session — should be reserved
	result, err := store.Check("pkg/dag/resolver.go", "session-2")
	require.NoError(t, err)
	assert.True(t, result.Reserved)
	assert.Equal(t, "session-1", result.Owner)
	assert.Equal(t, "pkg/dag/*.go", result.Pattern)

	// Check from the owning session — should NOT be reserved (own reservation)
	result, err = store.Check("pkg/dag/resolver.go", "session-1")
	require.NoError(t, err)
	assert.False(t, result.Reserved)

	// Check a non-matching path — should not be reserved
	result, err = store.Check("pkg/api/server.go", "session-2")
	require.NoError(t, err)
	assert.False(t, result.Reserved)
}

func TestCheckExactPath(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("session-1", []string{"main.go"}, DefaultTTL)
	require.NoError(t, err)

	result, err := store.Check("main.go", "session-2")
	require.NoError(t, err)
	assert.True(t, result.Reserved)
	assert.Equal(t, "session-1", result.Owner)

	result, err = store.Check("other.go", "session-2")
	require.NoError(t, err)
	assert.False(t, result.Reserved)
}

func TestRelease(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("session-1", []string{"pkg/dag/*.go", "main.go"}, DefaultTTL)
	require.NoError(t, err)

	_, err = store.Reserve("session-2", []string{"pkg/api/*.go"}, DefaultTTL)
	require.NoError(t, err)

	// Release session-1
	released, err := store.Release("session-1")
	require.NoError(t, err)
	assert.Equal(t, 2, released)

	// Verify session-1's reservations are gone
	all, err := store.List("")
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "session-2", all[0].SessionID)
}

func TestReleaseNonexistent(t *testing.T) {
	store := NewStore(tempStorePath(t))

	released, err := store.Release("nonexistent")
	require.NoError(t, err)
	assert.Equal(t, 0, released)
}

func TestListAll(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("s1", []string{"a.go"}, DefaultTTL)
	require.NoError(t, err)
	_, err = store.Reserve("s2", []string{"b.go", "c.go"}, DefaultTTL)
	require.NoError(t, err)

	all, err := store.List("")
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestListBySession(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("s1", []string{"a.go"}, DefaultTTL)
	require.NoError(t, err)
	_, err = store.Reserve("s2", []string{"b.go", "c.go"}, DefaultTTL)
	require.NoError(t, err)

	s2Only, err := store.List("s2")
	require.NoError(t, err)
	assert.Len(t, s2Only, 2)
	for _, r := range s2Only {
		assert.Equal(t, "s2", r.SessionID)
	}
}

func TestExpiredReservationsCleanedUp(t *testing.T) {
	store := NewStore(tempStorePath(t))

	// Create a reservation with a very short TTL
	_, err := store.Reserve("session-1", []string{"old.go"}, 1*time.Millisecond)
	require.NoError(t, err)

	// Wait for it to expire
	time.Sleep(5 * time.Millisecond)

	// Create a new reservation — this triggers cleanup
	_, err = store.Reserve("session-2", []string{"new.go"}, DefaultTTL)
	require.NoError(t, err)

	// Only the new reservation should remain
	all, err := store.List("")
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "session-2", all[0].SessionID)
}

func TestCleanup(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("s1", []string{"a.go"}, 1*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	removed, err := store.Cleanup()
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	all, err := store.List("")
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestCleanupNoExpired(t *testing.T) {
	store := NewStore(tempStorePath(t))

	_, err := store.Reserve("s1", []string{"a.go"}, DefaultTTL)
	require.NoError(t, err)

	removed, err := store.Cleanup()
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

func TestEmptyStore(t *testing.T) {
	store := NewStore(tempStorePath(t))

	// List on empty store
	all, err := store.List("")
	require.NoError(t, err)
	assert.Empty(t, all)

	// Check on empty store
	result, err := store.Check("anything.go", "session-1")
	require.NoError(t, err)
	assert.False(t, result.Reserved)
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// Exact matches
		{"main.go", "main.go", true},
		{"main.go", "other.go", false},

		// Glob patterns
		{"*.go", "main.go", true},
		{"*.go", "main.rs", false},
		{"pkg/dag/*.go", "pkg/dag/resolver.go", true},
		{"pkg/dag/*.go", "pkg/dag/types.go", true},
		{"pkg/dag/*.go", "pkg/api/server.go", false},

		// Single-char wildcard
		{"?.go", "a.go", true},
		{"?.go", "ab.go", false},

		// Character classes
		{"[abc].go", "a.go", true},
		{"[abc].go", "d.go", false},

		// No recursive glob (filepath.Match limitation)
		{"**/*.go", "pkg/dag/resolver.go", false}, // filepath.Match doesn't support **
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchesPattern(tt.pattern, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStoreFileCreatedOnReserve(t *testing.T) {
	path := tempStorePath(t)
	store := NewStore(path)

	_, err := store.Reserve("s1", []string{"a.go"}, DefaultTTL)
	require.NoError(t, err)

	// File should exist now
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestStoreFilePermissions(t *testing.T) {
	path := tempStorePath(t)
	store := NewStore(path)

	_, err := store.Reserve("s1", []string{"a.go"}, DefaultTTL)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	// File should be owner-only read/write (0600)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestDefaultStorePath(t *testing.T) {
	path := DefaultStorePath()
	assert.Contains(t, path, ".agm")
	assert.Contains(t, path, "reservations.json")
}

func TestReservationIsExpired(t *testing.T) {
	r := Reservation{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	assert.True(t, r.IsExpired())

	r.ExpiresAt = time.Now().Add(1 * time.Hour)
	assert.False(t, r.IsExpired())
}

func TestCheckExpiredReservationIgnored(t *testing.T) {
	store := NewStore(tempStorePath(t))

	// Create expired reservation
	_, err := store.Reserve("session-1", []string{"important.go"}, 1*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	// Check should not find expired reservation
	result, err := store.Check("important.go", "session-2")
	require.NoError(t, err)
	assert.False(t, result.Reserved)
}

func TestConcurrentReserveAndCheck(t *testing.T) {
	store := NewStore(tempStorePath(t))

	// Simulate concurrent access (within same process via goroutines)
	done := make(chan error, 10)

	for i := range 5 {
		sessionID := "session-" + string(rune('a'+i))
		go func() {
			_, err := store.Reserve(sessionID, []string{sessionID + ".go"}, DefaultTTL)
			done <- err
		}()
	}

	for i := range 5 {
		path := "session-" + string(rune('a'+i)) + ".go"
		go func() {
			_, err := store.Check(path, "other")
			done <- err
		}()
	}

	for range 10 {
		err := <-done
		assert.NoError(t, err)
	}
}
