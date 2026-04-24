package health

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteCache writes health check results to cache (atomic operation)
func WriteCache(cache *HealthCheckCache) error {
	cacheFile := filepath.Join(os.Getenv("HOME"), ".engram", "cache", "health-check.json")

	// Create cache directory if it doesn't exist
	cacheDir := filepath.Dir(cacheFile)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	// Marshal to JSON with indentation (human-readable)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	// Write to temporary file first
	tmpFile := cacheFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write temp cache: %w", err)
	}

	// Atomic rename (POSIX guarantees atomicity)
	if err := os.Rename(tmpFile, cacheFile); err != nil {
		os.Remove(tmpFile) // Clean up on failure
		return fmt.Errorf("rename cache: %w", err)
	}

	return nil
}

// ReadCache reads and validates the health check cache
func ReadCache() (*HealthCheckCache, error) {
	cacheFile := filepath.Join(os.Getenv("HOME"), ".engram", "cache", "health-check.json")

	// Read file
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}

	// Unmarshal JSON
	var cache HealthCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}

	// Validate schema
	if cache.Version != "1.0" {
		return nil, fmt.Errorf("unsupported cache version: %s", cache.Version)
	}

	// Validate timestamp
	if _, err := time.Parse(time.RFC3339, cache.Timestamp); err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Validate TTL range
	if cache.TTL < 0 || cache.TTL > 86400 {
		return nil, fmt.Errorf("invalid TTL: %d (must be 0-86400)", cache.TTL)
	}

	return &cache, nil
}

// IsCacheFresh checks if cache is still valid (within TTL)
func IsCacheFresh(cache *HealthCheckCache) bool {
	timestamp, err := time.Parse(time.RFC3339, cache.Timestamp)
	if err != nil {
		return false
	}

	age := time.Since(timestamp)
	return age < time.Duration(cache.TTL)*time.Second
}

// BuildCacheFromResults converts health check results to cache format
func BuildCacheFromResults(results []CheckResult, summary Summary) *HealthCheckCache {
	cache := &HealthCheckCache{
		Version:   "1.0",
		Timestamp: time.Now().Format(time.RFC3339),
		TTL:       300, // 5 minutes
		Checks:    make(map[string]CheckResult),
		Plugins:   make(map[string]CheckResult),
		Summary:   summary,
	}

	for _, r := range results {
		cache.Checks[r.Name] = r
	}

	return cache
}
