// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
)

// sessionExecutor implements a2asrv.AgentExecutor by routing each task to
// a long-lived goroutine running a SessionHandler. The goroutine
// outlives a single Execute call so the handler can pause for input.
type sessionExecutor struct {
	handler  SessionHandler
	sessions sync.Map // a2a.TaskID -> *runtime
}

// runtime holds the per-task state shared between Execute and the
// handler goroutine.
type runtime struct {
	taskID    a2a.TaskID
	contextID string

	// ctx governs the handler goroutine's lifetime; cancel signals
	// cancellation. Cancellation is delivered via ctx (never by closing
	// nextInput), so no goroutine can ever send on a closed channel.
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	queue eventqueue.Queue // updated on each Execute call

	// nextInput delivers a follow-up message to a parked handler. Buffered
	// (1) so the resuming Execute call never blocks on a fast handler.
	nextInput chan string
	// paused is signalled by the handler when it transitions to either
	// input-required or a terminal state; this releases the current
	// Execute call.
	paused chan struct{}
	// closed when the handler goroutine exits.
	done chan struct{}
}

func newRuntime(taskID a2a.TaskID, contextID string) *runtime {
	ctx, cancel := context.WithCancel(context.Background())
	return &runtime{
		taskID:    taskID,
		contextID: contextID,
		ctx:       ctx,
		cancel:    cancel,
		nextInput: make(chan string, 1),
		paused:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

func (r *runtime) setQueue(q eventqueue.Queue) {
	r.mu.Lock()
	r.queue = q
	r.mu.Unlock()
}

func (r *runtime) getQueue() eventqueue.Queue {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queue
}

func (r *runtime) signalPaused() {
	select {
	case r.paused <- struct{}{}:
	default:
		// Already signalled; the next Execute call will pick it up.
	}
}

// Execute satisfies a2asrv.AgentExecutor.
//
// First call:  start a handler goroutine and wait for it to either pause
//
//	(input-required) or finish (terminal state).
//
// Resume call: hand the new message text to the parked goroutine and
//
//	wait for the next pause/finish signal.
func (e *sessionExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	v, loaded := e.sessions.LoadOrStore(reqCtx.TaskID, newRuntime(reqCtx.TaskID, reqCtx.ContextID))
	run := v.(*runtime)
	run.setQueue(queue)

	if !loaded {
		// First call. Write the standard submitted -> working transition,
		// then spawn the handler goroutine.
		if err := queue.Write(ctx, a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateSubmitted, nil)); err != nil {
			e.sessions.Delete(reqCtx.TaskID)
			return fmt.Errorf("a2a: write submitted: %w", err)
		}
		if err := queue.Write(ctx, a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, nil)); err != nil {
			e.sessions.Delete(reqCtx.TaskID)
			return fmt.Errorf("a2a: write working: %w", err)
		}
		prompt := extractText(reqCtx.Message)
		// Run the handler under run.ctx, which is detached from any
		// individual HTTP request (the handler may sit in input-required
		// across multiple short-lived request contexts) and is the single
		// cancellation channel: Cancel calls run.cancel() rather than
		// closing any data channel, so no goroutine can send on a closed
		// channel.
		go e.runHandler(run.ctx, run, reqCtx, prompt)
	} else {
		// Resume. Emit a working transition so the client observes
		// progress, then deliver the follow-up to the parked goroutine.
		if err := queue.Write(ctx, a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, nil)); err != nil {
			return fmt.Errorf("a2a: write working (resume): %w", err)
		}
		// Drain any stale pause signal left over from the prior pause so
		// the final select below blocks on *this* resume's pause/finish
		// rather than returning immediately on the previous one.
		select {
		case <-run.paused:
		default:
		}
		answer := extractText(reqCtx.Message)
		select {
		case run.nextInput <- answer:
		case <-ctx.Done():
			return ctx.Err()
		case <-run.done:
			return errors.New("a2a: handler exited before resume input delivered")
		}
	}

	// Wait until the handler pauses or finishes.
	select {
	case <-run.paused:
		return nil
	case <-run.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Cancel satisfies a2asrv.AgentExecutor. The handler goroutine, if any,
// is signalled via context cancellation (run.cancel); the canceled status
// is written to the queue. Because Cancel removes the session and writes
// the single terminal Canceled event, runHandler must not also write a
// terminal event once it observes the cancellation (see runHandler).
func (e *sessionExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	if v, ok := e.sessions.LoadAndDelete(reqCtx.TaskID); ok {
		run := v.(*runtime)
		// Cancelling run.ctx unblocks any AskInput call waiting on
		// <-run.ctx.Done() and tells a ctx-respecting handler to return.
		// runHandler detects run.ctx.Err() != nil and skips its own
		// terminal write so the Canceled event below is authoritative.
		run.cancel()
	}
	evt := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCanceled, nil)
	evt.Final = true
	return queue.Write(ctx, evt)
}

