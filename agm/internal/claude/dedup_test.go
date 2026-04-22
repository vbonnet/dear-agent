package claude

import (
	"fmt"
	"testing"
	"time"
)

func TestDeduplicate(t *testing.T) {
	tests := []struct {
		name    string
		entries []RawEntry
		want    int // expected session count
	}{
		{
			name: "648 entries with 17 unique UUIDs",
			entries: func() []RawEntry {
				// Load real test data if available
				entries, _, err := ParseHistory("testdata/history-586.jsonl")
				if err != nil {
					// Fallback to synthetic data
					return generateTestEntries(17, 38)
				}
				return entries
			}(),
			want: 17,
		},
		{
			name: "all entries same UUID",
			entries: []RawEntry{
				{SessionID: "uuid-1", Project: "/tmp/a", Timestamp: 1000},
				{SessionID: "uuid-1", Project: "/tmp/b", Timestamp: 2000},
				{SessionID: "uuid-1", Project: "/tmp/c", Timestamp: 3000},
			},
			want: 1,
		},
		{
			name: "each entry different UUID",
			entries: []RawEntry{
				{SessionID: "uuid-1", Project: "/tmp/a", Timestamp: 1000},
				{SessionID: "uuid-2", Project: "/tmp/b", Timestamp: 2000},
				{SessionID: "uuid-3", Project: "/tmp/c", Timestamp: 3000},
			},
			want: 3,
		},
		{
			name:    "empty input",
			entries: []RawEntry{},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := Deduplicate(tt.entries)

			if len(sessions) != tt.want {
				t.Errorf("got %d sessions, want %d", len(sessions), tt.want)
			}

			// Verify sessions are sorted by LastActivity (newest first)
			for i := 1; i < len(sessions); i++ {
				if sessions[i].LastActivity.After(sessions[i-1].LastActivity) {
					t.Errorf("sessions not sorted correctly: session %d is newer than session %d", i, i-1)
				}
			}
		})
	}
}

func TestCalculateStats(t *testing.T) {
	entries := []RawEntry{
		{SessionID: "uuid-1", Project: "/tmp/first", Timestamp: 1000},
		{SessionID: "uuid-1", Project: "/tmp/middle", Timestamp: 2000},
		{SessionID: "uuid-1", Project: "/tmp/last", Timestamp: 3000},
	}

	session := calculateStats("uuid-1", entries)

	// Verify UUID
	if session.UUID != "uuid-1" {
		t.Errorf("got UUID %q, want %q", session.UUID, "uuid-1")
	}

	// Verify project is from last entry
	if session.Project != "/tmp/last" {
		t.Errorf("got project %q, want %q", session.Project, "/tmp/last")
	}

	// Verify message count
	if session.MessageCount != 3 {
		t.Errorf("got message count %d, want 3", session.MessageCount)
	}

	// Verify timestamps
	expectedFirst := time.Unix(0, 1000*int64(time.Millisecond))
	expectedLast := time.Unix(0, 3000*int64(time.Millisecond))

	if !session.FirstActivity.Equal(expectedFirst) {
		t.Errorf("got first activity %v, want %v", session.FirstActivity, expectedFirst)
	}

	if !session.LastActivity.Equal(expectedLast) {
		t.Errorf("got last activity %v, want %v", session.LastActivity, expectedLast)
	}

	// Verify duration (2000ms = 2s = 2/3600 hours)
	expectedDuration := 2.0 / 3600.0 // hours
	if session.DurationHours != expectedDuration {
		t.Errorf("got duration %f hours, want %f hours", session.DurationHours, expectedDuration)
	}
}

// generateTestEntries creates synthetic test data with numUUIDs unique UUIDs
// and entriesPerUUID entries per UUID
func generateTestEntries(numUUIDs, entriesPerUUID int) []RawEntry {
	entries := make([]RawEntry, 0, numUUIDs*entriesPerUUID)
	baseTime := float64(time.Now().UnixNano() / int64(time.Millisecond))

	for i := 0; i < numUUIDs; i++ {
		uuid := fmt.Sprintf("uuid-%d", i+1)
		for j := 0; j < entriesPerUUID; j++ {
			entries = append(entries, RawEntry{
				SessionID: uuid,
				Project:   fmt.Sprintf("/tmp/project-%d", i+1),
				Timestamp: baseTime + float64(i*1000) + float64(j*100),
			})
		}
	}

	return entries
}
