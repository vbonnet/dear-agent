// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
)

// SessionHandler is the application hook a Server delegates each task to.
// Implementations run a task-shaped unit of work and may pause it to
// request supervisor input via SessionIO.AskInput.
//
// The handler runs in its own goroutine for the lifetime of a task. The
// ctx is detached from any individual HTTP request: the server keeps the
// handler alive while a task is parked in input-required.
type SessionHandler interface {
	Handle(ctx context.Context, prompt string, io SessionIO) error
}

// HandlerFunc adapts an ordinary function to SessionHandler.
type HandlerFunc func(ctx context.Context, prompt string, io SessionIO) error

// Handle satisfies SessionHandler.
func (f HandlerFunc) Handle(ctx context.Context, prompt string, io SessionIO) error {
	return f(ctx, prompt, io)
}

// SessionIO is the runtime interface a SessionHandler uses while a task
// is in flight. Methods write events on the underlying A2A event queue.
type SessionIO interface {
	// Emit publishes a progress message on the task (status remains
	// "working").
	Emit(ctx context.Context, text string) error

	// AskInput pauses the task with state=input-required, transmits the
	// prompt to the client, and blocks until the client sends a follow-up
	// message. The returned string is the text of that follow-up.
	//
	// This is the protocol-native replacement for AskUserQuestion: the
	// agent does not deadlock waiting on an in-process callback; the
	// supervisor sees a paused task and answers through the normal
	// message channel.
	AskInput(ctx context.Context, prompt string) (string, error)

	// TaskID returns the A2A task identifier for the in-flight task.
	TaskID() string

	// ContextID returns the A2A context identifier for the in-flight task.
	ContextID() string
}

// extractText concatenates the text-part contents of a Message.
// Non-text parts are ignored — the spike only handles text exchanges.
func extractText(m *a2a.Message) string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range m.Parts {
		if tp, ok := part.(a2a.TextPart); ok {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(tp.Text)
		}
	}
	return b.String()
}
