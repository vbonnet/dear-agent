package analytics

import "time"

// WaitDetector detects wait time vs. AI time using heuristics
type WaitDetector struct {
	threshold time.Duration // Gap threshold (default: 5 minutes)
}

// NewWaitDetector creates detector with default threshold (5min)
func NewWaitDetector() *WaitDetector {
	return &WaitDetector{
		threshold: 5 * time.Minute,
	}
}

// NewWaitDetectorWithThreshold creates detector with custom threshold
func NewWaitDetectorWithThreshold(threshold time.Duration) *WaitDetector {
	return &WaitDetector{
		threshold: threshold,
	}
}

// DetectWaitTime analyzes phase transitions to separate wait vs. AI time
// Returns (ai_time, wait_time)
//
// Heuristic:
//
//	For each gap between phase end → next phase start:
//	  If gap > threshold (5min): Classify as WAIT TIME
//	  Else: Classify as AI TIME (quick transition)
//
//	AI Time = Sum(phase durations) + Sum(short gaps)
//	Wait Time = Sum(long gaps)
func (wd *WaitDetector) DetectWaitTime(phases []Phase) (time.Duration, time.Duration) {
	if len(phases) == 0 {
		return 0, 0
	}

	var aiTime time.Duration
	var waitTime time.Duration

	// Sum up all phase durations (this is AI actively working)
	for _, phase := range phases {
		aiTime += phase.Duration
	}

	// Analyze gaps between consecutive phases
	for i := 0; i < len(phases)-1; i++ {
		currentPhase := phases[i]
		nextPhase := phases[i+1]

		// Gap = time between phase end → next phase start
		gap := nextPhase.StartTime.Sub(currentPhase.EndTime)

		if gap < 0 {
			// Phases overlap (shouldn't happen, but handle gracefully)
			continue
		}

		if gap > wd.threshold {
			// Long gap = user wait time
			waitTime += gap
		} else {
			// Short gap = AI transition time (thinking, loading next phase)
			aiTime += gap
		}
	}

	return aiTime, waitTime
}
