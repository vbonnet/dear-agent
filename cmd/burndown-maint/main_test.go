package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func TestRunAGMCommandHonorsCancellationAndTimeout(t *testing.T) {
	installTestAGM(t)

	t.Run("parent cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := runAGMCommand(ctx, time.Second, "-test.run=TestBurndownMaintHelperProcess")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runAGMCommand error = %v, want context cancellation", err)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		t.Setenv("BURNDOWN_MAINT_HELPER_PROCESS", "1")
		_, err := runAGMCommand(context.Background(), 10*time.Millisecond, "-test.run=TestBurndownMaintHelperProcess")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runAGMCommand error = %v, want deadline exceeded", err)
		}
	})
}

func installTestAGM(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "agm"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := t.TempDir()
	target := filepath.Join(dir, name)
	if err := os.Link(executable, target); err != nil {
		source, openErr := os.Open(executable)
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer source.Close()
		destination, createErr := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, copyErr := io.Copy(destination, source); copyErr != nil {
			_ = destination.Close()
			t.Fatal(copyErr)
		}
		if closeErr := destination.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
