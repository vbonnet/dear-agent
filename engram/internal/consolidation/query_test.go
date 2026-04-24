package consolidation

import (
	"testing"
	"time"
)

func TestQuery_Defaults(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		want  struct {
			limit         int
			minImportance float64
		}
	}{
		{
			name:  "empty query uses defaults",
			query: Query{},
			want: struct {
				limit         int
				minImportance float64
			}{limit: 0, minImportance: 0},
		},
		{
			name: "explicit limit",
			query: Query{
				Limit: 10,
			},
			want: struct {
				limit         int
				minImportance float64
			}{limit: 10, minImportance: 0},
		},
		{
			name: "explicit min importance",
			query: Query{
				MinImportance: 0.8,
			},
			want: struct {
				limit         int
				minImportance float64
			}{limit: 0, minImportance: 0.8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.query.Limit != tt.want.limit {
				t.Errorf("Limit = %d, want %d", tt.query.Limit, tt.want.limit)
			}
			if tt.query.MinImportance != tt.want.minImportance {
				t.Errorf("MinImportance = %f, want %f", tt.query.MinImportance, tt.want.minImportance)
			}
		})
	}
}

func TestTimeRange_Validation(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	tests := []struct {
		name      string
		timeRange TimeRange
		testTime  time.Time
		want      bool // Should time be in range?
	}{
		{
			name: "time within range",
			timeRange: TimeRange{
				Start: yesterday,
				End:   tomorrow,
			},
			testTime: now,
			want:     true,
		},
		{
			name: "time before range",
			timeRange: TimeRange{
				Start: now,
				End:   tomorrow,
			},
			testTime: yesterday,
			want:     false,
		},
		{
			name: "time after range",
			timeRange: TimeRange{
				Start: yesterday,
				End:   now,
			},
			testTime: tomorrow,
			want:     false,
		},
		{
			name: "time at start boundary (inclusive)",
			timeRange: TimeRange{
				Start: now,
				End:   tomorrow,
			},
			testTime: now,
			want:     true,
		},
		{
			name: "time at end boundary (exclusive)",
			timeRange: TimeRange{
				Start: yesterday,
				End:   now,
			},
			testTime: now,
			want:     false, // End is exclusive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inRange := !tt.testTime.Before(tt.timeRange.Start) && tt.testTime.Before(tt.timeRange.End)
			if inRange != tt.want {
				t.Errorf("Time %v in range [%v, %v) = %v, want %v",
					tt.testTime, tt.timeRange.Start, tt.timeRange.End, inRange, tt.want)
			}
		})
	}
}
