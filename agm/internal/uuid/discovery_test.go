package uuid

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Test helpers

// createTempHistory creates a temporary history.jsonl file for testing
func createTempHistory(t *testing.T, entries []history.Entry) string {
	t.Helper()

	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	file, err := os.Create(historyPath)
	if err != nil {
		t.Fatalf("failed to create temp history file: %v", err)
	}
	defer file.Close()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("failed to marshal entry: %v", err)
		}
		fmt.Fprintf(file, "%s\n", data)
	}

	return historyPath
}

// TestSearchHistoryByRename tests the SearchHistoryByRename function
func TestSearchHistoryByRename(t *testing.T) {
	// Create sample history entries in NEW format (ConversationEntry)
	now := time.Now()
	entries := []history.ConversationEntry{
		{
			SessionID: "11111111-1111-1111-1111-111111111111",
			Display:   "/rename test-session",
			Timestamp: now.Add(-2 * time.Hour).UnixMilli(),
		},
		{
			SessionID: "22222222-2222-2222-2222-222222222222",
			Display:   "/rename test-session",
			Timestamp: now.Add(-1 * time.Hour).UnixMilli(), // More recent
		},
		{
			SessionID: "33333333-3333-3333-3333-333333333333",
			Display:   "/rename other-session",
			Timestamp: now.UnixMilli(),
		},
		{
			SessionID: "44444444-4444-4444-4444-444444444444",
			Display:   "/rename trailing-space-session ", // Trailing space (common user input)
			Timestamp: now.Add(-30 * time.Minute).UnixMilli(),
		},
	}

	// Save original history parser behavior
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create .claude directory structure
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Create history file
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	file, err := os.Create(historyPath)
	if err != nil {
		t.Fatalf("failed to create history file: %v", err)
	}
	for _, entry := range entries {
		data, _ := json.Marshal(entry)
		fmt.Fprintf(file, "%s\n", data)
	}
	file.Close()

	tests := []struct {
		name        string
		sessionName string
		wantUUID    string
		wantErr     bool
	}{
		{
			name:        "single match",
			sessionName: "other-session",
			wantUUID:    "33333333-3333-3333-3333-333333333333",
			wantErr:     false,
		},
		{
			name:        "multiple matches - returns most recent",
			sessionName: "test-session",
			wantUUID:    "22222222-2222-2222-2222-222222222222",
			wantErr:     false,
		},
		{
			name:        "no match found",
			sessionName: "nonexistent-session",
			wantUUID:    "",
			wantErr:     true,
		},
		{
			name:        "empty session name",
			sessionName: "",
			wantUUID:    "",
			wantErr:     true,
		},
		{
			name:        "trailing whitespace in display field - should match",
			sessionName: "trailing-space-session",
			wantUUID:    "44444444-4444-4444-4444-444444444444",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUUID, err := SearchHistoryByRename(tt.sessionName)

			if (err != nil) != tt.wantErr {
				t.Errorf("SearchHistoryByRename() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotUUID != tt.wantUUID {
				t.Errorf("SearchHistoryByRename() = %v, want %v", gotUUID, tt.wantUUID)
			}
		})
	}
}

// TestSearchHistoryByTimestamp tests the SearchHistoryByTimestamp function
func TestSearchHistoryByTimestamp(t *testing.T) {
	now := time.Now()
	entries := []history.ConversationEntry{
		{
			SessionID: "11111111-1111-1111-1111-111111111111",
			Display:   "some command",
			Timestamp: now.Add(-20 * time.Minute).UnixMilli(), // Outside window
		},
		{
			SessionID: "22222222-2222-2222-2222-222222222222",
			Display:   "some command",
			Timestamp: now.Add(-5 * time.Minute).UnixMilli(), // Within window
		},
		{
			SessionID: "33333333-3333-3333-3333-333333333333",
			Display:   "some command",
			Timestamp: now.Add(5 * time.Minute).UnixMilli(), // Within window
		},
	}

	// Setup temp HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	os.MkdirAll(claudeDir, 0755)
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	file, _ := os.Create(historyPath)
	for _, entry := range entries {
		data, _ := json.Marshal(entry)
		fmt.Fprintf(file, "%s\n", data)
	}
	file.Close()

	tests := []struct {
		name          string
		timestamp     time.Time
		windowMinutes int
		wantUUID      string
		wantErr       bool
	}{
		{
			name:          "match found in window",
			timestamp:     now,
			windowMinutes: 10,
			wantUUID:      "33333333-3333-3333-3333-333333333333", // Returns first session found (newest in file order)
			wantErr:       false,
		},
		{
			name:          "no match - outside window",
			timestamp:     now,
			windowMinutes: 2,
			wantUUID:      "",
			wantErr:       true,
		},
		{
			name:          "zero timestamp",
			timestamp:     time.Time{},
			windowMinutes: 10,
			wantUUID:      "",
			wantErr:       true,
		},
		{
			name:          "invalid window - uses default",
			timestamp:     now,
			windowMinutes: 0,
			wantUUID:      "33333333-3333-3333-3333-333333333333",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUUID, err := SearchHistoryByTimestamp(tt.timestamp, tt.windowMinutes)

			if (err != nil) != tt.wantErr {
				t.Errorf("SearchHistoryByTimestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotUUID != tt.wantUUID {
				t.Errorf("SearchHistoryByTimestamp() = %v, want %v", gotUUID, tt.wantUUID)
			}
		})
	}
}

