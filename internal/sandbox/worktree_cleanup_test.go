package sandbox

import (
	"errors"
	"testing"
)

func TestWorktreeCleanupRetriesOnlyIncompletePhase(t *testing.T) {
	removeErr := errors.New("worktree locked")
	directoryErr := errors.New("directory busy")
	state := NewWorktreeCleanup(true)
	removeCalls := 0
	cleanupCalls := 0

	err := state.Run(func() error {
		removeCalls++
		return removeErr
	}, func() error {
		cleanupCalls++
		return nil
	})
	if !errors.Is(err, removeErr) || removeCalls != 1 || cleanupCalls != 0 {
		t.Fatalf("locked phase = err:%v remove:%d cleanup:%d", err, removeCalls, cleanupCalls)
	}

	err = state.Run(func() error {
		removeCalls++
		return nil
	}, func() error {
		cleanupCalls++
		return directoryErr
	})
	if !errors.Is(err, directoryErr) || removeCalls != 2 || cleanupCalls != 1 {
		t.Fatalf("directory failure = err:%v remove:%d cleanup:%d", err, removeCalls, cleanupCalls)
	}

	err = state.Run(func() error {
		removeCalls++
		return errors.New("completed Git removal repeated")
	}, func() error {
		cleanupCalls++
		return nil
	})
	if err != nil || removeCalls != 2 || cleanupCalls != 2 {
		t.Fatalf("retry = err:%v remove:%d cleanup:%d", err, removeCalls, cleanupCalls)
	}

	if err := state.Run(nil, nil); err != nil {
		t.Fatalf("completed cleanup should be idempotent: %v", err)
	}
	if removeCalls != 2 || cleanupCalls != 2 {
		t.Fatalf("completed cleanup repeated a phase: remove:%d cleanup:%d", removeCalls, cleanupCalls)
	}
}

func TestWorktreeCleanupWithoutGitStartsAtDirectories(t *testing.T) {
	state := NewWorktreeCleanup(false)
	cleanupCalls := 0
	if err := state.Run(func() error {
		return errors.New("unexpected worktree removal")
	}, func() error {
		cleanupCalls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("directory cleanup calls = %d, want 1", cleanupCalls)
	}
}
