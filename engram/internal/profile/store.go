package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sentinel errors returned by Store.
var (
	// ErrInvalidID indicates an agent or session id contains unsafe characters.
	ErrInvalidID = errors.New("invalid id")
	// ErrStaticExists indicates a static profile already exists and may not be
	// overwritten (static profiles are immutable once spawned).
	ErrStaticExists = errors.New("static profile already exists")
	// ErrNotFound indicates the requested profile does not exist on disk.
	ErrNotFound = errors.New("profile not found")
)

const (
	staticFileName    = "static.json"
	dynamicFilePrefix = "dynamic-"
	dynamicFileSuffix = ".json"
)

// Store persists agent profiles to the filesystem. The on-disk layout is:
//
//	<baseDir>/<agentID>/static.json
//	<baseDir>/<agentID>/dynamic-<sessionID>.json
//
// Static profiles are write-once; dynamic profiles are upserted. The store is
// safe for concurrent use.
type Store struct {
	baseDir string
	mu      sync.RWMutex
	// now is injectable for deterministic tests; defaults to time.Now.
	now func() time.Time
}

// NewStore creates a Store rooted at baseDir. If baseDir is empty it defaults
// to ~/.engram/profiles.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".engram", "profiles")
	}
	return &Store{baseDir: baseDir, now: time.Now}
}

// agentDir returns the directory for an agent, validating the id first.
func (s *Store) agentDir(agentID string) (string, error) {
	if !validID(agentID) {
		return "", fmt.Errorf("%w: agent_id %q", ErrInvalidID, agentID)
	}
	return filepath.Join(s.baseDir, agentID), nil
}

// SaveStatic persists a static profile. It is write-once: if a static profile
// already exists for the agent, it returns ErrStaticExists and leaves the
// existing profile untouched. SpawnedAt is set to now when zero.
func (s *Store) SaveStatic(p StaticProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	dir, err := s.agentDir(p.AgentID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(dir, staticFileName)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: agent %q", ErrStaticExists, p.AgentID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat static profile: %w", err)
	}

	if p.SpawnedAt.IsZero() {
		p.SpawnedAt = s.now()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	return writeJSON(path, p)
}