// runHandler executes the user's SessionHandler.Handle and writes the
// terminal status event when it returns.
func (e *sessionExecutor) runHandler(ctx context.Context, run *runtime, reqCtx *a2asrv.RequestContext, prompt string) {
	defer close(run.done)
	defer e.sessions.Delete(run.taskID)

	// A panicking handler must not take down the whole server. Recover,
	// best-effort write a Failed terminal event with the panic message,
	// then release the in-flight Execute call via signalPaused so it does
	// not block forever waiting on a pause/finish signal.
	defer func() {
		if r := recover(); r != nil {
			if run.ctx.Err() == nil {
				final := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateFailed,
					a2a.NewMessageForTask(a2a.MessageRoleAgent, reqCtx,
						a2a.TextPart{Text: fmt.Sprintf("a2a: handler panic: %v", r)}))
				final.Final = true
				_ = run.getQueue().Write(ctx, final)
			}
			run.signalPaused()
		}
	}()

	io := &sessionIO{run: run, reqCtx: reqCtx}

	handleErr := e.handler.Handle(ctx, prompt, io)

	// If the task was cancelled, Cancel already removed the session and
	// wrote the authoritative Canceled terminal event. Writing another
	// terminal event here would conflict; just release Execute and return.
	if run.ctx.Err() != nil {
		run.signalPaused()
		return
	}

	q := run.getQueue()
	state := a2a.TaskStateCompleted
	var statusMsg *a2a.Message
	if handleErr != nil {
		state = a2a.TaskStateFailed
		statusMsg = a2a.NewMessageForTask(a2a.MessageRoleAgent, reqCtx, a2a.TextPart{Text: handleErr.Error()})
	}
	final := a2a.NewStatusUpdateEvent(reqCtx, state, statusMsg)
	final.Final = true
	_ = q.Write(ctx, final)

	run.signalPaused()
}

// sessionIO is the per-task SessionIO implementation handed to the
// SessionHandler. It writes events on whichever queue the executor
// currently holds for this task.
type sessionIO struct {
	run    *runtime
	reqCtx *a2asrv.RequestContext
}

func (io *sessionIO) Emit(ctx context.Context, text string) error {
	msg := a2a.NewMessageForTask(a2a.MessageRoleAgent, io.reqCtx, a2a.TextPart{Text: text})
	evt := a2a.NewStatusUpdateEvent(io.reqCtx, a2a.TaskStateWorking, msg)
	return io.run.getQueue().Write(ctx, evt)
}

func (io *sessionIO) AskInput(ctx context.Context, prompt string) (string, error) {
	promptMsg := a2a.NewMessageForTask(a2a.MessageRoleAgent, io.reqCtx, a2a.TextPart{Text: prompt})
	evt := a2a.NewStatusUpdateEvent(io.reqCtx, a2a.TaskStateInputRequired, promptMsg)
	evt.Final = true
	if err := io.run.getQueue().Write(ctx, evt); err != nil {
		return "", fmt.Errorf("a2a: write input-required: %w", err)
	}

	// Release the in-flight Execute call so the server can return the
	// paused task to the supervisor.
	io.run.signalPaused()

	select {
	case answer, ok := <-io.run.nextInput:
		// nextInput is never closed (cancellation is delivered via ctx,
		// not by closing the channel), so ok is always true here. The
		// guard is retained defensively.
		if !ok {
			return "", context.Canceled
		}
		return answer, nil
	case <-ctx.Done():
		// ctx is run.ctx: either the request was cancelled or Cancel
		// called run.cancel(). Either way, abandon the wait.
		return "", ctx.Err()
	}
}

func (io *sessionIO) TaskID() string    { return string(io.run.taskID) }
func (io *sessionIO) ContextID() string { return io.run.contextID }
