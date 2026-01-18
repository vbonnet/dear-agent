package main

import (
	"sync"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// In-memory session list cache for performance
var (
	sessionListCache []*manifest.Manifest
	cacheTimestamp   time.Time
	cacheMutex       sync.RWMutex
)

// listSessionsCached returns cached session list (5s TTL) or refreshes from disk
func listSessionsCached(sessionsDir string) ([]*manifest.Manifest, error) {
	// Check cache (read lock)
	cacheMutex.RLock()
	if time.Since(cacheTimestamp) < 5*time.Second && sessionListCache != nil {
		defer cacheMutex.RUnlock()
		return sessionListCache, nil
	}
	cacheMutex.RUnlock()

	// Refresh cache (write lock)
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check: another goroutine may have refreshed while we waited
	if time.Since(cacheTimestamp) < 5*time.Second && sessionListCache != nil {
		return sessionListCache, nil
	}

	// Read from disk
	sessions, err := manifest.List(sessionsDir)
	if err != nil {
		return nil, err
	}

	// Update cache
	sessionListCache = sessions
	cacheTimestamp = time.Now()

	return sessions, nil
}

// invalidateCache forces cache refresh on next query
// Call this when sessions are created/updated
func invalidateCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cacheTimestamp = time.Time{} // Zero value forces refresh
}
