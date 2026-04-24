package session

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestGitCache_GetSet(t *testing.T) {
	cache := NewGitCache()

	// Get from empty cache
	_, ok := cache.Get("missing")
	if ok {
		t.Error("Get on empty cache should return false")
	}

	// Set and retrieve
	cache.Set("key1", "value1")
	val, ok := cache.Get("key1")
	if !ok {
		t.Error("Get after Set should return true")
	}
	if val.(string) != "value1" {
		t.Errorf("Get = %q, want %q", val, "value1")
	}

	// Set integer value
	cache.Set("count", 42)
	val, ok = cache.Get("count")
	if !ok {
		t.Error("Get for int value should return true")
	}
	if val.(int) != 42 {
		t.Errorf("Get = %d, want 42", val)
	}
}

func TestGitCache_Expiration(t *testing.T) {
	cache := &GitCache{
		entries: make(map[string]*gitCacheEntry),
		ttl:     50 * time.Millisecond, // Short TTL for testing
	}

	cache.Set("expires", "soon")

	// Should exist immediately
	_, ok := cache.Get("expires")
	if !ok {
		t.Error("entry should exist immediately after Set")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("expires")
	if ok {
		t.Error("entry should be expired after TTL")
	}
}

func TestGitCache_Clear(t *testing.T) {
	cache := NewGitCache()

	cache.Set("a", 1)
	cache.Set("b", 2)

	cache.Clear()

	_, ok := cache.Get("a")
	if ok {
		t.Error("Get after Clear should return false")
	}
	_, ok = cache.Get("b")
	if ok {
		t.Error("Get after Clear should return false")
	}
}

func TestGitCache_CleanExpired(t *testing.T) {
	cache := &GitCache{
		entries: make(map[string]*gitCacheEntry),
		ttl:     50 * time.Millisecond,
	}

	cache.Set("short", "lived")

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Add a fresh entry
	cache.ttl = 5 * time.Second
	cache.Set("fresh", "value")

	// Clean expired
	cache.CleanExpired()

	// Expired entry should be gone
	_, ok := cache.Get("short")
	if ok {
		t.Error("expired entry should be removed by CleanExpired")
	}

	// Fresh entry should remain
	val, ok := cache.Get("fresh")
	if !ok {
		t.Error("fresh entry should survive CleanExpired")
	}
	if val.(string) != "value" {
		t.Errorf("fresh entry = %q, want %q", val, "value")
	}
}

func TestClearGitCache(t *testing.T) {
	globalGitCache.Set("test-key", "test-value")
	ClearGitCache()

	_, ok := globalGitCache.Get("test-key")
	if ok {
		t.Error("ClearGitCache should clear global cache")
	}
}

func TestGetCurrentBranchCached(t *testing.T) {
	// Use a temp git repo as a known git directory
	gitDir := t.TempDir()
	// Initialize a git repo for the test
	cmd := exec.Command("git", "init", gitDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}
	cmd = exec.Command("git", "-C", gitDir, "commit", "--allow-empty", "-m", "init")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}
	ClearGitCache()

	// First call - cache miss
	branch, err := GetCurrentBranchCached(gitDir)
	if err != nil {
		t.Fatalf("GetCurrentBranchCached() error = %v", err)
	}
	if branch == "" {
		t.Error("expected non-empty branch")
	}

	// Second call - should use cache
	branch2, err := GetCurrentBranchCached(gitDir)
	if err != nil {
		t.Fatalf("GetCurrentBranchCached() cached call error = %v", err)
	}
	if branch2 != branch {
		t.Errorf("cached branch = %q, want %q", branch2, branch)
	}

	// Non-existent dir
	ClearGitCache()
	_, err = GetCurrentBranchCached("/nonexistent/dir/xyz-99999")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestGetUncommittedCountCached(t *testing.T) {
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}
	cmd = exec.Command("git", "-C", gitDir, "commit", "--allow-empty", "-m", "init")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}
	ClearGitCache()

	// First call - cache miss
	count, err := GetUncommittedCountCached(gitDir)
	if err != nil {
		t.Fatalf("GetUncommittedCountCached() error = %v", err)
	}
	// count can be 0 or more, just check it doesn't error

	// Second call - should use cache
	count2, err := GetUncommittedCountCached(gitDir)
	if err != nil {
		t.Fatalf("GetUncommittedCountCached() cached call error = %v", err)
	}
	if count2 != count {
		t.Errorf("cached count = %d, want %d", count2, count)
	}

	// Non-existent dir
	ClearGitCache()
	_, err = GetUncommittedCountCached("/nonexistent/dir/xyz-99999")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}
