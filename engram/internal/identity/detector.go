// Package identity implements user identity detection from multiple sources.
//
// Identity detection enables company enforcement by verifying user email/domain.
// Supports multiple detection methods with priority ordering:
//  1. GCP ADC (priority 100, verified)
//  2. Git config (priority 50, unverified)
//  3. Environment variable (priority 10, unverified)
//
// Example usage:
//
//	mgr := identity.NewManager(24 * time.Hour)
//	id, err := mgr.Detect(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Detected: %s via %s\n", id.Email, id.Method)
//
// See core/docs/adr/enforcement-architecture.md for design details.
package identity

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Identity represents a detected user identity
type Identity struct {
	// User email address
	Email string `json:"email"`

	// Email domain (e.g., "@acme.com")
	Domain string `json:"domain"`

	// Detection method: "gcp_adc", "git_config", "env_var"
	Method string `json:"method"`

	// True if cryptographically verified (GCP ADC)
	Verified bool `json:"verified"`

	// When identity was detected
	DetectedAt time.Time `json:"detected_at"`
}

// Detector detects user identity from a specific source
type Detector interface {
	// Name returns detector name (e.g., "gcp_adc")
	Name() string

	// Detect attempts to detect user identity.
	// Returns (nil, nil) if identity cannot be detected (not an error).
	// Returns (nil, err) only for actual errors like I/O failures.
	Detect(ctx context.Context) (*Identity, error)

	// Priority returns detector priority (higher = try first).
	// Standard priorities: GCP ADC=100, Git=50, Env=10
	Priority() int
}

// Manager coordinates multiple identity detectors
type Manager struct {
	detectors []Detector
	cache     *Cache
	diskCache *DiskCache
}

// NewManager creates identity manager with default detectors and cache TTL
func NewManager(cacheTTL time.Duration) *Manager {
	// Create disk cache (ignore errors - disk cache is optional)
	diskCache, _ := NewDiskCache(cacheTTL)

	return &Manager{
		detectors: []Detector{
			&GCPADCDetector{},
			&GitConfigDetector{},
			&EnvVarDetector{},
		},
		cache:     NewCache(cacheTTL),
		diskCache: diskCache,
	}
}

// Detect tries all detectors in priority order, returning first successful detection.
// Results are cached (both in-memory and on disk) for the configured TTL.
func (m *Manager) Detect(ctx context.Context) (*Identity, error) {
	// Check disk cache first (fastest - ~1ms)
	if m.diskCache != nil {
		if cached, err := m.diskCache.Get(); err == nil && cached != nil {
			// Also populate in-memory cache for even faster subsequent access
			m.cache.Set(cached)
			return cached, nil
		}
	}

	// Check in-memory cache (~100ns)
	if cached := m.cache.Get(); cached != nil {
		// Also save to disk cache for persistence across restarts
		if m.diskCache != nil {
			_ = m.diskCache.Set(cached)
		}
		return cached, nil
	}

	// Cache miss - run detectors (~20ms)
	// Sort detectors by priority (highest first)
	sort.Slice(m.detectors, func(i, j int) bool {
		return m.detectors[i].Priority() > m.detectors[j].Priority()
	})

	// Try each detector in order
	for _, detector := range m.detectors {
		id, err := detector.Detect(ctx)
		if err != nil {
			// Log error but continue to next detector
			// TODO: Add structured logging here
			continue
		}

		if id != nil {
			// Success! Cache in both memory and disk
			m.cache.Set(id)
			if m.diskCache != nil {
				_ = m.diskCache.Set(id)
			}
			return id, nil
		}
	}

	// No detector succeeded
	return nil, fmt.Errorf("no identity detected (tried %d methods)", len(m.detectors))
}

// ClearCache forces re-detection on next Detect() call
// Clears both in-memory and disk caches
func (m *Manager) ClearCache() {
	m.cache.Clear()
	if m.diskCache != nil {
		_ = m.diskCache.Clear()
	}
}
