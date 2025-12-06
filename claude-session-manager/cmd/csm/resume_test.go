package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

func TestCheckSessionHealth(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	worktreePath := filepath.Join(tmpDir, "worktree")
	sessionEnvPath := filepath.Join(tmpDir, "session-env")
	fileHistoryPath := filepath.Join(tmpDir, "file-history")
	manifestDir := filepath.Join(tmpDir, "manifest")
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	// Create manifest directory
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		t.Fatalf("Failed to create manifest dir: %v", err)
	}

	tests := []struct {
		name              string
		setupWorktree     bool
		setupSessionEnv   bool
		setupFileHistory  bool
		expectCanResume   bool
		expectIssues      int
		expectWarnings    int
	}{
		{
			name:              "healthy session - all directories exist",
			setupWorktree:     true,
			setupSessionEnv:   true,
			setupFileHistory:  true,
			expectCanResume:   true,
			expectIssues:      0,
			expectWarnings:    0,
		},
		{
			name:              "missing worktree - cannot resume",
			setupWorktree:     false,
			setupSessionEnv:   true,
			setupFileHistory:  true,
			expectCanResume:   false,
			expectIssues:      1,
			expectWarnings:    0,
		},
		{
			name:              "missing session-env - warning only",
			setupWorktree:     true,
			setupSessionEnv:   false,
			setupFileHistory:  true,
			expectCanResume:   true,
			expectIssues:      0,
			expectWarnings:    1,
		},
		{
			name:              "missing file-history - warning only",
			setupWorktree:     true,
			setupSessionEnv:   true,
			setupFileHistory:  false,
			expectCanResume:   true,
			expectIssues:      0,
			expectWarnings:    1,
		},
		{
			name:              "missing all optional dirs",
			setupWorktree:     true,
			setupSessionEnv:   false,
			setupFileHistory:  false,
			expectCanResume:   true,
			expectIssues:      0,
			expectWarnings:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup directories based on test case
			if tt.setupWorktree {
				os.MkdirAll(worktreePath, 0700)
				defer os.RemoveAll(worktreePath)
			}
			if tt.setupSessionEnv {
				os.MkdirAll(sessionEnvPath, 0700)
				defer os.RemoveAll(sessionEnvPath)
			}
			if tt.setupFileHistory {
				os.MkdirAll(fileHistoryPath, 0700)
				defer os.RemoveAll(fileHistoryPath)
			}

			// Create test manifest
			testUUID := "c4eb298c-8c89-4f75-8dae-c725a1291add"
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "test-session",
				Status:        manifest.StatusActive,
				CreatedAt:     time.Now(),
				LastActivity:  time.Now(),
				Worktree: manifest.Worktree{
					Path:   worktreePath,
					Branch: "main",
					Repo:   "test-repo",
				},
				Claude: manifest.Claude{
					SessionID:       testUUID,
					SessionEnvPath:  sessionEnvPath,
					FileHistoryPath: fileHistoryPath,
					StartedAt:       time.Now(),
					LastActivity:    time.Now(),
				},
				Tmux: manifest.Tmux{
					SessionName: "test-tmux",
					WindowName:  "main",
					CreatedAt:   time.Now(),
				},
			}

			// Write manifest
			if err := manifest.Write(manifestPath, m); err != nil {
				t.Fatalf("Failed to write manifest: %v", err)
			}

			// Run health check
			health, err := checkSessionHealth(testUUID, manifestPath)
			if err != nil {
				t.Fatalf("checkSessionHealth failed: %v", err)
			}

			// Verify results
			if health.CanResume != tt.expectCanResume {
				t.Errorf("CanResume = %v, want %v", health.CanResume, tt.expectCanResume)
			}

			if len(health.Issues) != tt.expectIssues {
				t.Errorf("Issues count = %d, want %d. Issues: %v",
					len(health.Issues), tt.expectIssues, health.Issues)
			}

			if len(health.Warnings) != tt.expectWarnings {
				t.Errorf("Warnings count = %d, want %d. Warnings: %v",
					len(health.Warnings), tt.expectWarnings, health.Warnings)
			}

			// Verify paths are set correctly
			if health.WorktreePath != worktreePath {
				t.Errorf("WorktreePath = %q, want %q", health.WorktreePath, worktreePath)
			}
			if health.SessionEnvPath != sessionEnvPath {
				t.Errorf("SessionEnvPath = %q, want %q", health.SessionEnvPath, sessionEnvPath)
			}
			if health.FileHistoryPath != fileHistoryPath {
				t.Errorf("FileHistoryPath = %q, want %q", health.FileHistoryPath, fileHistoryPath)
			}
		})
	}
}

func TestResolveSessionIdentifier_NoManifests(t *testing.T) {
	// This test requires a sessions directory with no manifests
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0700)

	// Temporarily override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, _, err := resolveSessionIdentifier("test")
	if err == nil {
		t.Error("Expected error when no manifests exist, got nil")
	}
}

