package claude

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHistory(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantCount   int
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid history with 648 entries",
			fixture:   "history-586.jsonl",
			wantCount: 647, // 647 valid entries (1 with null sessionId)
			wantErr:   false,
		},
		{
			name:      "empty file",
			fixture:   "history-empty.jsonl",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "malformed JSON lines",
			fixture:   "history-malformed.jsonl",
			wantCount: 2, // 2 valid, 3 malformed (skipped)
			wantErr:   false,
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

			entries, err := ParseHistory(path)

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
