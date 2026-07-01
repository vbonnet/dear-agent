package supervisor

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAGMQueue_Enqueue(t *testing.T) {
	q := &AGMQueue{}
	if err := q.Enqueue(Task{ID: "t1", Title: "task 1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("duplicate ID rejected", func(t *testing.T) {
		if err := q.Enqueue(Task{ID: "t1", Title: "dup"}); err == nil {
			t.Fatal("expected error on duplicate ID, got nil")
		}
	})

	t.Run("empty ID rejected", func(t *testing.T) {
		if err := q.Enqueue(Task{Title: "no-id"}); err == nil {
			t.Fatal("expected error on empty ID, got nil")
		}
	})

	t.Run("sets EnqueuedAt if zero", func(t *testing.T) {
		before := time.Now()
		if err := q.Enqueue(Task{ID: "t2"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tasks, _ := q.Pending(context.Background())
		for _, t2 := range tasks {
			if t2.ID == "t2" && t2.EnqueuedAt.Before(before) {
				t.Errorf("EnqueuedAt not set: %v", t2.EnqueuedAt)
			}
		}
	})
}

func TestAGMQueue_Pending(t *testing.T) {
	q := &AGMQueue{}
	_ = q.Enqueue(Task{ID: "a"})
	_ = q.Enqueue(Task{ID: "b"})

	tasks, err := q.Pending(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("want 2 tasks, got %d", len(tasks))
	}
}

func TestAGMQueue_Dispatch(t *testing.T) {
	t.Run("calls agm session new and removes from pending after success", func(t *testing.T) {
		var capturedArgs []string
		q := &AGMQueue{
			run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				capturedArgs = args
				return []byte("ok"), nil
			},
		}
		_ = q.Enqueue(Task{ID: "ce-abc", Title: "task"})

		if err := q.Dispatch(context.Background(), "ce-abc", "coder"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify session name includes bead id.
		joined := strings.Join(capturedArgs, " ")
		if !strings.Contains(joined, "worker-ce-abc") {
			t.Errorf("session name not in args: %v", capturedArgs)
		}
		if !containsArg(capturedArgs, "--detached") {
			t.Errorf("detached flag not in args: %v", capturedArgs)
		}
		if containsArg(capturedArgs, "--detach") {
			t.Errorf("obsolete detach flag in args: %v", capturedArgs)
		}
		// Task should be removed from pending.
		tasks, _ := q.Pending(context.Background())
		if len(tasks) != 0 {
			t.Errorf("task not removed from pending after dispatch: %v", tasks)
		}
	})

	t.Run("unknown task returns error", func(t *testing.T) {
		q := &AGMQueue{}
		err := q.Dispatch(context.Background(), "no-such", "coder")
		if err == nil {
			t.Fatal("expected error for unknown task, got nil")
		}
	})

	t.Run("agm failure propagated and task remains pending", func(t *testing.T) {
		q := &AGMQueue{
			run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return []byte("capacity exhausted"), errors.New("agm: session limit reached")
			},
		}
		_ = q.Enqueue(Task{ID: "t1"})
		err := q.Dispatch(context.Background(), "t1", "coder")
		if err == nil {
			t.Fatal("expected error from agm, got nil")
		}
		if !strings.Contains(err.Error(), "worker-t1") {
			t.Fatalf("error should include worker session name, got: %v", err)
		}
		if !strings.Contains(err.Error(), "capacity exhausted") {
			t.Fatalf("error should include agm output, got: %v", err)
		}
		tasks, pendingErr := q.Pending(context.Background())
		if pendingErr != nil {
			t.Fatalf("pending failed: %v", pendingErr)
		}
		if len(tasks) != 1 || tasks[0].ID != "t1" {
			t.Fatalf("failed dispatch must keep task pending, got: %v", tasks)
		}
	})

	t.Run("uses defaults for model and role", func(t *testing.T) {
		var capturedArgs []string
		q := &AGMQueue{
			run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				capturedArgs = args
				return []byte("ok"), nil
			},
		}
		_ = q.Enqueue(Task{ID: "t-def"})
		_ = q.Dispatch(context.Background(), "t-def", "coder")

		joined := strings.Join(capturedArgs, " ")
		if !strings.Contains(joined, defaultDispatchModel) {
			t.Errorf("default model not in args: %v", capturedArgs)
		}
		if !strings.Contains(joined, defaultWorkerRole) {
			t.Errorf("default role not in args: %v", capturedArgs)
		}
	})

	t.Run("rejects concurrent dispatch of same pending task", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		var runCalls atomic.Int32
		q := &AGMQueue{
			run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				runCalls.Add(1)
				close(started)
				<-release
				return []byte("ok"), nil
			},
		}
		_ = q.Enqueue(Task{ID: "t-race"})

		errCh := make(chan error, 1)
		go func() {
			errCh <- q.Dispatch(context.Background(), "t-race", "coder")
		}()
		<-started

		err := q.Dispatch(context.Background(), "t-race", "coder")
		if err == nil {
			t.Fatal("expected concurrent dispatch error, got nil")
		}
		if !strings.Contains(err.Error(), "already dispatching") {
			t.Fatalf("expected already-dispatching error, got: %v", err)
		}

		close(release)
		if err := <-errCh; err != nil {
			t.Fatalf("first dispatch failed: %v", err)
		}
		if got := runCalls.Load(); got != 1 {
			t.Fatalf("run calls = %d, want 1", got)
		}
		tasks, pendingErr := q.Pending(context.Background())
		if pendingErr != nil {
			t.Fatalf("pending failed: %v", pendingErr)
		}
		if len(tasks) != 0 {
			t.Fatalf("successful first dispatch should remove pending task, got: %v", tasks)
		}
	})
}

func containsArg(args []string, want string) bool {
	return slices.Contains(args, want)
}
