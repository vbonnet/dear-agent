package dolt

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// mockAccessData tracks frecency fields for a session in the mock adapter.
type mockAccessData struct {
	accessCount    int
	lastAccessedAt *time.Time
}

// MockAdapter is an in-memory implementation of the Adapter interface for testing
type MockAdapter struct {
	mu       sync.RWMutex
	sessions map[string]*manifest.Manifest
	access   map[string]*mockAccessData
	closed   bool
}

// NewMockAdapter creates a new in-memory mock adapter for testing
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		sessions: make(map[string]*manifest.Manifest),
		access:   make(map[string]*mockAccessData),
	}
}

// CreateSession stores a new session in memory
func (m *MockAdapter) CreateSession(session *manifest.Manifest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("adapter is closed")
	}

	if session.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	// Check for duplicate
	if _, exists := m.sessions[session.SessionID]; exists {
		return fmt.Errorf("session already exists: %s", session.SessionID)
	}

	// Store a deep copy to prevent external modifications
	m.sessions[session.SessionID] = m.copyManifest(session)
	return nil
}

// GetSession retrieves a session by ID from memory
func (m *MockAdapter) GetSession(sessionID string) (*manifest.Manifest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("adapter is closed")
	}

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Return a deep copy to prevent external modifications
	return m.copyManifest(session), nil
}

// UpdateSession updates an existing session in memory
func (m *MockAdapter) UpdateSession(session *manifest.Manifest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("adapter is closed")
	}

	if session.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	// Check session exists
	if _, exists := m.sessions[session.SessionID]; !exists {
		return fmt.Errorf("session not found: %s", session.SessionID)
	}

	// Update UpdatedAt timestamp
	session.UpdatedAt = time.Now()

	// Store a deep copy
	m.sessions[session.SessionID] = m.copyManifest(session)
	return nil
}

// DeleteSession removes a session from memory
func (m *MockAdapter) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("adapter is closed")
	}

	if _, exists := m.sessions[sessionID]; !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	delete(m.sessions, sessionID)
	return nil
}

// ListSessions returns sessions matching the filter
func (m *MockAdapter) ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("adapter is closed")
	}

	var results []*manifest.Manifest

	for _, session := range m.sessions {
		// Apply filters
		if filter != nil {
			// ExcludeArchived takes precedence over Lifecycle filter
			if filter.ExcludeArchived && session.Lifecycle == manifest.LifecycleArchived {
				continue
			}

			// Lifecycle filter
			if filter.Lifecycle != "" && session.Lifecycle != filter.Lifecycle {
				continue
			}

			// Harness filter
			if filter.Harness != "" && session.Harness != filter.Harness {
				continue
			}

			// ParentSessionID filter (pointer comparison)
			if filter.ParentSessionID != nil {
				// Skip sessions without parent_session_id support
				// TODO: Add ParentSessionID field to manifest.Manifest if needed
				continue
			}

			// Workspace filter
			if filter.Workspace != "" && session.Workspace != filter.Workspace {
				continue
			}

			// Tag filter: all specified tags must be present
			if len(filter.Tags) > 0 {
				tagSet := make(map[string]bool, len(session.Context.Tags))
				for _, t := range session.Context.Tags {
					tagSet[t] = true
				}
				allMatch := true
				for _, t := range filter.Tags {
					if !tagSet[t] {
						allMatch = false
						break
					}
				}
				if !allMatch {
					continue
				}
			}
		}

		// Add matching session (deep copy)
		results = append(results, m.copyManifest(session))
	}

	// Apply limit and offset
	if filter != nil {
		if filter.Offset > 0 {
			if filter.Offset >= len(results) {
				return []*manifest.Manifest{}, nil
			}
			results = results[filter.Offset:]
		}

		if filter.Limit > 0 && filter.Limit < len(results) {
			results = results[:filter.Limit]
		}
	}

	return results, nil
}

// Close marks the adapter as closed
func (m *MockAdapter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// ApplyMigrations is a no-op for the mock adapter
func (m *MockAdapter) ApplyMigrations() error {
	return nil
}

// Reset clears all sessions (for testing)
func (m *MockAdapter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]*manifest.Manifest)
	m.access = make(map[string]*mockAccessData)
	m.closed = false
}

