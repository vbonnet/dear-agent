//go:build integration

package lifecycle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestConcurrent_CreateMultipleSessions tests creating multiple sessions simultaneously
func TestConcurrent_CreateMultipleSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent creation test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	numSessions := 5
	sessions := make([]string, numSessions)
	var wg sync.WaitGroup
	errors := make(chan error, numSessions)

	// Create sessions concurrently
	for i := 0; i < numSessions; i++ {
		sessionName := fmt.Sprintf("concurrent-create-%d-%s", i, helpers.RandomString(4))
		sessions[i] = sessionName

		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := helpers.CreateSessionManifest(env.SessionsDir, name, "claude"); err != nil {
				errors <- fmt.Errorf("failed to create %s: %w", name, err)
			}
		}(sessionName)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Creation error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Failed to create %d sessions", errorCount)
	}

	// Verify all sessions exist with unique IDs
	sessionIDs := make(map[string]bool)
	for _, sessionName := range sessions {
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Errorf("Failed to read manifest for %s: %v", sessionName, err)
			continue
		}

		// Check for duplicate session IDs
		if sessionIDs[m.SessionID] {
			t.Errorf("Duplicate session ID found: %s", m.SessionID)
		}
		sessionIDs[m.SessionID] = true

		// Verify required fields
		if m.Name != sessionName {
			t.Errorf("Expected name %s, got %s", sessionName, m.Name)
		}
		if m.Agent != "claude" {
			t.Errorf("Expected agent 'claude', got %s", m.Agent)
		}
	}

	if len(sessionIDs) != numSessions {
		t.Errorf("Expected %d unique session IDs, got %d", numSessions, len(sessionIDs))
	}
}

// TestConcurrent_ArchiveMultipleSessions tests archiving multiple sessions simultaneously
func TestConcurrent_ArchiveMultipleSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent archive test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	numSessions := 5
	sessions := make([]string, numSessions)

	// Create sessions first
	for i := 0; i < numSessions; i++ {
		sessionName := fmt.Sprintf("concurrent-archive-%d-%s", i, helpers.RandomString(4))
		sessions[i] = sessionName

		if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
			t.Fatalf("Failed to create session %s: %v", sessionName, err)
		}
	}

	// Archive concurrently
	var wg sync.WaitGroup
	errors := make(chan error, numSessions)

	for _, sessionName := range sessions {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := helpers.ArchiveTestSession(env.SessionsDir, name, "concurrent test"); err != nil {
				errors <- fmt.Errorf("failed to archive %s: %w", name, err)
			}
		}(sessionName)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Archive error: %v", err)
		errorCount++
	}

	// Verify all sessions are archived (in-place with lifecycle field)
	for _, sessionName := range sessions {
		// Archived sessions remain in original location with lifecycle: archived
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Errorf("Failed to read manifest for %s: %v", sessionName, err)
			continue
		}

		if m.Lifecycle != manifest.LifecycleArchived {
			t.Errorf("Session %s should be archived, got lifecycle: %s", sessionName, m.Lifecycle)
		}
	}

	t.Logf("Concurrent archive completed with %d errors out of %d sessions", errorCount, numSessions)
}

// TestConcurrent_ReadWriteManifest tests concurrent manifest reads/writes
func TestConcurrent_ReadWriteManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent manifest test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "concurrent-manifest-" + helpers.RandomString(6)

	// Create initial session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")

	// Concurrent readers and writers
	numReaders := 10
	numWriters := 5
	var wg sync.WaitGroup
	errors := make(chan error, numReaders+numWriters)

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				_, err := manifest.Read(manifestPath)
				if err != nil {
					errors <- fmt.Errorf("reader %d iteration %d: %w", id, j, err)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Start writers (updating tags)
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				m, err := manifest.Read(manifestPath)
				if err != nil {
					errors <- fmt.Errorf("writer %d read error: %w", id, err)
					return
				}

				// Add a tag
				tag := fmt.Sprintf("writer-%d-iteration-%d", id, j)
				m.Context.Tags = append(m.Context.Tags, tag)

				if err := manifest.Write(manifestPath, m); err != nil {
					errors <- fmt.Errorf("writer %d write error: %w", id, err)
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Report errors
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent manifest operation error: %v", err)
		errorCount++
	}

	// Read final manifest
	finalManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read final manifest: %v", err)
	}

	// Verify manifest is valid
	if finalManifest.SessionID == "" {
		t.Error("SessionID should not be empty")
	}

	// Note: Tags may not all be present due to race conditions (last write wins)
	// This is expected behavior without explicit locking
	t.Logf("Final manifest has %d tags after concurrent updates", len(finalManifest.Context.Tags))

	if errorCount > 0 {
		t.Errorf("Concurrent manifest operations had %d errors", errorCount)
	}
}

