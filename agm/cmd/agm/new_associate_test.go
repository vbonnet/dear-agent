package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/readiness"
)

// TestAssociateSpawnedClaudeSession_WritesReadyFileWithoutStorage is the core
// regression test for ce-o1sg: readiness must be signalled deterministically by
// the spawner, NOT by an in-session /agm:agm-assoc subprocess that may never run
// in a spawned/sandbox session where the agm plugin isn't loaded.
//
// We force the Dolt-unavailable path (AGM_DB_PATH short-circuits getStorage),
// which is the worst case — no manifest resolution, no UUID backfill — and
// assert the ready-file is still written. If this passes, the post-create wait
// can never hang on a missing in-session subprocess.
func TestAssociateSpawnedClaudeSession_WritesReadyFileWithoutStorage(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)                            // ready-file lands here, not ~/.agm
	t.Setenv("AGM_SESSIONS_DIR", t.TempDir())                      // isolate manifest path resolution
	t.Setenv("AGM_DB_PATH", filepath.Join(t.TempDir(), "fake.db")) // force getStorage() error

	sessionName := "test-det-assoc-" + time.Now().Format("150405.000000")

	associateSpawnedClaudeSession(sessionName)

	readyFile := filepath.Join(stateDir, "ready-"+sessionName)
	info, err := os.Stat(readyFile)
	require.NoError(t, err, "ready-file must be written even when Dolt/association is unavailable")
	assert.False(t, info.IsDir(), "ready-file must be a regular file")
}

// TestAssociateSpawnedClaudeSession_ReadyFileIsParseable verifies the ready-file
// the spawner writes is consumable by WaitForReady — i.e. the deterministic
// signal the post-create flow blocks on actually resolves.
func TestAssociateSpawnedClaudeSession_ReadyFileIsParseable(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Setenv("AGM_SESSIONS_DIR", t.TempDir())
	t.Setenv("AGM_DB_PATH", filepath.Join(t.TempDir(), "fake.db"))

	sessionName := "test-det-ready-" + time.Now().Format("150405.000000")

	associateSpawnedClaudeSession(sessionName)

	// WaitForReady consumes (and removes) the ready-file; it must succeed
	// immediately since the spawner already wrote it.
	err := readiness.WaitForReady(sessionName, 2*time.Second)
	assert.NoError(t, err, "WaitForReady should resolve the spawner-written ready-file")
}

// TestReadyFileCreation verifies the underlying ready-file primitive used by the
// deterministic association path (readiness.CreateReadyFile → WaitForReady).
func TestReadyFileCreation(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)

	sessionName := "test-ready-file-" + time.Now().Format("150405.000000")
	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")

	err := readiness.CreateReadyFile(sessionName, manifestPath)
	require.NoError(t, err, "Should create ready-file")

	readyFile := filepath.Join(stateDir, "ready-"+sessionName)
	_, err = os.Stat(readyFile)
	assert.NoError(t, err, "Ready-file should exist")

	err = readiness.WaitForReady(sessionName, 5*time.Second)
	assert.NoError(t, err, "WaitForReady should succeed when file exists")
}
