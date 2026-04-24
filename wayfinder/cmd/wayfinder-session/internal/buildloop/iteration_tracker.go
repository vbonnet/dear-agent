package buildloop

import (
	"sync"
	"time"
)

// IterationTracker tracks iterations for a single task
type IterationTracker struct {
	taskID       string
	currentIter  int
	iterations   []Iteration
	stateVisits  map[State]int
	testRunCount int
	startTime    time.Time
	mu           sync.RWMutex
}

// Iteration represents a single iteration through the BUILD loop
type Iteration struct {
	Number        int
	StartTime     time.Time
	EndTime       *time.Time
	StatesVisited []State
	Success       bool
	Error         string
}

// NewIterationTracker creates a new iteration tracker
func NewIterationTracker(taskID string) *IterationTracker {
	return &IterationTracker{
		taskID:      taskID,
		currentIter: 0,
		iterations:  make([]Iteration, 0),
		stateVisits: make(map[State]int),
		startTime:   time.Now(),
	}
}

// StartIteration begins a new iteration
func (it *IterationTracker) StartIteration() {
	it.mu.Lock()
	defer it.mu.Unlock()

	it.currentIter++
	iter := Iteration{
		Number:        it.currentIter,
		StartTime:     time.Now(),
		StatesVisited: make([]State, 0),
		Success:       false,
	}
	it.iterations = append(it.iterations, iter)
}

// RecordState records a state visit in the current iteration
func (it *IterationTracker) RecordState(state State) {
	it.mu.Lock()
	defer it.mu.Unlock()

	if len(it.iterations) == 0 {
		return
	}

	// Record in current iteration
	idx := len(it.iterations) - 1
	it.iterations[idx].StatesVisited = append(it.iterations[idx].StatesVisited, state)

	// Track state visit count
	it.stateVisits[state]++

	// Track test runs
	if state == StateTestFirst || state == StateCoding || state == StateGreen {
		it.testRunCount++
	}
}

// CompleteIteration marks the current iteration as complete
func (it *IterationTracker) CompleteIteration(success bool, err error) {
	it.mu.Lock()
	defer it.mu.Unlock()

	if len(it.iterations) == 0 {
		return
	}

	idx := len(it.iterations) - 1
	endTime := time.Now()
	it.iterations[idx].EndTime = &endTime
	it.iterations[idx].Success = success

	if err != nil {
		it.iterations[idx].Error = err.Error()
	}
}

// GetCurrentIteration returns the current iteration number
func (it *IterationTracker) GetCurrentIteration() int {
	it.mu.RLock()
	defer it.mu.RUnlock()
	return it.currentIter
}

// GetIterations returns all iterations
func (it *IterationTracker) GetIterations() []Iteration {
	it.mu.RLock()
	defer it.mu.RUnlock()

	result := make([]Iteration, len(it.iterations))
	copy(result, it.iterations)
	return result
}

// GetStateVisitCount returns how many times a state was visited
func (it *IterationTracker) GetStateVisitCount(state State) int {
	it.mu.RLock()
	defer it.mu.RUnlock()
	return it.stateVisits[state]
}

// GetTestRunCount returns the total number of test runs
func (it *IterationTracker) GetTestRunCount() int {
	it.mu.RLock()
	defer it.mu.RUnlock()
	return it.testRunCount
}

// GetTotalDuration returns total time spent on this task
func (it *IterationTracker) GetTotalDuration() time.Duration {
	it.mu.RLock()
	defer it.mu.RUnlock()
	return time.Since(it.startTime)
}

// GetAverageIterationDuration returns average duration per iteration
func (it *IterationTracker) GetAverageIterationDuration() time.Duration {
	it.mu.RLock()
	defer it.mu.RUnlock()

	if len(it.iterations) == 0 {
		return 0
	}

	var total time.Duration
	count := 0

	for _, iter := range it.iterations {
		if iter.EndTime != nil {
			total += iter.EndTime.Sub(iter.StartTime)
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return total / time.Duration(count)
}

// GetMetrics returns summary metrics
func (it *IterationTracker) GetMetrics() IterationMetrics {
	it.mu.RLock()
	defer it.mu.RUnlock()

	successCount := 0
	for _, iter := range it.iterations {
		if iter.Success {
			successCount++
		}
	}

	return IterationMetrics{
		TaskID:               it.taskID,
		TotalIterations:      it.currentIter,
		SuccessfulIterations: successCount,
		FailedIterations:     it.currentIter - successCount,
		StateVisits:          copyStateVisits(it.stateVisits),
		TestRunCount:         it.testRunCount,
		TotalDuration:        time.Since(it.startTime),
		AvgIterationDuration: it.getAvgDurationLocked(),
	}
}

// getAvgDurationLocked calculates average iteration duration (assumes lock held)
func (it *IterationTracker) getAvgDurationLocked() time.Duration {
	if len(it.iterations) == 0 {
		return 0
	}

	var total time.Duration
	count := 0

	for _, iter := range it.iterations {
		if iter.EndTime != nil {
			total += iter.EndTime.Sub(iter.StartTime)
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return total / time.Duration(count)
}

// copyStateVisits creates a copy of the state visits map
func copyStateVisits(m map[State]int) map[State]int {
	result := make(map[State]int, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// IterationMetrics holds summary metrics for a task's iterations
type IterationMetrics struct {
	TaskID               string
	TotalIterations      int
	SuccessfulIterations int
	FailedIterations     int
	StateVisits          map[State]int
	TestRunCount         int
	TotalDuration        time.Duration
	AvgIterationDuration time.Duration
}

// GetMostVisitedState returns the state visited most often
func (im *IterationMetrics) GetMostVisitedState() (State, int) {
	var maxState State
	maxCount := 0

	for state, count := range im.StateVisits {
		if count > maxCount {
			maxState = state
			maxCount = count
		}
	}

	return maxState, maxCount
}

// GetSuccessRate returns the success rate as a percentage
func (im *IterationMetrics) GetSuccessRate() float64 {
	if im.TotalIterations == 0 {
		return 0.0
	}
	return float64(im.SuccessfulIterations) / float64(im.TotalIterations) * 100.0
}
