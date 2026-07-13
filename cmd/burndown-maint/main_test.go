package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestTickResultJSON(t *testing.T) {
	r := tickResult{
		Active:  0,
		Target:  1,
		Spawned: 0,
		DryRun:  true,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded tickResult
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Active != 0 {
		t.Errorf("active = %d, want 0", decoded.Active)
	}
	if decoded.Target != 1 {
		t.Errorf("target = %d, want 1", decoded.Target)
	}
	if !decoded.DryRun {
		t.Error("dry_run should be true")
	}
}

func TestRunCommandHonorsCancellationAndTimeout(t *testing.T) {
	t.Run("parent cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := runCommand(ctx, time.Second, os.Args[0], "-test.run=TestBurndownMaintHelperProcess")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runCommand error = %v, want context cancellation", err)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		t.Setenv("BURNDOWN_MAINT_HELPER_PROCESS", "1")
		_, err := runCommand(context.Background(), 10*time.Millisecond, os.Args[0], "-test.run=TestBurndownMaintHelperProcess")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runCommand error = %v, want deadline exceeded", err)
		}
	})
}

func TestBurndownMaintHelperProcess(t *testing.T) {
	if os.Getenv("BURNDOWN_MAINT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(time.Second)
}

func TestTickResultOmitsEmptySession(t *testing.T) {
	r := tickResult{Active: 1, Target: 1}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["spawned_session"]; ok {
		t.Error("spawned_session should be omitted when empty")
	}
	if _, ok := m["dry_run"]; ok {
		t.Error("dry_run should be omitted when false")
	}
}