// TestFindMostRecentJSONL tests the FindMostRecentJSONL function
func TestFindMostRecentJSONL(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string // Returns project path
		wantUUID string
		wantErr  bool
	}{
		{
			name: "single JSONL file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				os.WriteFile(filepath.Join(dir, "11111111-1111-1111-1111-111111111111.jsonl"), []byte("{}"), 0644)
				return dir
			},
			wantUUID: "11111111-1111-1111-1111-111111111111",
			wantErr:  false,
		},
		{
			name: "multiple files - returns most recent",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				old := filepath.Join(dir, "11111111-1111-1111-1111-111111111111.jsonl")
				recent := filepath.Join(dir, "22222222-2222-2222-2222-222222222222.jsonl")

				os.WriteFile(old, []byte("{}"), 0644)
				time.Sleep(10 * time.Millisecond)
				os.WriteFile(recent, []byte("{}"), 0644)

				return dir
			},
			wantUUID: "22222222-2222-2222-2222-222222222222",
			wantErr:  false,
		},
		{
			name: "no JSONL files",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("test"), 0644)
				return dir
			},
			wantUUID: "",
			wantErr:  true,
		},
		{
			name: "directory doesn't exist",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/12345"
			},
			wantUUID: "",
			wantErr:  true,
		},
		{
			name: "invalid UUID in filename",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				os.WriteFile(filepath.Join(dir, "not-a-uuid.jsonl"), []byte("{}"), 0644)
				return dir
			},
			wantUUID: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectPath := tt.setup(t)
			gotUUID, err := FindMostRecentJSONL(projectPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("FindMostRecentJSONL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotUUID != tt.wantUUID {
				t.Errorf("FindMostRecentJSONL() = %v, want %v", gotUUID, tt.wantUUID)
			}
		})
	}
}

