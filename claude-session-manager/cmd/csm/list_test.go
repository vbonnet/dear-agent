package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
)

func TestSortSessions(t *testing.T) {
	// Create test sessions with known values
	sessions := []claude.Session{
		{
			UUID:          "uuid-1",
			Project:       "/tmp/a",
			MessageCount:  10,
			DurationHours: 5.0,
			LastActivity:  time.Date(2025, 12, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			UUID:          "uuid-2",
			Project:       "/tmp/b",
			MessageCount:  20,
			DurationHours: 3.0,
			LastActivity:  time.Date(2025, 12, 3, 10, 0, 0, 0, time.UTC),
		},
		{
			UUID:          "uuid-3",
			Project:       "/tmp/c",
			MessageCount:  5,
			DurationHours: 10.0,
			LastActivity:  time.Date(2025, 12, 2, 10, 0, 0, 0, time.UTC),
		},
	}

	tests := []struct {
		name          string
		sortBy        string
		wantFirstUUID string
		wantLastUUID  string
	}{
		{
			name:          "sort by activity (newest first)",
			sortBy:        "activity",
			wantFirstUUID: "uuid-2", // Dec 3 (newest)
			wantLastUUID:  "uuid-1", // Dec 1 (oldest)
		},
		{
			name:          "sort by messages (most first)",
			sortBy:        "messages",
			wantFirstUUID: "uuid-2", // 20 messages
			wantLastUUID:  "uuid-3", // 5 messages
		},
		{
			name:          "sort by duration (longest first)",
			sortBy:        "duration",
			wantFirstUUID: "uuid-3", // 10.0 hours
			wantLastUUID:  "uuid-2", // 3.0 hours
		},
		{
			name:          "empty string defaults to activity",
			sortBy:        "",
			wantFirstUUID: "uuid-2",
			wantLastUUID:  "uuid-1",
		},
		{
			name:          "invalid sort key defaults to activity",
			sortBy:        "invalid",
			wantFirstUUID: "uuid-2",
			wantLastUUID:  "uuid-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original
			testSessions := make([]claude.Session, len(sessions))
			copy(testSessions, sessions)

			sortSessions(testSessions, tt.sortBy)

			if testSessions[0].UUID != tt.wantFirstUUID {
				t.Errorf("first session UUID = %q, want %q", testSessions[0].UUID, tt.wantFirstUUID)
			}

			lastIdx := len(testSessions) - 1
			if testSessions[lastIdx].UUID != tt.wantLastUUID {
				t.Errorf("last session UUID = %q, want %q", testSessions[lastIdx].UUID, tt.wantLastUUID)
			}
		})
	}
}

func TestFormatSessionsTable(t *testing.T) {
	// Empty tmux mapping and active set for basic tests
	emptyTmuxMapping := make(map[string]string)
	emptyActiveTmux := make(map[string]bool)

	tests := []struct {
		name     string
		sessions []claude.Session
		want     []string // Strings that should appear in output
		notWant  []string // Strings that should NOT appear in output
	}{
		{
			name: "single session",
			sessions: []claude.Session{
				{
					UUID:          "test-uuid-123",
					Project:       "/tmp/test",
					MessageCount:  10,
					DurationHours: 5.5,
					LastActivity:  time.Date(2025, 12, 5, 15, 30, 0, 0, time.UTC),
				},
			},
			want: []string{
				"UUID",
				"TMUX",
				"PROJECT",
				"MESSAGES",
				"LAST ACTIVITY",
				"test-uui", // Truncated UUID
				"-",        // No tmux mapping
				"/tmp/test",
				"10",
				"2025-12-05 15:30",
			},
		},
		{
			name: "long UUID truncated",
			sessions: []claude.Session{
				{
					UUID:          "very-long-uuid-that-exceeds-8-characters",
					Project:       "/tmp/test",
					MessageCount:  1,
					DurationHours: 0.1,
					LastActivity:  time.Now(),
				},
			},
			want: []string{
				"very-lon", // First 8 chars
			},
			notWant: []string{
				"very-long-uuid-that-exceeds-8-characters", // Full UUID should not appear
			},
		},
		{
			name: "long project path truncated with ellipsis",
			sessions: []claude.Session{
				{
					UUID:          "uuid-1",
					Project:       "/very/long/path/that/exceeds/forty/characters/in/length/test",
					MessageCount:  1,
					DurationHours: 0.1,
					LastActivity:  time.Now(),
				},
			},
			want: []string{
				"...",          // Ellipsis for truncation
				"/length/test", // End of path
			},
		},
		{
			name:     "empty sessions",
			sessions: []claude.Session{},
			want: []string{
				"UUID",
				"PROJECT",
				"MESSAGES",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatSessionsTable(tt.sessions, emptyTmuxMapping, emptyActiveTmux)

			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("output should contain %q, got:\n%s", want, output)
				}
			}

			for _, notWant := range tt.notWant {
				if strings.Contains(output, notWant) {
					t.Errorf("output should NOT contain %q, got:\n%s", notWant, output)
				}
			}

			// Verify header is present
			if !strings.Contains(output, "UUID") || !strings.Contains(output, "TMUX") || !strings.Contains(output, "PROJECT") {
				t.Errorf("output missing header, got:\n%s", output)
			}
		})
	}
}