// LoadStatic reads the static profile for an agent. Returns ErrNotFound if it
// does not exist.
func (s *Store) LoadStatic(agentID string) (StaticProfile, error) {
	dir, err := s.agentDir(agentID)
	if err != nil {
		return StaticProfile{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var p StaticProfile
	if err := readJSON(filepath.Join(dir, staticFileName), &p); err != nil {
		return StaticProfile{}, err
	}
	return p, nil
}

// HasStatic reports whether a static profile exists for the agent.
func (s *Store) HasStatic(agentID string) bool {
	dir, err := s.agentDir(agentID)
	if err != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err = os.Stat(filepath.Join(dir, staticFileName))
	return err == nil
}

func dynamicFileName(sessionID string) string {
	return dynamicFilePrefix + sessionID + dynamicFileSuffix
}

// SaveDynamic upserts a dynamic profile, stamping UpdatedAt with now. Unlike
// SaveStatic it overwrites any existing profile for the same agent + session.
func (s *Store) SaveDynamic(p DynamicProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	dir, err := s.agentDir(p.AgentID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	p.UpdatedAt = s.now()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	return writeJSON(filepath.Join(dir, dynamicFileName(p.SessionID)), p)
}

// LoadDynamic reads the dynamic profile for an agent + session. Returns
// ErrNotFound if it does not exist.
func (s *Store) LoadDynamic(agentID, sessionID string) (DynamicProfile, error) {
	if !validID(sessionID) {
		return DynamicProfile{}, fmt.Errorf("%w: session_id %q", ErrInvalidID, sessionID)
	}
	dir, err := s.agentDir(agentID)
	if err != nil {
		return DynamicProfile{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var p DynamicProfile
	if err := readJSON(filepath.Join(dir, dynamicFileName(sessionID)), &p); err != nil {
		return DynamicProfile{}, err
	}
	return p, nil
}

// UpdateDynamic applies mutate to the dynamic profile for an agent + session
// and persists the result, returning the saved profile. If no dynamic profile
// exists yet, a fresh one (with AgentID and SessionID populated) is created and
// passed to mutate. The read-modify-write is atomic with respect to other Store
// operations. This is the primary way a running worker records progress.
func (s *Store) UpdateDynamic(agentID, sessionID string, mutate func(*DynamicProfile)) (DynamicProfile, error) {
	if !validID(sessionID) {
		return DynamicProfile{}, fmt.Errorf("%w: session_id %q", ErrInvalidID, sessionID)
	}
	dir, err := s.agentDir(agentID)
	if err != nil {
		return DynamicProfile{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(dir, dynamicFileName(sessionID))
	p := DynamicProfile{AgentID: agentID, SessionID: sessionID}
	if err := readJSON(path, &p); err != nil && !errors.Is(err, ErrNotFound) {
		return DynamicProfile{}, err
	}
	// Guard against a corrupt file losing the identity keys.
	p.AgentID = agentID
	p.SessionID = sessionID

	if mutate != nil {
		mutate(&p)
	}
	p.UpdatedAt = s.now()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return DynamicProfile{}, fmt.Errorf("create agent dir: %w", err)
	}
	if err := writeJSON(path, p); err != nil {
		return DynamicProfile{}, err
	}
	return p, nil
}

// RecordDiscovery appends a discovery to the dynamic profile for an agent +
// session, creating the profile if needed. It is a convenience wrapper over
// UpdateDynamic.
func (s *Store) RecordDiscovery(agentID, sessionID, summary, detail string) (DynamicProfile, error) {
	return s.UpdateDynamic(agentID, sessionID, func(p *DynamicProfile) {
		p.Discoveries = append(p.Discoveries, Discovery{
			Timestamp: s.now(),
			Summary:   summary,
			Detail:    detail,
		})
	})
}

// SetTaskFocus updates the current task focus on the dynamic profile,
// creating it if needed. It is a convenience wrapper over UpdateDynamic.
func (s *Store) SetTaskFocus(agentID, sessionID, focus string) (DynamicProfile, error) {
	return s.UpdateDynamic(agentID, sessionID, func(p *DynamicProfile) {
		p.TaskFocus = focus
	})
}

// LoadProfile loads both tiers for an agent + session into an AgentProfile —
// the unit used for handoff and resumption. The static profile must exist; the
// dynamic profile may be absent, in which case an empty (but keyed) dynamic
// profile is returned.
func (s *Store) LoadProfile(agentID, sessionID string) (AgentProfile, error) {
	static, err := s.LoadStatic(agentID)
	if err != nil {
		return AgentProfile{}, err
	}
	dynamic, err := s.LoadDynamic(agentID, sessionID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return AgentProfile{}, err
		}
		dynamic = DynamicProfile{AgentID: agentID, SessionID: sessionID}
	}
	return AgentProfile{Static: static, Dynamic: dynamic}, nil
}

// ListSessions returns the session ids that have a dynamic profile for the
// agent, sorted. Returns an empty slice (not an error) if the agent has none.
func (s *Store) ListSessions(agentID string) ([]string, error) {
	dir, err := s.agentDir(agentID)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read agent dir: %w", err)
	}

	sessions := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, dynamicFilePrefix) || !strings.HasSuffix(name, dynamicFileSuffix) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, dynamicFilePrefix), dynamicFileSuffix)
		sessions = append(sessions, id)
	}
	sort.Strings(sessions)
	return sessions, nil
}

// writeJSON atomically writes v as indented JSON to path via a temp file +
// rename so a crash mid-write cannot leave a partial profile.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// readJSON reads JSON from path into v, mapping a missing file to ErrNotFound.
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("read profile: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal profile: %w", err)
	}
	return nil
}
