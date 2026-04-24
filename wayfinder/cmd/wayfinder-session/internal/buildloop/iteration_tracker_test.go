package buildloop

import (
	"fmt"
	"testing"
	"time"
)

func TestNewIterationTracker(t *testing.T) {
	taskID := "T1"
	tracker := NewIterationTracker(taskID)

	if tracker == nil {
		t.Fatal("NewIterationTracker() returned nil")
	}
	if tracker.taskID != taskID {
		t.Errorf("taskID = %q, want %q", tracker.taskID, taskID)
	}
	if tracker.currentIter != 0 {
		t.Errorf("currentIter = %d, want 0", tracker.currentIter)
	}
	if tracker.testRunCount != 0 {
		t.Errorf("testRunCount = %d, want 0", tracker.testRunCount)
	}
}

func TestStartIteration(t *testing.T) {
	tracker := NewIterationTracker("T1")

	tracker.StartIteration()

	if tracker.GetCurrentIteration() != 1 {
		t.Errorf("currentIter = %d, want 1", tracker.GetCurrentIteration())
	}

	iterations := tracker.GetIterations()
	if len(iterations) != 1 {
		t.Fatalf("iterations length = %d, want 1", len(iterations))
	}

	iter := iterations[0]
	if iter.Number != 1 {
		t.Errorf("iteration Number = %d, want 1", iter.Number)
	}
	if iter.Success {
		t.Errorf("iteration Success = true, want false")
	}
	if len(iter.StatesVisited) != 0 {
		t.Errorf("StatesVisited length = %d, want 0", len(iter.StatesVisited))
	}
}

func TestRecordState(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()

	states := []State{StateTestFirst, StateCoding, StateGreen}
	for _, state := range states {
		tracker.RecordState(state)
	}

	iterations := tracker.GetIterations()
	if len(iterations) != 1 {
		t.Fatalf("iterations length = %d, want 1", len(iterations))
	}

	visited := iterations[0].StatesVisited
	if len(visited) != len(states) {
		t.Fatalf("StatesVisited length = %d, want %d", len(visited), len(states))
	}

	for i, state := range states {
		if visited[i] != state {
			t.Errorf("StatesVisited[%d] = %v, want %v", i, visited[i], state)
		}
	}
}

func TestGetStateVisitCount(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()

	// Record same state multiple times
	tracker.RecordState(StateTestFirst)
	tracker.RecordState(StateCoding)
	tracker.RecordState(StateTestFirst)
	tracker.RecordState(StateTestFirst)

	if count := tracker.GetStateVisitCount(StateTestFirst); count != 3 {
		t.Errorf("StateTestFirst visit count = %d, want 3", count)
	}
	if count := tracker.GetStateVisitCount(StateCoding); count != 1 {
		t.Errorf("StateCoding visit count = %d, want 1", count)
	}
	if count := tracker.GetStateVisitCount(StateGreen); count != 0 {
		t.Errorf("StateGreen visit count = %d, want 0", count)
	}
}

func TestGetTestRunCount(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()

	// These states count as test runs
	tracker.RecordState(StateTestFirst)
	tracker.RecordState(StateCoding)
	tracker.RecordState(StateGreen)
	tracker.RecordState(StateValidation) // Not a test run
	tracker.RecordState(StateCoding)

	// Expected: 4 test runs (TEST_FIRST, CODING, GREEN, CODING)
	if count := tracker.GetTestRunCount(); count != 4 {
		t.Errorf("test run count = %d, want 4", count)
	}
}

func TestCompleteIteration(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()

	time.Sleep(10 * time.Millisecond) // Ensure some duration

	err := fmt.Errorf("test error")
	tracker.CompleteIteration(true, err)

	iterations := tracker.GetIterations()
	if len(iterations) != 1 {
		t.Fatalf("iterations length = %d, want 1", len(iterations))
	}

	iter := iterations[0]
	if !iter.Success {
		t.Errorf("iteration Success = false, want true")
	}
	if iter.EndTime == nil {
		t.Errorf("EndTime is nil, want set")
	}
	if iter.Error != "test error" {
		t.Errorf("Error = %q, want %q", iter.Error, "test error")
	}
}

func TestMultipleIterations(t *testing.T) {
	tracker := NewIterationTracker("T1")

	// Start and complete 3 iterations
	for i := 1; i <= 3; i++ {
		tracker.StartIteration()
		tracker.RecordState(StateTestFirst)
		tracker.RecordState(StateCoding)
		tracker.CompleteIteration(i > 1, nil) // First fails, rest succeed
	}

	if current := tracker.GetCurrentIteration(); current != 3 {
		t.Errorf("currentIter = %d, want 3", current)
	}

	iterations := tracker.GetIterations()
	if len(iterations) != 3 {
		t.Fatalf("iterations length = %d, want 3", len(iterations))
	}

	// Check success pattern
	if iterations[0].Success {
		t.Errorf("iteration 1 Success = true, want false")
	}
	if !iterations[1].Success {
		t.Errorf("iteration 2 Success = false, want true")
	}
	if !iterations[2].Success {
		t.Errorf("iteration 3 Success = false, want true")
	}
}

func TestGetTotalDuration(t *testing.T) {
	tracker := NewIterationTracker("T1")

	time.Sleep(50 * time.Millisecond)

	duration := tracker.GetTotalDuration()
	if duration < 50*time.Millisecond {
		t.Errorf("duration = %v, want >= 50ms", duration)
	}
}

