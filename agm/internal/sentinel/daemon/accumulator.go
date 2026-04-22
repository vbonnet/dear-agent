package daemon

import (
	"sort"
	"sync"
	"time"
)

// PatternAccumulator tracks pattern violations per session over sliding time windows.
// Thread-safe for concurrent access from multiple monitoring goroutines.
type PatternAccumulator struct {
	mu       sync.Mutex
	sessions map[string]*SessionViolations
	window   time.Duration
}

// SessionViolations holds accumulated violations for a single session.
type SessionViolations struct {
	Violations []AccumulatedViolation
}

// AccumulatedViolation records a single violation occurrence.
type AccumulatedViolation struct {
	PatternID string
	Severity  string
	Command   string
	Timestamp time.Time
}

// PatternFrequency pairs a pattern ID with its occurrence count.
type PatternFrequency struct {
	PatternID string
	Count     int
}

// NewPatternAccumulator creates a new accumulator with the given sliding window duration.
func NewPatternAccumulator(window time.Duration) *PatternAccumulator {
	return &PatternAccumulator{
		sessions: make(map[string]*SessionViolations),
		window:   window,
	}
}

// Record adds a violation to the accumulator for the given session.
func (a *PatternAccumulator) Record(sessionName, patternID, severity, command string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	sv, exists := a.sessions[sessionName]
	if !exists {
		sv = &SessionViolations{}
		a.sessions[sessionName] = sv
	}

	sv.Violations = append(sv.Violations, AccumulatedViolation{
		PatternID: patternID,
		Severity:  severity,
		Command:   command,
		Timestamp: time.Now(),
	})
}

// GetFrequency returns the number of times a specific pattern was violated
// within the sliding window for the given session.
func (a *PatternAccumulator) GetFrequency(sessionName, patternID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	sv, exists := a.sessions[sessionName]
	if !exists {
		return 0
	}

	cutoff := time.Now().Add(-a.window)
	count := 0
	for _, v := range sv.Violations {
		if v.PatternID == patternID && v.Timestamp.After(cutoff) {
			count++
		}
	}
	return count
}

// GetSessionTotal returns the total number of violations within the sliding
// window for the given session.
func (a *PatternAccumulator) GetSessionTotal(sessionName string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	sv, exists := a.sessions[sessionName]
	if !exists {
		return 0
	}

	cutoff := time.Now().Add(-a.window)
	count := 0
	for _, v := range sv.Violations {
		if v.Timestamp.After(cutoff) {
			count++
		}
	}
	return count
}

// GetTopPatterns returns the n most frequent patterns within the sliding window
// for the given session, sorted by count descending.
func (a *PatternAccumulator) GetTopPatterns(sessionName string, n int) []PatternFrequency {
	a.mu.Lock()
	defer a.mu.Unlock()

	sv, exists := a.sessions[sessionName]
	if !exists {
		return nil
	}

	cutoff := time.Now().Add(-a.window)
	counts := make(map[string]int)
	for _, v := range sv.Violations {
		if v.Timestamp.After(cutoff) {
			counts[v.PatternID]++
		}
	}

	freqs := make([]PatternFrequency, 0, len(counts))
	for id, c := range counts {
		freqs = append(freqs, PatternFrequency{PatternID: id, Count: c})
	}

	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i].Count > freqs[j].Count
	})

	if n > len(freqs) {
		n = len(freqs)
	}
	return freqs[:n]
}

// ShouldEscalate returns true if the total violations within the sliding window
// for the given session meets or exceeds the threshold.
func (a *PatternAccumulator) ShouldEscalate(sessionName string, threshold int) bool {
	return a.GetSessionTotal(sessionName) >= threshold
}

// GetCrossSessionCount returns the number of distinct sessions that have
// recorded violations for the given patternID within the sliding window.
func (a *PatternAccumulator) GetCrossSessionCount(patternID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-a.window)
	count := 0
	for _, sv := range a.sessions {
		for _, v := range sv.Violations {
			if v.PatternID == patternID && v.Timestamp.After(cutoff) {
				count++
				break // one hit per session is enough
			}
		}
	}
	return count
}

// GetCrossSessionPatterns returns all pattern IDs that appear in at least
// minSessions distinct sessions within the sliding window.
func (a *PatternAccumulator) GetCrossSessionPatterns(minSessions int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-a.window)
	sessionSets := make(map[string]map[string]bool)
	for name, sv := range a.sessions {
		for _, v := range sv.Violations {
			if v.Timestamp.After(cutoff) {
				if sessionSets[v.PatternID] == nil {
					sessionSets[v.PatternID] = make(map[string]bool)
				}
				sessionSets[v.PatternID][name] = true
			}
		}
	}

	var result []string
	for pid, sessions := range sessionSets {
		if len(sessions) >= minSessions {
			result = append(result, pid)
		}
	}
	sort.Strings(result)
	return result
}

// Cleanup removes violations outside the sliding window across all sessions.
// Removes empty sessions entirely.
func (a *PatternAccumulator) Cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-a.window)
	for name, sv := range a.sessions {
		kept := sv.Violations[:0]
		for _, v := range sv.Violations {
			if v.Timestamp.After(cutoff) {
				kept = append(kept, v)
			}
		}
		if len(kept) == 0 {
			delete(a.sessions, name)
		} else {
			sv.Violations = kept
		}
	}
}