func TestFormatSessionsTable_ColumnAlignment(t *testing.T) {
	emptyTmuxMapping := make(map[string]string)
	emptyActiveTmux := make(map[string]bool)

	sessions := []claude.Session{
		{
			UUID:          "uuid-1",
			Project:       "/tmp/a",
			MessageCount:  10,
			DurationHours: 5.5,
			LastActivity:  time.Date(2025, 12, 5, 15, 30, 0, 0, time.UTC),
		},
		{
			UUID:          "uuid-2",
			Project:       "/tmp/b",
			MessageCount:  999,
			DurationHours: 100.1,
			LastActivity:  time.Date(2025, 12, 5, 16, 45, 0, 0, time.UTC),
		},
	}

	output := formatSessionsTable(sessions, emptyTmuxMapping, emptyActiveTmux)
	lines := strings.Split(output, "\n")

	// Should have at least header + separator + 2 data rows
	if len(lines) < 4 {
		t.Errorf("expected at least 4 lines, got %d", len(lines))
	}

	// All lines (except last empty) should have roughly similar length due to alignment
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
			if len(line) < 50 { // Reasonable minimum for formatted table
				t.Errorf("line too short, might not be properly formatted: %q", line)
			}
		}
	}

	if nonEmptyLines < 4 {
		t.Errorf("expected at least 4 non-empty lines, got %d", nonEmptyLines)
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are defined and sensible
	tests := []struct {
		name  string
		value int
		min   int
		max   int
	}{
		{"uuidDisplayLen", uuidDisplayLen, 6, 12},
		{"projectMaxLen", projectMaxLen, 20, 100},
		{"messagesColWidth", messagesColWidth, 6, 12},
		{"durationColWidth", durationColWidth, 8, 15},
		{"tmuxColWidth", tmuxColWidth, 10, 20},
		{"recentDays", recentDays, 7, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value < tt.min || tt.value > tt.max {
				t.Errorf("%s = %d, want between %d and %d", tt.name, tt.value, tt.min, tt.max)
			}
		})
	}
}

func TestFilterRecentSessions(t *testing.T) {
	now := time.Now()
	sessions := []claude.Session{
		{
			UUID:         "recent-1",
			LastActivity: now.AddDate(0, 0, -5), // 5 days ago
		},
		{
			UUID:         "recent-2",
			LastActivity: now.AddDate(0, 0, -10), // 10 days ago
		},
		{
			UUID:         "old-1",
			LastActivity: now.AddDate(0, 0, -35), // 35 days ago
		},
		{
			UUID:         "old-2",
			LastActivity: now.AddDate(0, 0, -100), // 100 days ago
		},
	}

	tests := []struct {
		name      string
		days      int
		wantCount int
		wantUUIDs []string
	}{
		{
			name:      "last 7 days",
			days:      7,
			wantCount: 1,
			wantUUIDs: []string{"recent-1"},
		},
		{
			name:      "last 30 days",
			days:      30,
			wantCount: 2,
			wantUUIDs: []string{"recent-1", "recent-2"},
		},
		{
			name:      "last 60 days",
			days:      60,
			wantCount: 3,
			wantUUIDs: []string{"recent-1", "recent-2", "old-1"},
		},
		{
			name:      "last 365 days",
			days:      365,
			wantCount: 4,
			wantUUIDs: []string{"recent-1", "recent-2", "old-1", "old-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRecentSessions(sessions, tt.days)

			if len(result) != tt.wantCount {
				t.Errorf("got %d sessions, want %d", len(result), tt.wantCount)
			}

			for _, wantUUID := range tt.wantUUIDs {
				found := false
				for _, s := range result {
					if s.UUID == wantUUID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find session %q in results", wantUUID)
				}
			}
		})
	}
}

func TestFormatSessionsTable_WithTmux(t *testing.T) {
	sessions := []claude.Session{
		{
			UUID:         "uuid-with-active-tmux",
			Project:      "/tmp/test1",
			MessageCount: 10,
			LastActivity: time.Date(2025, 12, 5, 15, 30, 0, 0, time.UTC),
		},
		{
			UUID:         "uuid-with-inactive-tmux",
			Project:      "/tmp/test2",
			MessageCount: 20,
			LastActivity: time.Date(2025, 12, 5, 16, 30, 0, 0, time.UTC),
		},
		{
			UUID:         "uuid-without-tmux",
			Project:      "/tmp/test3",
			MessageCount: 30,
			LastActivity: time.Date(2025, 12, 5, 17, 30, 0, 0, time.UTC),
		},
	}

	tmuxMapping := map[string]string{
		"uuid-with-active-tmux":   "claude-1",
		"uuid-with-inactive-tmux": "claude-2",
	}

	activeTmux := map[string]bool{
		"claude-1": true,
		// claude-2 is not active
	}

	output := formatSessionsTable(sessions, tmuxMapping, activeTmux)

	// Should show claude-1 with checkmark
	if !strings.Contains(output, "claude-1 ✓") {
		t.Error("expected active tmux session to show checkmark")
	}

	// Should show claude-2 without checkmark
	if !strings.Contains(output, "claude-2") {
		t.Error("expected inactive tmux session name to appear")
	}
	if strings.Contains(output, "claude-2 ✓") {
		t.Error("expected inactive tmux session NOT to have checkmark")
	}

	// Should show "-" for session without tmux
	lines := strings.Split(output, "\n")
	foundDash := false
	for _, line := range lines {
		if strings.Contains(line, "uuid-wit") && strings.Contains(line, "-") {
			foundDash = true
			break
		}
	}
	if !foundDash {
		t.Error("expected session without tmux to show '-'")
	}
}
