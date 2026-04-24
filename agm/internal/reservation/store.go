// Package reservation provides advisory file reservations for parallel agent safety.
//
// When multiple agents work in parallel (swarm mode), they can destructively
// interfere by editing the same files. This package lets agents declare intent
// to edit files/paths so other agents can check before editing.
//
// Reservations are stored in a JSON file (~/.agm/reservations.json) and use
// atomic writes for concurrent safety. Expired reservations are automatically
// cleaned up on every store operation.
package reservation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultTTL is the default reservation time-to-live (2 hours).
const DefaultTTL = 2 * time.Hour

// DefaultStorePath returns the default path for the reservations file.
func DefaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".agm", "reservations.json")
	}
	return filepath.Join(home, ".agm", "reservations.json")
}

// Reservation represents a single file/path reservation by a session.
type Reservation struct {
	// SessionID is the identifier of the session holding this reservation.
	SessionID string `json:"session_id"`

	// Pattern is the file path or glob pattern reserved (e.g. "pkg/dag/*.go").
	Pattern string `json:"pattern"`

	// CreatedAt is when the reservation was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the reservation expires.
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired returns true if the reservation has expired.
func (r *Reservation) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// storeData is the on-disk JSON structure.
type storeData struct {
	Reservations []Reservation `json:"reservations"`
}

// Store manages advisory file reservations backed by a JSON file.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore creates a new reservation store at the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Reserve claims one or more file path patterns for a session.
// Patterns can be exact paths or glob patterns (e.g. "pkg/dag/*.go").
// TTL controls how long the reservation lasts before auto-expiring.
func (s *Store) Reserve(sessionID string, patterns []string, ttl time.Duration) ([]Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, fmt.Errorf("loading reservations: %w", err)
	}

	// Clean expired entries
	data.Reservations = filterActive(data.Reservations)

	now := time.Now()
	var created []Reservation

	for _, pattern := range patterns {
		r := Reservation{
			SessionID: sessionID,
			Pattern:   pattern,
			CreatedAt: now,
			ExpiresAt: now.Add(ttl),
		}
		data.Reservations = append(data.Reservations, r)
		created = append(created, r)
	}

	if err := s.save(data); err != nil {
		return nil, fmt.Errorf("saving reservations: %w", err)
	}

	return created, nil
}

// CheckResult holds the result of checking a path against reservations.
type CheckResult struct {
	// Reserved is true if the path is reserved by another session.
	Reserved bool `json:"reserved"`

	// Owner is the session ID of the reservation holder (empty if not reserved).
	Owner string `json:"owner,omitempty"`

	// Pattern is the matching reservation pattern (empty if not reserved).
	Pattern string `json:"pattern,omitempty"`

	// ExpiresAt is when the reservation expires (zero if not reserved).
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// Check tests whether a specific file path is reserved by any session other
// than the given session. Returns the reservation owner info if reserved.
func (s *Store) Check(path string, currentSession string) (*CheckResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, fmt.Errorf("loading reservations: %w", err)
	}

	// Clean expired entries
	data.Reservations = filterActive(data.Reservations)

	for _, r := range data.Reservations {
		// Skip reservations by the current session
		if r.SessionID == currentSession {
			continue
		}

		if matchesPattern(r.Pattern, path) {
			return &CheckResult{
				Reserved:  true,
				Owner:     r.SessionID,
				Pattern:   r.Pattern,
				ExpiresAt: r.ExpiresAt,
			}, nil
		}
	}

	return &CheckResult{Reserved: false}, nil
}

// Release removes all reservations for a given session.
func (s *Store) Release(sessionID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return 0, fmt.Errorf("loading reservations: %w", err)
	}

	// Count and remove reservations for this session
	original := len(data.Reservations)
	filtered := make([]Reservation, 0, len(data.Reservations))
	for _, r := range data.Reservations {
		if r.SessionID != sessionID {
			filtered = append(filtered, r)
		}
	}
	data.Reservations = filtered
	released := original - len(filtered)

	// Also clean expired
	data.Reservations = filterActive(data.Reservations)

	if err := s.save(data); err != nil {
		return 0, fmt.Errorf("saving reservations: %w", err)
	}

	return released, nil
}

// List returns all active (non-expired) reservations. If sessionID is non-empty,
// only reservations for that session are returned.
func (s *Store) List(sessionID string) ([]Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, fmt.Errorf("loading reservations: %w", err)
	}

	active := filterActive(data.Reservations)

	if sessionID == "" {
		return active, nil
	}

	var result []Reservation
	for _, r := range active {
		if r.SessionID == sessionID {
			result = append(result, r)
		}
	}
	return result, nil
}

// Cleanup removes all expired reservations and returns the count removed.
func (s *Store) Cleanup() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return 0, fmt.Errorf("loading reservations: %w", err)
	}

	original := len(data.Reservations)
	data.Reservations = filterActive(data.Reservations)
	removed := original - len(data.Reservations)

	if removed > 0 {
		if err := s.save(data); err != nil {
			return 0, fmt.Errorf("saving reservations: %w", err)
		}
	}

	return removed, nil
}

// load reads the store data from disk. Returns empty data if file doesn't exist.
func (s *Store) load() (*storeData, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storeData{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}

	var data storeData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.path, err)
	}

	return &data, nil
}

// save writes the store data to disk atomically using temp file + rename.
func (s *Store) save(data *storeData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling reservations: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Atomic write: temp file + rename
	tmpFile, err := os.CreateTemp(dir, ".reservations-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Cleanup on error
	success := false
	defer func() {
		if !success {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(raw); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}

	success = true
	return nil
}

// filterActive returns only non-expired reservations.
func filterActive(reservations []Reservation) []Reservation {
	now := time.Now()
	active := make([]Reservation, 0, len(reservations))
	for _, r := range reservations {
		if now.Before(r.ExpiresAt) {
			active = append(active, r)
		}
	}
	return active
}

// matchesPattern checks if a file path matches a reservation pattern.
// The pattern can be an exact path or a glob pattern.
func matchesPattern(pattern, path string) bool {
	// Try exact match first
	if pattern == path {
		return true
	}

	// Try glob match
	matched, err := filepath.Match(pattern, path)
	if err != nil {
		// Invalid pattern, fall back to exact match only
		return false
	}

	return matched
}