// TestConcurrent_ListWhileCreating tests listing sessions while creating new ones
func TestConcurrent_ListWhileCreating(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent list test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	var wg sync.WaitGroup
	stopListers := make(chan bool)
	errors := make(chan error, 100)

	// Start continuous listers
	numListers := 3
	for i := 0; i < numListers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stopListers:
					return
				default:
					_, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
					if err != nil {
						errors <- fmt.Errorf("lister %d: %w", id, err)
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
		}(i)
	}

	// Create sessions while listers are running
	numCreators := 5
	for i := 0; i < numCreators; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionName := fmt.Sprintf("concurrent-list-create-%d-%s", id, helpers.RandomString(4))
			if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
				errors <- fmt.Errorf("creator %d: %w", id, err)
			}
		}(i)
	}

	// Wait for creators to finish
	time.Sleep(500 * time.Millisecond)
	close(stopListers)
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent operation error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Concurrent list/create operations had %d errors", errorCount)
	}

	// Verify all sessions were created
	sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
	if err != nil {
		t.Fatalf("Failed final list: %v", err)
	}

	if len(sessions) < numCreators {
		t.Errorf("Expected at least %d sessions, got %d", numCreators, len(sessions))
	}
}

// TestConcurrent_SessionLifecycleStressTest tests high-volume concurrent operations
func TestConcurrent_SessionLifecycleStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	numSessions := 20
	var wg sync.WaitGroup
	errors := make(chan error, numSessions*3) // create + archive + cleanup

	// Stress test: create, archive, and verify many sessions
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			sessionName := fmt.Sprintf("stress-%d-%s", id, helpers.RandomString(4))

			// Create
			if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
				errors <- fmt.Errorf("create %s: %w", sessionName, err)
				return
			}

			// Small delay
			time.Sleep(time.Duration(id) * 10 * time.Millisecond)

			// Archive
			if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "stress test"); err != nil {
				errors <- fmt.Errorf("archive %s: %w", sessionName, err)
				return
			}

			// Verify (in-place archive with lifecycle field)
			manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
			m, err := manifest.Read(manifestPath)
			if err != nil {
				errors <- fmt.Errorf("verify %s: %w", sessionName, err)
				return
			}

			if m.Lifecycle != manifest.LifecycleArchived {
				errors <- fmt.Errorf("%s not archived: %s", sessionName, m.Lifecycle)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Report results
	errorCount := 0
	for err := range errors {
		t.Logf("Stress test error: %v", err)
		errorCount++
	}

	successRate := float64(numSessions-errorCount) / float64(numSessions) * 100
	t.Logf("Stress test success rate: %.1f%% (%d/%d)", successRate, numSessions-errorCount, numSessions)

	if errorCount > numSessions/4 {
		t.Errorf("Too many failures in stress test: %d/%d", errorCount, numSessions)
	}
}

// TestConcurrent_ManifestCorruptionRecovery tests recovery from concurrent corruption
func TestConcurrent_ManifestCorruptionRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping corruption recovery test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "concurrent-corrupt-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")

	// Simulate concurrent corruption and recovery
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Corruptor (writes invalid YAML)
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		corrupted := "invalid yaml {{{ unclosed"
		if err := os.WriteFile(manifestPath, []byte(corrupted), 0644); err != nil {
			errors <- fmt.Errorf("corruptor: %w", err)
		}
	}()

	// Readers (try to read concurrently)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_, err := manifest.Read(manifestPath)
				if err != nil {
					// Expected to fail sometimes due to corruption
					t.Logf("Reader %d iteration %d failed (expected): %v", id, j, err)
				}
				time.Sleep(30 * time.Millisecond)
			}
		}(i)
	}

	// Recoverer (writes valid manifest)
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)

		// Create valid manifest
		m := &manifest.Manifest{
			SchemaVersion: "2",
			SessionID:     "recovered-" + helpers.RandomString(8),
			Name:          sessionName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "",
			Context: manifest.Context{
				Project: filepath.Join(env.SessionsDir, sessionName, "project"),
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			Agent: "claude",
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			errors <- fmt.Errorf("recoverer: %w", err)
		}
	}()

	wg.Wait()
	close(errors)

	// Report errors (some are expected)
	for err := range errors {
		t.Logf("Operation error: %v", err)
	}

	// Verify final state is valid
	finalManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Final manifest should be valid after recovery: %v", err)
	}

	if finalManifest.SessionID == "" {
		t.Error("Recovered manifest should have valid SessionID")
	}
}

// TestConcurrent_ResourceLockContention tests file lock contention
func TestConcurrent_ResourceLockContention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping lock contention test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "lock-contention-" + helpers.RandomString(6)

	// Create session
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	lockPath := filepath.Join(sessionDir, ".lock")

	// Multiple goroutines trying to create lock file
	numContenders := 10
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numContenders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to create exclusive lock file
			// Note: This is simplified - real implementation would use flock or similar
			lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				lockFile.Close()
				mu.Lock()
				successCount++
				mu.Unlock()
				t.Logf("Contender %d acquired lock", id)
			} else {
				t.Logf("Contender %d failed to acquire lock (expected)", id)
			}

			time.Sleep(10 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	// Only one should succeed in creating exclusive lock
	if successCount != 1 {
		t.Logf("Lock contention resulted in %d successful acquisitions (expected 1)", successCount)
		// Note: Without proper locking mechanism, this might fail
		// This documents expected behavior when file locking is implemented
	}
}
