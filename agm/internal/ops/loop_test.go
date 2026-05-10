package ops

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// openTestLoopStore creates an in-memory loop store for tests.
func openTestLoopStore(t *testing.T) *LoopStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "loops.db")
	s, err := OpenLoopStore(path)
	if err != nil {
		t.Fatalf("OpenLoopStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestLoopStore_CreateAndGet(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	l, err := s.CreateLoop(ctx, "babysit-prs", "Check open PRs", "gh pr list", 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}
	if l.Name != "babysit-prs" {
		t.Errorf("Name = %q, want %q", l.Name, "babysit-prs")
	}
	if l.Cadence != 5*time.Minute {
		t.Errorf("Cadence = %v, want %v", l.Cadence, 5*time.Minute)
	}
	if l.Status != LoopStatusActive {
		t.Errorf("Status = %q, want %q", l.Status, LoopStatusActive)
	}
	if l.NextRunAt == nil {
		t.Fatal("NextRunAt is nil")
	}

	got, err := s.GetLoop(ctx, "babysit-prs")
	if err != nil {
		t.Fatalf("GetLoop: %v", err)
	}
	if got.Name != l.Name {
		t.Errorf("GetLoop Name = %q, want %q", got.Name, l.Name)
	}
	if got.Cmd != "gh pr list" {
		t.Errorf("GetLoop Cmd = %q, want %q", got.Cmd, "gh pr list")
	}
}

func TestLoopStore_CreateDuplicate(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "dup", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("first CreateLoop: %v", err)
	}
	if _, err := s.CreateLoop(ctx, "dup", "", "echo bye", time.Minute); err == nil {
		t.Fatal("second CreateLoop: expected error for duplicate name, got nil")
	}
}

func TestLoopStore_CreateValidation(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		loopN   string
		cmd     string
		cadence time.Duration
	}{
		{"empty name", "", "echo hi", time.Minute},
		{"empty cmd", "my-loop", "", time.Minute},
		{"zero cadence", "my-loop", "echo hi", 0},
		{"negative cadence", "my-loop", "echo hi", -time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.CreateLoop(ctx, tc.loopN, "", tc.cmd, tc.cadence); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestLoopStore_ListLoops(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	for _, name := range []string{"b-loop", "a-loop", "c-loop"} {
		if _, err := s.CreateLoop(ctx, name, "", "echo "+name, time.Minute); err != nil {
			t.Fatalf("CreateLoop %q: %v", name, err)
		}
	}

	loops, err := s.ListLoops(ctx)
	if err != nil {
		t.Fatalf("ListLoops: %v", err)
	}
	if len(loops) != 3 {
		t.Fatalf("len(loops) = %d, want 3", len(loops))
	}
	// Must be returned in alphabetical order.
	want := []string{"a-loop", "b-loop", "c-loop"}
	for i, l := range loops {
		if l.Name != want[i] {
			t.Errorf("loops[%d].Name = %q, want %q", i, l.Name, want[i])
		}
	}
}

func TestLoopStore_SetStatus(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "my-loop", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}

	if err := s.SetStatus(ctx, "my-loop", LoopStatusPaused); err != nil {
		t.Fatalf("SetStatus(paused): %v", err)
	}
	l, _ := s.GetLoop(ctx, "my-loop")
	if l.Status != LoopStatusPaused {
		t.Errorf("Status = %q, want paused", l.Status)
	}

	if err := s.SetStatus(ctx, "my-loop", LoopStatusActive); err != nil {
		t.Fatalf("SetStatus(active): %v", err)
	}
	l, _ = s.GetLoop(ctx, "my-loop")
	if l.Status != LoopStatusActive {
		t.Errorf("Status = %q, want active", l.Status)
	}

	// Non-existent loop.
	if err := s.SetStatus(ctx, "ghost", LoopStatusPaused); err == nil {
		t.Fatal("expected error for missing loop, got nil")
	}
}

func TestLoopStore_DeleteLoop(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "my-loop", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}
	if err := s.DeleteLoop(ctx, "my-loop"); err != nil {
		t.Fatalf("DeleteLoop: %v", err)
	}
	if _, err := s.GetLoop(ctx, "my-loop"); err == nil {
		t.Fatal("GetLoop after delete: expected error, got nil")
	}
	// Deleting non-existent.
	if err := s.DeleteLoop(ctx, "ghost"); err == nil {
		t.Fatal("DeleteLoop(ghost): expected error, got nil")
	}
}