func TestResolveSessionIdentifier_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create test manifests with different characteristics
	testUUID1 := "c4eb298c-8c89-4f75-8dae-c725a1291add"
	testUUID2 := "e6121188-1234-4567-8901-234567890abc"

	manifests := []struct {
		sessionID   string
		claudeUUID  string
		tmuxName    string
		worktreePath string
	}{
		{
			sessionID:   "workspace-design",
			claudeUUID:  testUUID1,
			tmuxName:    "claude-1",
			worktreePath: "/home/user/workspace-design",
		},
		{
			sessionID:   "test-project",
			claudeUUID:  testUUID2,
			tmuxName:    "claude-2",
			worktreePath: "/home/user/test-project",
		},
	}

	// Create manifests
	for _, spec := range manifests {
		manifestDir := filepath.Join(sessionsDir, spec.sessionID)
		manifestPath := filepath.Join(manifestDir, "manifest.yaml")
		os.MkdirAll(manifestDir, 0700)

		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     spec.sessionID,
			Status:        manifest.StatusActive,
			CreatedAt:     time.Now(),
			LastActivity:  time.Now(),
			Worktree: manifest.Worktree{
				Path:   spec.worktreePath,
				Branch: "main",
				Repo:   "test-repo",
			},
			Claude: manifest.Claude{
				SessionID:       spec.claudeUUID,
				SessionEnvPath:  "/tmp/session-env",
				FileHistoryPath: "/tmp/file-history",
				StartedAt:       time.Now(),
				LastActivity:    time.Now(),
			},
			Tmux: manifest.Tmux{
				SessionName: spec.tmuxName,
				WindowName:  "main",
				CreatedAt:   time.Now(),
			},
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			t.Fatalf("Failed to create test manifest: %v", err)
		}
	}

	// Temporarily override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	tests := []struct {
		name       string
		identifier string
		wantUUID   string
		wantError  bool
	}{
		{
			name:       "resolve by full UUID",
			identifier: testUUID1,
			wantUUID:   testUUID1,
			wantError:  false,
		},
		{
			name:       "resolve by UUID prefix",
			identifier: "c4eb298c",
			wantUUID:   testUUID1,
			wantError:  false,
		},
		{
			name:       "resolve by tmux name",
			identifier: "claude-1",
			wantUUID:   testUUID1,
			wantError:  false,
		},
		{
			name:       "resolve by project path fuzzy match",
			identifier: "workspace-design",
			wantUUID:   testUUID1,
			wantError:  false,
		},
		{
			name:       "resolve by session ID",
			identifier: "test-project",
			wantUUID:   testUUID2,
			wantError:  false,
		},
		{
			name:       "no match returns error",
			identifier: "nonexistent",
			wantUUID:   "",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUUID, gotPath, err := resolveSessionIdentifier(tt.identifier)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for identifier %q, got nil", tt.identifier)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if gotUUID != tt.wantUUID {
				t.Errorf("UUID = %q, want %q", gotUUID, tt.wantUUID)
			}

			if gotPath == "" {
				t.Error("Expected non-empty manifest path")
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple path",
			input: "/tmp/test",
			want:  "'/tmp/test'",
		},
		{
			name:  "path with spaces",
			input: "/tmp/test dir/file",
			want:  "'/tmp/test dir/file'",
		},
		{
			name:  "path with single quote",
			input: "/tmp/test'dir",
			want:  "'/tmp/test'\"'\"'dir'",
		},
		{
			name:  "path with multiple single quotes",
			input: "it's a test's path",
			want:  "'it'\"'\"'s a test'\"'\"'s path'",
		},
		{
			name:  "path with semicolon (command injection attempt)",
			input: "/tmp/test; rm -rf /",
			want:  "'/tmp/test; rm -rf /'",
		},
		{
			name:  "path with backticks (command substitution attempt)",
			input: "/tmp/`whoami`",
			want:  "'/tmp/`whoami`'",
		},
		{
			name:  "path with dollar sign (variable expansion attempt)",
			input: "/tmp/$HOME",
			want:  "'/tmp/$HOME'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUpdateManifestActivity(t *testing.T) {
	tmpDir := t.TempDir()
	manifestDir := filepath.Join(tmpDir, "test-session")
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		t.Fatalf("Failed to create manifest dir: %v", err)
	}

	// Create initial manifest
	oldTime := time.Now().Add(-24 * time.Hour)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "test-session",
		Status:        manifest.StatusActive,
		CreatedAt:     oldTime,
		LastActivity:  oldTime,
		Worktree: manifest.Worktree{
			Path:   "/tmp/test",
			Branch: "main",
			Repo:   "test-repo",
		},
		Claude: manifest.Claude{
			SessionID:       "c4eb298c-8c89-4f75-8dae-c725a1291add",
			SessionEnvPath:  "/tmp/session-env",
			FileHistoryPath: "/tmp/file-history",
			StartedAt:       oldTime,
			LastActivity:    oldTime,
		},
		Tmux: manifest.Tmux{
			SessionName: "test-tmux",
			WindowName:  "main",
			CreatedAt:   oldTime,
		},
	}

	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write initial manifest: %v", err)
	}

	// Update activity
	if err := updateManifestActivity(manifestPath); err != nil {
		t.Fatalf("updateManifestActivity failed: %v", err)
	}

	// Read back and verify
	updated, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read updated manifest: %v", err)
	}

	if !updated.LastActivity.After(oldTime) {
		t.Error("LastActivity was not updated")
	}

	if !updated.Claude.LastActivity.After(oldTime) {
		t.Error("Claude.LastActivity was not updated")
	}
}
