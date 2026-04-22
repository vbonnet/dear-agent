package tracking

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestTracker(t *testing.T) *FailureTracker {
	t.Helper()
	path := filepath.Join(t.TempDir(), "failure-tracking.json")
	ft, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker: %v", err)
	}
	return ft
}

func TestRecordAndGetFailures(t *testing.T) {
	ft := newTestTracker(t)

	if got := ft.GetFailures("task-1"); got != 0 {
		t.Errorf("expected 0 failures for unknown task, got %d", got)
	}

	for i := 1; i <= 3; i++ {
		if err := ft.RecordFailure("task-1"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
		if got := ft.GetFailures("task-1"); got != i {
			t.Errorf("after %d failures, GetFailures = %d", i, got)
		}
	}
}

func TestShouldSkip(t *testing.T) {
	ft := newTestTracker(t)

	if ft.ShouldSkip("task-1", 3) {
		t.Error("ShouldSkip should be false with 0 failures")
	}

	for i := 0; i < 2; i++ {
		if err := ft.RecordFailure("task-1"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if ft.ShouldSkip("task-1", 3) {
		t.Error("ShouldSkip should be false with 2 failures (max=3)")
	}

	if err := ft.RecordFailure("task-1"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if !ft.ShouldSkip("task-1", 3) {
		t.Error("ShouldSkip should be true with 3 failures (max=3)")
	}
}

func TestReset(t *testing.T) {
	ft := newTestTracker(t)

	for i := 0; i < 3; i++ {
		if err := ft.RecordFailure("task-1"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if err := ft.Reset("task-1"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got := ft.GetFailures("task-1"); got != 0 {
		t.Errorf("after reset, expected 0 failures, got %d", got)
	}
	if ft.ShouldSkip("task-1", 3) {
		t.Error("ShouldSkip should be false after reset")
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure-tracking.json")

	ft1, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := ft1.RecordFailure("task-persist"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	ft2, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker reload: %v", err)
	}
	if got := ft2.GetFailures("task-persist"); got != 2 {
		t.Errorf("after reload, expected 2 failures, got %d", got)
	}
}

func TestMultipleTasks(t *testing.T) {
	ft := newTestTracker(t)

	if err := ft.RecordFailure("a"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := ft.RecordFailure("b"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	if ft.ShouldSkip("a", 3) {
		t.Error("task 'a' should not be skipped (1 failure)")
	}
	if !ft.ShouldSkip("b", 3) {
		t.Error("task 'b' should be skipped (3 failures)")
	}
}

func TestNewFailureTrackerDefaultPath(t *testing.T) {
	// Verify constructor works with empty path (uses default).
	ft, err := NewFailureTracker("")
	if err != nil {
		t.Fatalf("NewFailureTracker with default path: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".agm", "failure-tracking.json")
	if ft.path != expected {
		t.Errorf("default path = %q, want %q", ft.path, expected)
	}
}

// --- Edge-case tests below ---

func TestShouldSkipMaxFailuresZero(t *testing.T) {
	ft := newTestTracker(t)

	// Unknown task always returns false (no record exists).
	if ft.ShouldSkip("new-task", 0) {
		t.Error("ShouldSkip(unknown, 0) should be false — no record")
	}

	// After recording one failure, count (1) >= max (0) is true.
	if err := ft.RecordFailure("new-task"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if !ft.ShouldSkip("new-task", 0) {
		t.Error("ShouldSkip(1 failure, max=0) should be true")
	}
}

func TestShouldSkipMaxFailuresOne(t *testing.T) {
	ft := newTestTracker(t)

	if ft.ShouldSkip("task", 1) {
		t.Error("ShouldSkip should be false with 0 failures and max=1")
	}

	if err := ft.RecordFailure("task"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if !ft.ShouldSkip("task", 1) {
		t.Error("ShouldSkip should be true with 1 failure and max=1")
	}
}

func TestShouldSkipCountExceedsMax(t *testing.T) {
	ft := newTestTracker(t)

	// Record 5 failures, check with max=3.
	for i := 0; i < 5; i++ {
		if err := ft.RecordFailure("task"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if !ft.ShouldSkip("task", 3) {
		t.Error("ShouldSkip should be true when count (5) exceeds max (3)")
	}
}

func TestShouldSkipNegativeMax(t *testing.T) {
	ft := newTestTracker(t)

	// Unknown task always returns false (no record exists), even with negative max.
	if ft.ShouldSkip("task", -1) {
		t.Error("ShouldSkip(unknown, -1) should be false — no record")
	}

	// After recording one failure, count (1) >= max (-1) is true.
	if err := ft.RecordFailure("task"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if !ft.ShouldSkip("task", -1) {
		t.Error("ShouldSkip(1 failure, max=-1) should be true")
	}
}

func TestShouldSkipUnknownTask(t *testing.T) {
	ft := newTestTracker(t)

	// Unknown task with positive max returns false (no record exists).
	if ft.ShouldSkip("nonexistent", 5) {
		t.Error("ShouldSkip for unknown task with positive max should be false")
	}
}

func TestRecordFailureEmptyTaskID(t *testing.T) {
	ft := newTestTracker(t)

	if err := ft.RecordFailure(""); err != nil {
		t.Fatalf("RecordFailure with empty ID: %v", err)
	}
	if got := ft.GetFailures(""); got != 1 {
		t.Errorf("GetFailures(\"\") = %d, want 1", got)
	}
}

func TestGetFailuresUnknownTask(t *testing.T) {
	ft := newTestTracker(t)

	if got := ft.GetFailures("does-not-exist"); got != 0 {
		t.Errorf("GetFailures for unknown task = %d, want 0", got)
	}
}

func TestResetNonexistentTask(t *testing.T) {
	ft := newTestTracker(t)

	// Resetting a task that was never recorded should not error.
	if err := ft.Reset("never-existed"); err != nil {
		t.Fatalf("Reset nonexistent task: %v", err)
	}
	if got := ft.GetFailures("never-existed"); got != 0 {
		t.Errorf("GetFailures after resetting nonexistent = %d, want 0", got)
	}
}

func TestDoubleReset(t *testing.T) {
	ft := newTestTracker(t)

	if err := ft.RecordFailure("task"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := ft.Reset("task"); err != nil {
		t.Fatalf("first Reset: %v", err)
	}
	if err := ft.Reset("task"); err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	if got := ft.GetFailures("task"); got != 0 {
		t.Errorf("after double reset, GetFailures = %d, want 0", got)
	}
}

func TestRecordFailureAfterReset(t *testing.T) {
	ft := newTestTracker(t)

	for i := 0; i < 3; i++ {
		if err := ft.RecordFailure("task"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if err := ft.Reset("task"); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// After reset, recording starts from 0 again.
	if err := ft.RecordFailure("task"); err != nil {
		t.Fatalf("RecordFailure after reset: %v", err)
	}
	if got := ft.GetFailures("task"); got != 1 {
		t.Errorf("after reset + 1 failure, GetFailures = %d, want 1", got)
	}
	if ft.ShouldSkip("task", 3) {
		t.Error("ShouldSkip should be false after reset + 1 failure (max=3)")
	}
}

func TestPersistenceAfterReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure-tracking.json")

	ft1, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := ft1.RecordFailure("task"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if err := ft1.Reset("task"); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Reload and verify reset persisted.
	ft2, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker reload: %v", err)
	}
	if got := ft2.GetFailures("task"); got != 0 {
		t.Errorf("after reload post-reset, GetFailures = %d, want 0", got)
	}
}

func TestPersistenceMultipleTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure-tracking.json")

	ft1, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker: %v", err)
	}
	if err := ft1.RecordFailure("alpha"); err != nil {
		t.Fatalf("RecordFailure alpha: %v", err)
	}
	for i := 0; i < 4; i++ {
		if err := ft1.RecordFailure("beta"); err != nil {
			t.Fatalf("RecordFailure beta: %v", err)
		}
	}

	// Reload and verify both tasks survived.
	ft2, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker reload: %v", err)
	}
	if got := ft2.GetFailures("alpha"); got != 1 {
		t.Errorf("alpha after reload = %d, want 1", got)
	}
	if got := ft2.GetFailures("beta"); got != 4 {
		t.Errorf("beta after reload = %d, want 4", got)
	}
}

func TestNewFailureTrackerCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure-tracking.json")

	if err := os.WriteFile(path, []byte("{invalid json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := NewFailureTracker(path)
	if err == nil {
		t.Fatal("expected error loading corrupt JSON, got nil")
	}
}

func TestNewFailureTrackerEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure-tracking.json")

	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	// Empty file triggers unmarshal error (unexpected end of JSON input).
	_, err := NewFailureTracker(path)
	if err == nil {
		t.Fatal("expected error loading empty file, got nil")
	}
}

func TestNewFailureTrackerCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "failure-tracking.json")

	ft, err := NewFailureTracker(path)
	if err != nil {
		t.Fatalf("NewFailureTracker with nested path: %v", err)
	}

	// Recording a failure should create the parent directories.
	if err := ft.RecordFailure("task"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected persistence file to be created in nested directory")
	}
}

func TestConcurrentRecordFailure(t *testing.T) {
	ft := newTestTracker(t)
	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := ft.RecordFailure("concurrent-task"); err != nil {
				t.Errorf("concurrent RecordFailure: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := ft.GetFailures("concurrent-task"); got != goroutines {
		t.Errorf("after %d concurrent failures, GetFailures = %d", goroutines, got)
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	ft := newTestTracker(t)
	const iterations = 10

	var wg sync.WaitGroup
	// Record failures on two tasks concurrently, interleaved with reads.
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := ft.RecordFailure("task-x"); err != nil {
				t.Errorf("RecordFailure task-x: %v", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := ft.RecordFailure("task-y"); err != nil {
				t.Errorf("RecordFailure task-y: %v", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			// Just exercise concurrent reads — no correctness assertion
			// since the writers are still running.
			ft.GetFailures("task-x")
			ft.ShouldSkip("task-y", 5)
		}
	}()

	wg.Wait()

	if got := ft.GetFailures("task-x"); got != iterations {
		t.Errorf("task-x: got %d failures, want %d", got, iterations)
	}
	if got := ft.GetFailures("task-y"); got != iterations {
		t.Errorf("task-y: got %d failures, want %d", got, iterations)
	}
}

func TestShouldSkipVaryingThresholds(t *testing.T) {
	ft := newTestTracker(t)

	for i := 0; i < 5; i++ {
		if err := ft.RecordFailure("task"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	tests := []struct {
		max  int
		want bool
	}{
		{max: 0, want: true},
		{max: 1, want: true},
		{max: 4, want: true},
		{max: 5, want: true},  // count == max
		{max: 6, want: false}, // count < max
		{max: 100, want: false},
	}
	for _, tc := range tests {
		got := ft.ShouldSkip("task", tc.max)
		if got != tc.want {
			t.Errorf("ShouldSkip(5 failures, max=%d) = %v, want %v", tc.max, got, tc.want)
		}
	}
}

func TestResetDoesNotAffectOtherTasks(t *testing.T) {
	ft := newTestTracker(t)

	if err := ft.RecordFailure("keep"); err != nil {
		t.Fatalf("RecordFailure keep: %v", err)
	}
	if err := ft.RecordFailure("remove"); err != nil {
		t.Fatalf("RecordFailure remove: %v", err)
	}
	if err := ft.Reset("remove"); err != nil {
		t.Fatalf("Reset remove: %v", err)
	}

	if got := ft.GetFailures("keep"); got != 1 {
		t.Errorf("reset of 'remove' affected 'keep': got %d, want 1", got)
	}
	if got := ft.GetFailures("remove"); got != 0 {
		t.Errorf("'remove' after reset = %d, want 0", got)
	}
}

func TestRecordFailureSetsLastFail(t *testing.T) {
	ft := newTestTracker(t)

	if err := ft.RecordFailure("task"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	ft.mu.Lock()
	rec, ok := ft.failures["task"]
	ft.mu.Unlock()

	if !ok {
		t.Fatal("expected failure record to exist")
	}
	if rec.LastFail.IsZero() {
		t.Error("LastFail should be set after RecordFailure")
	}
}

func TestLargeFailureCount(t *testing.T) {
	ft := newTestTracker(t)
	const count = 1000

	for i := 0; i < count; i++ {
		if err := ft.RecordFailure("heavy"); err != nil {
			t.Fatalf("RecordFailure at %d: %v", i, err)
		}
	}

	if got := ft.GetFailures("heavy"); got != count {
		t.Errorf("GetFailures = %d, want %d", got, count)
	}
	if !ft.ShouldSkip("heavy", 1) {
		t.Error("ShouldSkip should be true for 1000 failures with max=1")
	}
}