func TestLoopStore_RunLoop_Success(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "echo-loop", "", "echo hello", time.Minute); err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}

	r, err := s.RunLoop(ctx, "echo-loop")
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if !r.Success {
		t.Errorf("Success = false, want true; stderr=%q", r.Stderr)
	}
	if r.ExitCode == nil || *r.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", r.ExitCode)
	}
	if r.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", r.Stdout, "hello\n")
	}

	// Loop metadata should be updated.
	l, _ := s.GetLoop(ctx, "echo-loop")
	if l.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", l.RunCount)
	}
	if l.LastRunAt == nil {
		t.Error("LastRunAt is nil after run")
	}
	if l.NextRunAt == nil {
		t.Error("NextRunAt is nil after run")
	}
}

func TestLoopStore_RunLoop_Failure(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "fail-loop", "", "exit 42", time.Minute); err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}

	r, err := s.RunLoop(ctx, "fail-loop")
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if r.Success {
		t.Error("Success = true, want false")
	}
	if r.ExitCode == nil || *r.ExitCode != 42 {
		t.Errorf("ExitCode = %v, want 42", r.ExitCode)
	}
}

func TestLoopStore_GetRuns(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "my-loop", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := s.RunLoop(ctx, "my-loop"); err != nil {
			t.Fatalf("RunLoop[%d]: %v", i, err)
		}
	}

	runs, err := s.GetRuns(ctx, "my-loop", 10)
	if err != nil {
		t.Fatalf("GetRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("len(runs) = %d, want 3", len(runs))
	}

	// Should be newest first.
	for i := 1; i < len(runs); i++ {
		if runs[i].StartedAt.After(runs[i-1].StartedAt) {
			t.Errorf("runs not newest-first at index %d", i)
		}
	}

	// Limit=1 returns only one.
	limited, err := s.GetRuns(ctx, "my-loop", 1)
	if err != nil {
		t.Fatalf("GetRuns(limit=1): %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("GetRuns(limit=1) = %d, want 1", len(limited))
	}
}

func TestLoopStore_DueLoops(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	now := time.Now()

	// Override the store's clock so we control next_run_at.
	// Active loop whose next_run is in the past: should be due.
	s.now = func() time.Time { return now.Add(-time.Hour) }
	if _, err := s.CreateLoop(ctx, "overdue", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("CreateLoop overdue: %v", err)
	}

	// Restore clock for "future" loop.
	s.now = func() time.Time { return now }
	if _, err := s.CreateLoop(ctx, "future", "", "echo hi", time.Hour); err != nil {
		t.Fatalf("CreateLoop future: %v", err)
	}

	// Paused loop: even if overdue, should not appear.
	s.now = func() time.Time { return now.Add(-time.Hour) }
	if _, err := s.CreateLoop(ctx, "paused-loop", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("CreateLoop paused: %v", err)
	}
	s.now = func() time.Time { return now }
	if err := s.SetStatus(ctx, "paused-loop", LoopStatusPaused); err != nil {
		t.Fatalf("SetStatus paused: %v", err)
	}

	due, err := s.DueLoops(ctx)
	if err != nil {
		t.Fatalf("DueLoops: %v", err)
	}
	if len(due) != 1 {
		t.Errorf("DueLoops = %d loop(s), want 1; got: %v", len(due), loopNames(due))
	}
	if len(due) > 0 && due[0].Name != "overdue" {
		t.Errorf("DueLoops[0].Name = %q, want %q", due[0].Name, "overdue")
	}
}

func TestLoopStore_RunLoop_NotFound(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.RunLoop(ctx, "ghost"); err == nil {
		t.Fatal("RunLoop(ghost): expected error, got nil")
	}
}

func TestLoopStore_GetRuns_Empty(t *testing.T) {
	s := openTestLoopStore(t)
	ctx := context.Background()

	if _, err := s.CreateLoop(ctx, "new-loop", "", "echo hi", time.Minute); err != nil {
		t.Fatalf("CreateLoop: %v", err)
	}
	runs, err := s.GetRuns(ctx, "new-loop", 10)
	if err != nil {
		t.Fatalf("GetRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("GetRuns = %d runs, want 0", len(runs))
	}
}

// loopNames is a debugging helper for test failure messages.
func loopNames(loops []*Loop) []string {
	names := make([]string, len(loops))
	for i, l := range loops {
		names[i] = l.Name
	}
	return names
}
