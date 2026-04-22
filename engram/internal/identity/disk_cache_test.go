package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDiskCache_GetSet tests basic get/set operations
func TestDiskCache_GetSet(t *testing.T) {
	// Create temp cache
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "identity.json"),
		ttl:  1 * time.Hour,
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		Verified:   true,
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Get identity
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get() returned nil")
	}

	if retrieved.Email != id.Email {
		t.Errorf("Email = %s, want %s", retrieved.Email, id.Email)
	}

	if retrieved.Domain != id.Domain {
		t.Errorf("Domain = %s, want %s", retrieved.Domain, id.Domain)
	}
}

// TestDiskCache_Expiry tests TTL expiration
func TestDiskCache_Expiry(t *testing.T) {
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "identity.json"),
		ttl:  100 * time.Millisecond,
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Should retrieve before expiry
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Get() should return identity before expiry")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should return nil after expiry
	expired, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() after expiry failed: %v", err)
	}
	if expired != nil {
		t.Error("Get() should return nil after expiry")
	}
}

// TestDiskCache_Clear tests cache clearing
func TestDiskCache_Clear(t *testing.T) {
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "identity.json"),
		ttl:  1 * time.Hour,
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Clear cache
	err = cache.Clear()
	if err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	// Should return nil after clear
	cleared, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() after clear failed: %v", err)
	}
	if cleared != nil {
		t.Error("Get() should return nil after Clear()")
	}

	// Clearing again should not error
	err = cache.Clear()
	if err != nil {
		t.Errorf("Clear() on already cleared cache should not error: %v", err)
	}
}

// TestDiskCache_MissingFile tests behavior when cache file doesn't exist
func TestDiskCache_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "nonexistent.json"),
		ttl:  1 * time.Hour,
	}

	// Get should return nil, no error
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() on missing file should not error: %v", err)
	}
	if retrieved != nil {
		t.Error("Get() on missing file should return nil")
	}
}

// TestDiskCache_CorruptedFile tests handling of corrupted cache
func TestDiskCache_CorruptedFile(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "identity.json")
	cache := &DiskCache{
		path: cachePath,
		ttl:  1 * time.Hour,
	}

	// Write corrupted JSON
	err := os.WriteFile(cachePath, []byte("not valid json"), 0600)
	if err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}

	// Get should return nil and delete corrupted file
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() on corrupted file should not error: %v", err)
	}
	if retrieved != nil {
		t.Error("Get() on corrupted file should return nil")
	}

	// File should be deleted
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("Corrupted cache file should be deleted")
	}
}

// TestDiskCache_AtomicWrite tests atomic write behavior
func TestDiskCache_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "identity.json"),
		ttl:  1 * time.Hour,
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Verify temp file doesn't exist
	tempPath := cache.path + ".tmp"
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after Set()")
	}

	// Verify cache file exists
	if _, err := os.Stat(cache.path); err != nil {
		t.Errorf("Cache file should exist: %v", err)
	}
}

// TestDiskCache_Permissions tests file permissions
func TestDiskCache_Permissions(t *testing.T) {
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "identity.json"),
		ttl:  1 * time.Hour,
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Check file permissions (should be 0600 - user read/write only)
	stat, err := os.Stat(cache.path)
	if err != nil {
		t.Fatalf("Failed to stat cache file: %v", err)
	}

	mode := stat.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Cache file permissions = %o, want 0600", mode)
	}
}

// TestDiskCache_DetectedAtZero tests handling of zero DetectedAt
func TestDiskCache_DetectedAtZero(t *testing.T) {
	tempDir := t.TempDir()
	cache := &DiskCache{
		path: filepath.Join(tempDir, "identity.json"),
		ttl:  1 * time.Hour,
	}

	// Set identity with zero DetectedAt (invalid)
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Time{}, // Zero value
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Get should return nil for invalid DetectedAt
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Get() should return nil for identity with zero DetectedAt")
	}
}

// TestDiskCache_Invalidation tests source file modification invalidation
func TestDiskCache_Invalidation(t *testing.T) {
	tempDir := t.TempDir()

	// Create fake source files
	gcpADCPath := filepath.Join(tempDir, "adc.json")
	gitConfigPath := filepath.Join(tempDir, ".gitconfig")

	// Write initial source files
	_ = os.WriteFile(gcpADCPath, []byte(`{"type":"authorized_user"}`), 0600)
	_ = os.WriteFile(gitConfigPath, []byte(`[user]\nemail=old@example.com`), 0600)

	// Wait to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	// Create cache with custom paths
	cache := &DiskCache{
		path:              filepath.Join(tempDir, "identity.json"),
		ttl:               1 * time.Hour,
		gcpADCPath:        gcpADCPath,
		gitConfigPath:     gitConfigPath,
		checkInvalidation: true,
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Get should succeed (cache valid)
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Get() should return cached identity")
	}

	// Wait to ensure different timestamp
	time.Sleep(10 * time.Millisecond)

	// Modify source file (simulate gcloud auth login)
	_ = os.WriteFile(gitConfigPath, []byte(`[user]\nemail=new@example.com`), 0600)

	// Get should now return nil (cache invalidated)
	invalidated, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() after source modification failed: %v", err)
	}
	if invalidated != nil {
		t.Error("Get() should return nil after source file modification")
	}
}

// TestDiskCache_NoInvalidation tests cache with invalidation checking disabled
func TestDiskCache_NoInvalidation(t *testing.T) {
	tempDir := t.TempDir()

	// Create fake source file
	gcpADCPath := filepath.Join(tempDir, "adc.json")
	_ = os.WriteFile(gcpADCPath, []byte(`{"type":"authorized_user"}`), 0600)

	// Wait for different timestamp
	time.Sleep(10 * time.Millisecond)

	// Create cache with invalidation disabled
	cache := &DiskCache{
		path:              filepath.Join(tempDir, "identity.json"),
		ttl:               1 * time.Hour,
		gcpADCPath:        gcpADCPath,
		checkInvalidation: false, // Disabled
	}

	// Set identity
	id := &Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}

	err := cache.Set(id)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Wait for different timestamp
	time.Sleep(10 * time.Millisecond)

	// Modify source file
	_ = os.WriteFile(gcpADCPath, []byte(`{"type":"service_account"}`), 0600)

	// Get should still return cached value (invalidation disabled)
	retrieved, err := cache.Get()
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Get() should return cached identity when invalidation disabled")
	}
}
