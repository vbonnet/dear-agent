package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DiskCache persists identity to disk for faster subsequent loads
type DiskCache struct {
	path              string
	ttl               time.Duration
	gcpADCPath        string // Path to GCP ADC file for invalidation checking
	gitConfigPath     string // Path to git config for invalidation checking
	checkInvalidation bool   // Whether to check source file modification times
}

// NewDiskCache creates a disk cache instance with file invalidation checking
func NewDiskCache(ttl time.Duration) (*DiskCache, error) {
	// Use XDG cache directory standard: ~/.cache/engram/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "engram")
	cachePath := filepath.Join(cacheDir, "identity.json")

	// Set up paths for invalidation checking
	gcpADCPath := filepath.Join(homeDir, ".config", "gcloud", "application_default_credentials.json")
	gitConfigPath := filepath.Join(homeDir, ".gitconfig")

	return &DiskCache{
		path:              cachePath,
		ttl:               ttl,
		gcpADCPath:        gcpADCPath,
		gitConfigPath:     gitConfigPath,
		checkInvalidation: true,
	}, nil
}

// Get loads identity from disk cache if valid
func (dc *DiskCache) Get() (*Identity, error) {
	// Check if cache file exists
	stat, err := os.Stat(dc.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Cache miss, not an error
		}
		return nil, fmt.Errorf("stat cache file: %w", err)
	}

	cacheModTime := stat.ModTime()

	// Check if cache is expired based on modification time
	age := time.Since(cacheModTime)
	if age > dc.ttl {
		return nil, nil // Expired, not an error
	}

	// Check if source files have been modified since cache was created
	if dc.checkInvalidation {
		if dc.isSourceFileModified(cacheModTime) {
			// Source file changed, invalidate cache
			_ = dc.Clear()
			return nil, nil
		}
	}

	// Read cache file
	data, err := os.ReadFile(dc.path)
	if err != nil {
		return nil, fmt.Errorf("reading cache file: %w", err)
	}

	// Parse JSON
	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		// Cache corrupted, delete it
		_ = os.Remove(dc.path)
		return nil, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Verify DetectedAt is set and within TTL
	if identity.DetectedAt.IsZero() || time.Since(identity.DetectedAt) > dc.ttl {
		return nil, nil // Invalid or expired
	}

	return &identity, nil
}

// isSourceFileModified checks if any identity source files were modified after cache
func (dc *DiskCache) isSourceFileModified(cacheModTime time.Time) bool {
	// Check GCP ADC file
	if stat, err := os.Stat(dc.gcpADCPath); err == nil {
		if stat.ModTime().After(cacheModTime) {
			return true
		}
	}

	// Check git config file
	if stat, err := os.Stat(dc.gitConfigPath); err == nil {
		if stat.ModTime().After(cacheModTime) {
			return true
		}
	}

	// Also check gcloud config_default (contains account email)
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		configDefaultPath := filepath.Join(homeDir, ".config", "gcloud", "configurations", "config_default")
		if stat, err := os.Stat(configDefaultPath); err == nil {
			if stat.ModTime().After(cacheModTime) {
				return true
			}
		}
	}

	return false
}

// Set saves identity to disk cache
func (dc *DiskCache) Set(identity *Identity) error {
	if identity == nil {
		return nil // Nothing to cache
	}

	// Create cache directory if it doesn't exist
	cacheDir := filepath.Dir(dc.path)
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling identity: %w", err)
	}

	// Write to temp file first (atomic write)
	tempPath := dc.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("writing temp cache file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, dc.path); err != nil {
		_ = os.Remove(tempPath) // Cleanup temp file
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

// Clear removes the disk cache file
func (dc *DiskCache) Clear() error {
	if err := os.Remove(dc.path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already cleared
		}
		return fmt.Errorf("removing cache file: %w", err)
	}
	return nil
}

// Path returns the cache file path (for testing)
func (dc *DiskCache) Path() string {
	return dc.path
}
