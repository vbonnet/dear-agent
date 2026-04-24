// Package transcript provides transcript functionality.
package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SessionMap manages UUID -> human-readable name mappings
type SessionMap struct {
	mu       sync.RWMutex
	filePath string
	names    map[string]string // UUID -> name
}

// NewSessionMap creates or loads a session map from file
func NewSessionMap(projectRoot string) (*SessionMap, error) {
	mapPath := filepath.Join(projectRoot, "transcripts", "session_map.json")

	sm := &SessionMap{
		filePath: mapPath,
		names:    make(map[string]string),
	}

	// Load existing mappings if file exists
	if _, err := os.Stat(mapPath); err == nil {
		if err := sm.load(); err != nil {
			return nil, fmt.Errorf("failed to load session map: %w", err)
		}
	}

	return sm, nil
}

// SetName assigns a human-readable name to a session UUID
func (sm *SessionMap) SetName(uuid, name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.names[uuid] = name

	return sm.save()
}

// GetName retrieves the name for a UUID (returns empty string if not found)
func (sm *SessionMap) GetName(uuid string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.names[uuid]
}

// DeleteName removes a name mapping
func (sm *SessionMap) DeleteName(uuid string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.names, uuid)

	return sm.save()
}

// ListAll returns all UUID -> name mappings
func (sm *SessionMap) ListAll() map[string]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Return copy to prevent external modification
	copy := make(map[string]string, len(sm.names))
	for k, v := range sm.names {
		copy[k] = v
	}

	return copy
}

// load reads session map from JSON file
func (sm *SessionMap) load() error {
	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &sm.names)
}

// save writes session map to JSON file
func (sm *SessionMap) save() error {
	// Ensure directory exists
	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal to JSON (pretty-printed)
	data, err := json.MarshalIndent(sm.names, "", "  ")
	if err != nil {
		return err
	}

	// Write atomically (write to temp, then rename)
	tempPath := sm.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, sm.filePath)
}