// Count returns the number of sessions in memory (for testing)
func (m *MockAdapter) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// copyManifest creates a deep copy of a manifest
func (m *MockAdapter) copyManifest(src *manifest.Manifest) *manifest.Manifest {
	if src == nil {
		return nil
	}

	dst := &manifest.Manifest{
		SchemaVersion:  src.SchemaVersion,
		SessionID:      src.SessionID,
		Name:           src.Name,
		Harness:        src.Harness,
		Model:          src.Model,
		Workspace:      src.Workspace,
		CreatedAt:      src.CreatedAt,
		UpdatedAt:      src.UpdatedAt,
		Lifecycle:      src.Lifecycle,
		State:          src.State,
		StateUpdatedAt: src.StateUpdatedAt,
		StateSource:    src.StateSource,
		Context: manifest.Context{
			Project: src.Context.Project,
			Purpose: src.Context.Purpose,
			Tags:    append([]string{}, src.Context.Tags...),
			Notes:   src.Context.Notes,
		},
		Tmux: manifest.Tmux{
			SessionName: src.Tmux.SessionName,
		},
		Claude: manifest.Claude{
			UUID: src.Claude.UUID,
		},
	}

	// Copy OpenCode if present
	if src.OpenCode != nil {
		dst.OpenCode = &manifest.OpenCode{
			ServerPort: src.OpenCode.ServerPort,
			ServerHost: src.OpenCode.ServerHost,
			AttachTime: src.OpenCode.AttachTime,
		}
	}

	// Copy EngramMetadata if present
	if src.EngramMetadata != nil {
		dst.EngramMetadata = &manifest.EngramMetadata{
			Enabled:   src.EngramMetadata.Enabled,
			Query:     src.EngramMetadata.Query,
			EngramIDs: append([]string{}, src.EngramMetadata.EngramIDs...),
			LoadedAt:  src.EngramMetadata.LoadedAt,
			Count:     src.EngramMetadata.Count,
		}
	}

	// Copy Disposable fields
	dst.Disposable = src.Disposable
	dst.DisposableTTL = src.DisposableTTL

	// Copy ContextUsage if present
	if src.ContextUsage != nil {
		dst.ContextUsage = &manifest.ContextUsage{
			TotalTokens:    src.ContextUsage.TotalTokens,
			UsedTokens:     src.ContextUsage.UsedTokens,
			PercentageUsed: src.ContextUsage.PercentageUsed,
			LastUpdated:    src.ContextUsage.LastUpdated,
			Source:         src.ContextUsage.Source,
		}
	}

	return dst
}

// UpdateAccess increments the access count and sets last_accessed_at for a session.
func (m *MockAdapter) UpdateAccess(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("adapter is closed")
	}

	if _, exists := m.sessions[sessionID]; !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ad, exists := m.access[sessionID]
	if !exists {
		ad = &mockAccessData{}
		m.access[sessionID] = ad
	}
	ad.accessCount++
	now := time.Now()
	ad.lastAccessedAt = &now

	return nil
}

// GetByFrecency returns non-archived sessions ranked by frecency score.
func (m *MockAdapter) GetByFrecency(limit int) ([]FrecencyResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("adapter is closed")
	}

	now := time.Now()
	var results []FrecencyResult

	for id, session := range m.sessions {
		if session.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		var count int
		var lastAccess *time.Time
		if ad, ok := m.access[id]; ok {
			count = ad.accessCount
			lastAccess = ad.lastAccessedAt
		}

		score := FrecencyScore(count, lastAccess, now)
		results = append(results, FrecencyResult{
			Session: m.copyManifest(session),
			Score:   score,
		})
	}

	sortFrecencyResults(results)

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

// NewTestManifest creates a session manifest populated with sensible test defaults.
func NewTestManifest(sessionID string, name string) *manifest.Manifest {
	now := time.Now()
	return &manifest.Manifest{
		SchemaVersion:  "2",
		SessionID:      sessionID,
		Name:           name,
		Harness:        "claude-code",
		Workspace:      "test",
		CreatedAt:      now,
		UpdatedAt:      now,
		Lifecycle:      "",
		State:          "DONE",
		StateUpdatedAt: now,
		StateSource:    "test",
		Context: manifest.Context{
			Project: filepath.Join("/tmp/test", sessionID),
			Purpose: "Test session",
			Tags:    []string{"test"},
			Notes:   "Created by mock adapter for testing",
		},
		Tmux: manifest.Tmux{
			SessionName: name,
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-" + sessionID,
		},
	}
}

// --- manifest.Store implementation (delegates to legacy methods) ---

// Create implements manifest.Store.
func (m *MockAdapter) Create(man *manifest.Manifest) error {
	return m.CreateSession(man)
}

// Get implements manifest.Store.
func (m *MockAdapter) Get(sessionID string) (*manifest.Manifest, error) {
	return m.GetSession(sessionID)
}

// Update implements manifest.Store.
func (m *MockAdapter) Update(man *manifest.Manifest) error {
	return m.UpdateSession(man)
}

// Delete implements manifest.Store.
func (m *MockAdapter) Delete(sessionID string) error {
	return m.DeleteSession(sessionID)
}

// List implements manifest.Store by converting manifest.Filter to SessionFilter.
func (m *MockAdapter) List(filter *manifest.Filter) ([]*manifest.Manifest, error) {
	if filter == nil {
		return m.ListSessions(nil)
	}
	sf := &SessionFilter{
		Workspace: filter.Workspace,
		Harness:   filter.Harness,
		Tags:      filter.Tags,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	}
	switch filter.Status {
	case "archived":
		sf.Lifecycle = manifest.LifecycleArchived
	case "active":
		sf.ExcludeArchived = true
	}
	return m.ListSessions(sf)
}
