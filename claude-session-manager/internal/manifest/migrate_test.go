package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestMigrateV1ToV2(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	backupPath := manifestPath + ".v1.bak"

	// Create a v1 manifest
	v1 := ManifestV1{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		Status:        StatusActive,
		CreatedAt:     testTime(),
		LastActivity:  testTime(),
		Worktree: WorktreeV1{
			Path: "/home/user/test",
		},
		Claude: ClaudeV1{
			SessionID: "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
		},
		Tmux: TmuxV1{
			SessionName: "test-tmux",
		},
	}

	// Write v1 manifest
	v1Data, err := yaml.Marshal(v1)
	assert.NoError(t, err)
	err = os.WriteFile(manifestPath, v1Data, 0600)
	assert.NoError(t, err)

	// Migrate
	err = MigrateV1ToV2(manifestPath)
	assert.NoError(t, err)

	// Verify backup exists
	_, err = os.Stat(backupPath)
	assert.NoError(t, err, "backup file should exist")

	// Verify backup contains original v1 data
	backupData, err := os.ReadFile(backupPath)
	assert.NoError(t, err)
	assert.Equal(t, v1Data, backupData, "backup should match original v1 data")

	// Verify v2 manifest was created
	v2Data, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	var v2 Manifest
	err = yaml.Unmarshal(v2Data, &v2)
	assert.NoError(t, err)

	// Verify v2 fields
	assert.Equal(t, SchemaVersion, v2.SchemaVersion)
	assert.Equal(t, v1.SessionID, v2.SessionID)
	assert.Equal(t, v1.Tmux.SessionName, v2.Name)
	assert.Equal(t, v1.CreatedAt, v2.CreatedAt)
	assert.Equal(t, "", v2.Lifecycle) // active status -> empty lifecycle
	assert.Equal(t, v1.Worktree.Path, v2.Context.Project)
	assert.Equal(t, v1.Tmux.SessionName, v2.Tmux.SessionName)
}

func TestMigrateV1ToV2_ArchivedStatus(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Create a v1 manifest with archived status
	v1 := ManifestV1{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		Status:        StatusArchived,
		CreatedAt:     testTime(),
		LastActivity:  testTime(),
		Worktree: WorktreeV1{
			Path: "/home/user/test",
		},
		Claude: ClaudeV1{
			SessionID: "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
		},
		Tmux: TmuxV1{
			SessionName: "test-tmux",
		},
	}

	v1Data, err := yaml.Marshal(v1)
	assert.NoError(t, err)
	err = os.WriteFile(manifestPath, v1Data, 0600)
	assert.NoError(t, err)

	// Migrate
	err = MigrateV1ToV2(manifestPath)
	assert.NoError(t, err)

	// Read v2 manifest
	v2Data, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	var v2 Manifest
	err = yaml.Unmarshal(v2Data, &v2)
	assert.NoError(t, err)

	// Verify archived status -> archived lifecycle
	assert.Equal(t, LifecycleArchived, v2.Lifecycle)
}

func TestMigrateV1ToV2_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	backupPath := manifestPath + ".v1.bak"

	// Create a v1 manifest
	v1 := ManifestV1{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		Status:        StatusActive,
		CreatedAt:     testTime(),
		LastActivity:  testTime(),
		Worktree: WorktreeV1{
			Path: "/home/user/test",
		},
		Claude: ClaudeV1{
			SessionID: "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
		},
		Tmux: TmuxV1{
			SessionName: "test-tmux",
		},
	}

	v1Data, err := yaml.Marshal(v1)
	assert.NoError(t, err)
	err = os.WriteFile(manifestPath, v1Data, 0600)
	assert.NoError(t, err)

	// First migration
	err = MigrateV1ToV2(manifestPath)
	assert.NoError(t, err)

	// Get v2 data after first migration
	v2DataFirst, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	// Get backup mtime
	backupInfo1, err := os.Stat(backupPath)
	assert.NoError(t, err)
	backupMtime1 := backupInfo1.ModTime()

	// Wait a bit to ensure different mtime if backup is recreated
	time.Sleep(10 * time.Millisecond)

	// Second migration (should skip)
	err = MigrateV1ToV2(manifestPath)
	assert.NoError(t, err)

	// Verify backup was NOT recreated (mtime unchanged)
	backupInfo2, err := os.Stat(backupPath)
	assert.NoError(t, err)
	backupMtime2 := backupInfo2.ModTime()
	assert.Equal(t, backupMtime1, backupMtime2, "backup should not be recreated")

	// Verify v2 data is unchanged
	v2DataSecond, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)
	assert.Equal(t, v2DataFirst, v2DataSecond, "v2 manifest should be unchanged")
}

func TestMigrateV1ToV2_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	backupPath := manifestPath + ".v1.bak"

	// Create a v1 manifest
	v1 := ManifestV1{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		Status:        StatusActive,
		CreatedAt:     testTime(),
		LastActivity:  testTime(),
		Worktree: WorktreeV1{
			Path: "/home/user/test",
		},
		Claude: ClaudeV1{
			SessionID: "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
		},
		Tmux: TmuxV1{
			SessionName: "test-tmux",
		},
	}

	v1Data, err := yaml.Marshal(v1)
	assert.NoError(t, err)
	err = os.WriteFile(manifestPath, v1Data, 0600)
	assert.NoError(t, err)

	// Run two concurrent migrations
	done := make(chan error, 2)

	go func() {
		done <- MigrateV1ToV2(manifestPath)
	}()

	go func() {
		done <- MigrateV1ToV2(manifestPath)
	}()

	// Wait for both to complete
	err1 := <-done
	err2 := <-done

	// At least one should succeed (the other may fail due to lock or skip due to backup)
	assert.True(t, err1 == nil || err2 == nil, "at least one migration should succeed")

	// Verify backup exists
	_, err = os.Stat(backupPath)
	assert.NoError(t, err, "backup file should exist")

	// Verify v2 manifest is valid
	v2Data, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	var v2 Manifest
	err = yaml.Unmarshal(v2Data, &v2)
	assert.NoError(t, err)
	assert.Equal(t, SchemaVersion, v2.SchemaVersion)
}

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		name         string
		manifestData string
		wantVersion  string
		wantErr      bool
	}{
		{
			name: "v2 manifest",
			manifestData: `schema_version: "2.0"
session_id: test`,
			wantVersion: "2.0",
			wantErr:     false,
		},
		{
			name: "v1 manifest",
			manifestData: `schema_version: "1.0"
session_id: test`,
			wantVersion: "1.0",
			wantErr:     false,
		},
		{
			name:         "missing schema_version (defaults to v1)",
			manifestData: `session_id: test`,
			wantVersion:  "1.0",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			manifestPath := filepath.Join(tmpDir, "manifest.yaml")

			err := os.WriteFile(manifestPath, []byte(tt.manifestData), 0600)
			assert.NoError(t, err)

			version, err := detectVersion(manifestPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantVersion, version)
			}
		})
	}
}
