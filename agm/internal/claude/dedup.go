package claude

import (
	"math"
	"sort"
	"time"
)

// Session represents a deduplicated Claude session with aggregated stats
type Session struct {
	UUID          string
	Project       string // Last project path
	FirstActivity time.Time
	LastActivity  time.Time
	MessageCount  int
	DurationHours float64
}

// Deduplicate groups entries by UUID and calculates session stats
func Deduplicate(entries []RawEntry) []Session {
	// Group by UUID
	grouped := make(map[string][]RawEntry)
	for _, e := range entries {
		grouped[e.SessionID] = append(grouped[e.SessionID], e)
	}

	// Calculate stats per session
	sessions := make([]Session, 0, len(grouped))
	for uuid, entryList := range grouped {
		session := calculateStats(uuid, entryList)
		sessions = append(sessions, session)
	}

	// Sort by last activity (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	return sessions
}

// calculateStats computes aggregated statistics for a session
func calculateStats(uuid string, entries []RawEntry) Session {
	var minTS, maxTS float64 = math.MaxFloat64, 0
	var lastProject string

	for _, e := range entries {
		if e.Timestamp < minTS {
			minTS = e.Timestamp
		}
		if e.Timestamp > maxTS {
			maxTS = e.Timestamp
			lastProject = e.Project // Use project from latest entry
		}
	}

	// Convert Unix milliseconds to time.Time
	first := time.Unix(0, int64(minTS)*int64(time.Millisecond))
	last := time.Unix(0, int64(maxTS)*int64(time.Millisecond))
	duration := last.Sub(first).Hours()

	return Session{
		UUID:          uuid,
		Project:       lastProject,
		FirstActivity: first,
		LastActivity:  last,
		MessageCount:  len(entries),
		DurationHours: duration,
	}
}