func TestGetAverageIterationDuration(t *testing.T) {
	tracker := NewIterationTracker("T1")

	// Create iterations with known durations
	for i := 0; i < 3; i++ {
		tracker.StartIteration()
		time.Sleep(10 * time.Millisecond)
		tracker.CompleteIteration(true, nil)
	}

	avg := tracker.GetAverageIterationDuration()
	if avg < 10*time.Millisecond {
		t.Errorf("average duration = %v, want >= 10ms", avg)
	}
}

func TestGetMetrics(t *testing.T) {
	tracker := NewIterationTracker("T1")

	// Create 3 iterations: 2 success, 1 failure
	for i := 0; i < 3; i++ {
		tracker.StartIteration()
		tracker.RecordState(StateTestFirst)
		tracker.RecordState(StateCoding)
		tracker.CompleteIteration(i > 0, nil)
	}

	metrics := tracker.GetMetrics()

	if metrics.TaskID != "T1" {
		t.Errorf("TaskID = %q, want %q", metrics.TaskID, "T1")
	}
	if metrics.TotalIterations != 3 {
		t.Errorf("TotalIterations = %d, want 3", metrics.TotalIterations)
	}
	if metrics.SuccessfulIterations != 2 {
		t.Errorf("SuccessfulIterations = %d, want 2", metrics.SuccessfulIterations)
	}
	if metrics.FailedIterations != 1 {
		t.Errorf("FailedIterations = %d, want 1", metrics.FailedIterations)
	}
	if metrics.TestRunCount != 6 { // 3 iterations * 2 test states each
		t.Errorf("TestRunCount = %d, want 6", metrics.TestRunCount)
	}
	if metrics.StateVisits[StateTestFirst] != 3 {
		t.Errorf("StateTestFirst visits = %d, want 3", metrics.StateVisits[StateTestFirst])
	}
}

func TestGetMostVisitedState(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()

	// Visit states with different frequencies
	tracker.RecordState(StateTestFirst)
	tracker.RecordState(StateCoding)
	tracker.RecordState(StateTestFirst)
	tracker.RecordState(StateGreen)
	tracker.RecordState(StateTestFirst)

	metrics := tracker.GetMetrics()
	state, count := metrics.GetMostVisitedState()

	if state != StateTestFirst {
		t.Errorf("most visited state = %v, want %v", state, StateTestFirst)
	}
	if count != 3 {
		t.Errorf("most visited count = %d, want 3", count)
	}
}

func TestGetSuccessRate(t *testing.T) {
	tests := []struct {
		name            string
		totalIters      int
		successfulIters int
		expectedRate    float64
	}{
		{"all success", 10, 10, 100.0},
		{"half success", 10, 5, 50.0},
		{"no success", 10, 0, 0.0},
		{"partial success", 10, 7, 70.0},
		{"no iterations", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := IterationMetrics{
				TotalIterations:      tt.totalIters,
				SuccessfulIterations: tt.successfulIters,
			}

			rate := metrics.GetSuccessRate()
			if rate != tt.expectedRate {
				t.Errorf("success rate = %f, want %f", rate, tt.expectedRate)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()

	// Test concurrent access to tracker methods
	done := make(chan bool, 2)

	// Goroutine 1: Record states
	go func() {
		for i := 0; i < 100; i++ {
			tracker.RecordState(StateTestFirst)
		}
		done <- true
	}()

	// Goroutine 2: Read metrics
	go func() {
		for i := 0; i < 100; i++ {
			_ = tracker.GetMetrics()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify state was recorded correctly
	if count := tracker.GetStateVisitCount(StateTestFirst); count != 100 {
		t.Errorf("StateTestFirst visit count = %d, want 100", count)
	}
}

func TestIterationMetricsCopy(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()
	tracker.RecordState(StateTestFirst)

	metrics1 := tracker.GetMetrics()
	metrics2 := tracker.GetMetrics()

	// Modify metrics1's StateVisits map
	metrics1.StateVisits[StateTestFirst] = 999

	// metrics2 should not be affected (independent copy)
	if metrics2.StateVisits[StateTestFirst] == 999 {
		t.Errorf("StateVisits map was not copied, shared reference detected")
	}
}

func TestRecordStateWithoutIteration(t *testing.T) {
	tracker := NewIterationTracker("T1")

	// Recording state without starting iteration should not panic
	tracker.RecordState(StateTestFirst)

	// Should have no effect
	if tracker.GetStateVisitCount(StateTestFirst) != 0 {
		t.Errorf("StateTestFirst count = %d, want 0 (no iteration started)",
			tracker.GetStateVisitCount(StateTestFirst))
	}
}

func TestCompleteIterationWithoutIteration(t *testing.T) {
	tracker := NewIterationTracker("T1")

	// Completing without starting should not panic
	tracker.CompleteIteration(true, nil)

	// Should have no effect
	iterations := tracker.GetIterations()
	if len(iterations) != 0 {
		t.Errorf("iterations length = %d, want 0", len(iterations))
	}
}

func TestAverageIterationDurationWithNoCompletedIterations(t *testing.T) {
	tracker := NewIterationTracker("T1")
	tracker.StartIteration()
	// Don't complete the iteration

	avg := tracker.GetAverageIterationDuration()
	if avg != 0 {
		t.Errorf("average duration = %v, want 0 (no completed iterations)", avg)
	}
}