// TestDiscover tests the Discover orchestrator function
func TestDiscover(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name               string
		sessionName        string
		manifestSearchFunc func(string) (*manifest.Manifest, error)
		setupHistory       func(t *testing.T) // Setup history.jsonl
		verbose            bool
		wantUUID           string
		wantErr            bool
		wantErrContains    string
	}{
		{
			name:        "Level 1 succeeds - manifest has UUID",
			sessionName: "test-session",
			manifestSearchFunc: func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "11111111-1111-1111-1111-111111111111",
					},
				}, nil
			},
			setupHistory: func(t *testing.T) {
				// Need to setup history with rename for verification
				originalHome := os.Getenv("HOME")
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Cleanup(func() { t.Setenv("HOME", originalHome) })

				claudeDir := filepath.Join(tmpHome, ".claude")
				os.MkdirAll(claudeDir, 0755)
				historyPath := filepath.Join(claudeDir, "history.jsonl")
				file, _ := os.Create(historyPath)
				entry := history.ConversationEntry{
					SessionID: "11111111-1111-1111-1111-111111111111",
					Display:   "/rename test-session",
					Timestamp: now.UnixMilli(),
				}
				data, _ := json.Marshal(entry)
				fmt.Fprintf(file, "%s\n", data)
				file.Close()
			},
			verbose:  false,
			wantUUID: "11111111-1111-1111-1111-111111111111",
			wantErr:  false,
		},
		{
			name:        "Level 2a succeeds - found by rename",
			sessionName: "renamed-session",
			manifestSearchFunc: func(name string) (*manifest.Manifest, error) {
				return nil, fmt.Errorf("not found")
			},
			setupHistory: func(t *testing.T) {
				originalHome := os.Getenv("HOME")
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Cleanup(func() { t.Setenv("HOME", originalHome) })

				claudeDir := filepath.Join(tmpHome, ".claude")
				os.MkdirAll(claudeDir, 0755)
				historyPath := filepath.Join(claudeDir, "history.jsonl")
				file, _ := os.Create(historyPath)
				entry := history.ConversationEntry{
					SessionID: "22222222-2222-2222-2222-222222222222",
					Display:   "/rename renamed-session",
					Timestamp: now.UnixMilli(),
				}
				data, _ := json.Marshal(entry)
				fmt.Fprintf(file, "%s\n", data)
				file.Close()
			},
			verbose:  false,
			wantUUID: "22222222-2222-2222-2222-222222222222",
			wantErr:  false,
		},
		{
			name:        "All levels fail",
			sessionName: "nonexistent",
			manifestSearchFunc: func(name string) (*manifest.Manifest, error) {
				return nil, fmt.Errorf("not found")
			},
			setupHistory: func(t *testing.T) {
				originalHome := os.Getenv("HOME")
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Cleanup(func() { t.Setenv("HOME", originalHome) })

				claudeDir := filepath.Join(tmpHome, ".claude")
				os.MkdirAll(claudeDir, 0755)
				historyPath := filepath.Join(claudeDir, "history.jsonl")
				os.WriteFile(historyPath, []byte(""), 0644) // Empty history
			},
			verbose:         false,
			wantUUID:        "",
			wantErr:         true,
			wantErrContains: "UUID discovery failed",
		},
		{
			name:        "Manifest UUID without /rename verification - trusts manifest",
			sessionName: "unverified-session",
			manifestSearchFunc: func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "44444444-4444-4444-4444-444444444444",
					},
				}, nil
			},
			setupHistory: func(t *testing.T) {
				// Create empty history (no /rename exists)
				originalHome := os.Getenv("HOME")
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Cleanup(func() { t.Setenv("HOME", originalHome) })

				claudeDir := filepath.Join(tmpHome, ".claude")
				os.MkdirAll(claudeDir, 0755)
				historyPath := filepath.Join(claudeDir, "history.jsonl")
				os.WriteFile(historyPath, []byte(""), 0644)
			},
			verbose:  false,
			wantUUID: "44444444-4444-4444-4444-444444444444", // Should trust manifest
			wantErr:  false,
		},
		{
			name:        "Manifest UUID mismatch with /rename - trusts /rename (BUG FIX)",
			sessionName: "mismatched-session",
			manifestSearchFunc: func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "wrong-uuid-0000-0000-0000-000000000000", // Wrong UUID in manifest
					},
				}, nil
			},
			setupHistory: func(t *testing.T) {
				// Create history with /rename pointing to different UUID
				originalHome := os.Getenv("HOME")
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Cleanup(func() { t.Setenv("HOME", originalHome) })

				claudeDir := filepath.Join(tmpHome, ".claude")
				os.MkdirAll(claudeDir, 0755)
				historyPath := filepath.Join(claudeDir, "history.jsonl")
				file, _ := os.Create(historyPath)
				entry := history.ConversationEntry{
					SessionID: "55555555-5555-5555-5555-555555555555", // Correct UUID
					Display:   "/rename mismatched-session",
					Timestamp: now.UnixMilli(),
				}
				data, _ := json.Marshal(entry)
				fmt.Fprintf(file, "%s\n", data)
				file.Close()
			},
			verbose:  false,
			wantUUID: "55555555-5555-5555-5555-555555555555", // Should trust /rename, not manifest
			wantErr:  false,
		},
		{
			name:        "Manifest exists but empty UUID - fails without timestamp fallback",
			sessionName: "empty-uuid-session",
			manifestSearchFunc: func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "", // Empty UUID (like which-vesion before fix)
					},
					UpdatedAt: now,
				}, nil
			},
			setupHistory: func(t *testing.T) {
				// Create history with entries around manifest time, but NO /rename
				originalHome := os.Getenv("HOME")
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Cleanup(func() { t.Setenv("HOME", originalHome) })

				claudeDir := filepath.Join(tmpHome, ".claude")
				os.MkdirAll(claudeDir, 0755)
				historyPath := filepath.Join(claudeDir, "history.jsonl")
				file, _ := os.Create(historyPath)
				// Add entry within timestamp window but different session name
				entry := history.ConversationEntry{
					SessionID: "wrong-session-6666-6666-6666-666666666666",
					Display:   "/rename other-session", // Different session
					Timestamp: now.Add(-5 * time.Minute).UnixMilli(),
				}
				data, _ := json.Marshal(entry)
				fmt.Fprintf(file, "%s\n", data)
				file.Close()
			},
			verbose:         false,
			wantUUID:        "",
			wantErr:         true,
			wantErrContains: "UUID discovery failed", // Should fail gracefully, not return wrong UUID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupHistory != nil {
				tt.setupHistory(t)
			}

			gotUUID, err := Discover(tt.sessionName, tt.manifestSearchFunc, tt.verbose)

			if (err != nil) != tt.wantErr {
				t.Errorf("Discover() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("Discover() error = %v, want error containing %q", err, tt.wantErrContains)
				}
			}

			if gotUUID != tt.wantUUID {
				t.Errorf("Discover() = %v, want %v", gotUUID, tt.wantUUID)
			}
		})
	}
}
