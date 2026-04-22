package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHistory(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantCount   int
		wantStats   *ParseStats
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid history with 648 entries",
			fixture:   "history-586.jsonl",
			wantCount: 647, // 647 valid entries (1 with null sessionId)
			wantStats: &ParseStats{
				TotalLines:   648,
				ValidEntries: 647,
				SkippedEmpty: 1,
			},
			wantErr: false,
		},
		{
			name:      "empty file",
			fixture:   "history-empty.jsonl",
			wantCount: 0,
			wantStats: &ParseStats{
				TotalLines:   0,
				ValidEntries: 0,
			},
			wantErr: false,
		},
		{
			name:      "malformed JSON lines",
			fixture:   "history-malformed.jsonl",
			wantCount: 2, // 2 valid, 3 malformed (skipped)
			wantStats: &ParseStats{
				TotalLines:    5,
				ValidEntries:  2,
				SkippedEmpty:  1, // One empty sessionId
				SkippedErrors: 2, // Two malformed lines
			},
			wantErr: false,
		},
		{
			name:        "file does not exist",
			fixture:     "nonexistent.jsonl",
			wantCount:   0,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("testdata", tt.fixture)

			entries, stats, err := ParseHistory(path)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(entries) != tt.wantCount {
				t.Errorf("got %d entries, want %d", len(entries), tt.wantCount)
			}

			// Verify stats if provided
			if tt.wantStats != nil {
				if stats.TotalLines != tt.wantStats.TotalLines {
					t.Errorf("stats.TotalLines = %d, want %d", stats.TotalLines, tt.wantStats.TotalLines)
				}
				if stats.ValidEntries != tt.wantStats.ValidEntries {
					t.Errorf("stats.ValidEntries = %d, want %d", stats.ValidEntries, tt.wantStats.ValidEntries)
				}
				if stats.SkippedEmpty != tt.wantStats.SkippedEmpty {
					t.Errorf("stats.SkippedEmpty = %d, want %d", stats.SkippedEmpty, tt.wantStats.SkippedEmpty)
				}
				if stats.SkippedErrors != tt.wantStats.SkippedErrors {
					t.Errorf("stats.SkippedErrors = %d, want %d", stats.SkippedErrors, tt.wantStats.SkippedErrors)
				}
			}

			// Verify entries have required fields
			for i, e := range entries {
				if e.SessionID == "" {
					t.Errorf("entry %d has empty SessionID", i)
				}
				if e.Timestamp == 0 {
					t.Errorf("entry %d has zero Timestamp", i)
				}
			}
		})
	}
}

func TestParseHistory_PermissionDenied(t *testing.T) {
	// Create a file with no read permissions
	tmpFile := filepath.Join(t.TempDir(), "noperm.jsonl")
	if err := os.WriteFile(tmpFile, []byte(`{"sessionId":"test","timestamp":1000}`), 0000); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, _, err := ParseHistory(tmpFile)
	if err == nil {
		t.Fatal("expected permission denied error, got nil")
	}

	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should contain 'permission denied', got: %v", err)
	}
}
