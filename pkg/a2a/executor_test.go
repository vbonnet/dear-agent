// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

// recordingQueue is a thread-safe eventqueue.Queue stand-in that records
// every event written to it. It lets the executor tests assert on the
// terminal state without standing up a full HTTP server.
type recordingQueue struct {
	mu     sync.Mutex
	events []a2a.Event
}

func (q *recordingQueue) Write(_ context.Context, event a2a.Event) error {
	q.mu.Lock()
	q.events = append(q.events, event)
	q.mu.Unlock()
	return nil
}

func (q *recordingQueue) WriteVersioned(ctx context.Context, event a2a.Event, _ a2a.TaskVersion) error {
	return q.Write(ctx, event)
}

func (q *recordingQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, context.Canceled
}

func (q *recordingQueue) Close() error { return nil }

// finalStates returns the states of all events flagged Final.
func (q *recordingQueue) finalStates() []a2a.TaskState {
	q.mu.Lock()
	defer q.mu.Unlock()
	var states []a2a.TaskState
	for _, e := range q.events {
		if su, ok := e.(*a2a.TaskStatusUpdateEvent); ok && su.Final {
			states = append(states, su.Status.State)
		}
	}
	return states
}

func newReqCtx(taskID a2a.TaskID, text string) *a2asrv.RequestContext {
	return &a2asrv.RequestContext{
		TaskID:    taskID,
		ContextID: "ctx-" + string(taskID),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: text}),
	}
}

// waitFor polls cond until it is true or the deadline elapses.
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

// TestCancel_WhilePaused exercises the headline fix: a handler parked in
// AskInput is cancelled. Pre-fix this closed nextInput and the handler's
// terminal write raced with Cancel's Canceled write (conflicting terminal
// states) and a second Cancel could send-on-closed-channel panic. Post-fix
// Cancel cancels run.ctx, the handler observes run.ctx.Err() != nil and
// skips its own terminal write, leaving exactly one Canceled event.
func TestCancel_WhilePaused(t *testing.T) {
	t.Parallel()

	paused := make(chan struct{})
	exec := &sessionExecutor{handler: HandlerFunc(func(ctx context.Context, _ string, io SessionIO) error {
		close(paused)
		// Blocks until cancellation arrives via ctx (run.ctx).
		_, err := io.AskInput(ctx, "are we go?")
		return err
	})}

	const taskID a2a.TaskID = "task-paused"
	reqCtx := newReqCtx(taskID, "start")
	q := &recordingQueue{}

	// Execute returns once the handler pauses in input-required.
	go func() { _ = exec.Execute(context.Background(), reqCtx, q) }()

	<-paused
	// Ensure the handler is actually parked on nextInput before cancelling.
	waitFor(t, 2*time.Second, func() bool {
		return slices.Contains(q.finalStates(), a2a.TaskStateInputRequired)
	})

	cancelReq := &a2asrv.RequestContext{TaskID: taskID, ContextID: reqCtx.ContextID}
	if err := exec.Cancel(context.Background(), cancelReq, q); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// The handler goroutine must observe cancellation and exit without
	// writing a second terminal event.
	v, _ := exec.sessions.Load(taskID)
	if v != nil {
		t.Fatalf("session should be deleted after Cancel, got %v", v)
	}

	// Exactly one Canceled terminal event; no Completed/Failed.
	waitFor(t, 2*time.Second, func() bool {
		canceled := 0
		for _, st := range q.finalStates() {
			switch st {
			case a2a.TaskStateCanceled:
				canceled++
			case a2a.TaskStateCompleted, a2a.TaskStateFailed:
				t.Errorf("unexpected terminal state %s after Cancel", st)
			}
		}
		return canceled == 1
	})
}

// TestCancel_ConcurrentDuringResume fires many Execute resume calls and a
// Cancel concurrently against the same task. Pre-fix, Cancel closed
// nextInput while a resuming Execute call was sending on it — a classic
// send-on-closed-channel panic, and a double Cancel double-closed. Post-fix
// there is no channel close at all, so -race must stay clean and no panic
// can occur. The assertion is simply "no panic / no race", enforced by the
// test binary.
func TestCancel_ConcurrentDuringResume(t *testing.T) {
	t.Parallel()

	for range 50 {
		release := make(chan struct{})
		exec := &sessionExecutor{handler: HandlerFunc(func(ctx context.Context, _ string, io SessionIO) error {
			// One pause, then complete. The resume answer (if it arrives
			// before cancel) is ignored.
			if _, err := io.AskInput(ctx, "q?"); err != nil {
				return err
			}
			<-release
			return nil
		})}

		taskID := a2a.TaskID("task-resume")
		reqCtx := newReqCtx(taskID, "start")
		q := &recordingQueue{}

		// First call: parks the handler.
		_ = exec.Execute(context.Background(), reqCtx, q)

		var wg sync.WaitGroup
		// Concurrent resume.
		wg.Go(func() {
			resumeReq := &a2asrv.RequestContext{
				TaskID:    taskID,
				ContextID: reqCtx.ContextID,
				Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "answer"}),
			}
			_ = exec.Execute(context.Background(), resumeReq, q)
		})
		// Concurrent cancel.
		wg.Go(func() {
			cancelReq := &a2asrv.RequestContext{TaskID: taskID, ContextID: reqCtx.ContextID}
			_ = exec.Cancel(context.Background(), cancelReq, q)
		})
		// Second concurrent cancel — pre-fix this could double-close.
		wg.Go(func() {
			cancelReq := &a2asrv.RequestContext{TaskID: taskID, ContextID: reqCtx.ContextID}
			_ = exec.Cancel(context.Background(), cancelReq, q)
		})

		close(release)
		wg.Wait()
	}
}
