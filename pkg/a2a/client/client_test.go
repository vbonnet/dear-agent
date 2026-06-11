// Copyright 2026 dear-agent contributors. See LICENSE.

package client

import (
	"context"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
)

// TestSend_NilReceiver verifies that calling Send on a nil *Client returns
// a clean initialization error instead of panicking with a nil-pointer
// dereference.
func TestSend_NilReceiver(t *testing.T) {
	t.Parallel()

	var c *Client
	_, err := c.Send(context.Background(), "hi", nil)
	if err == nil {
		t.Fatal("Send on nil client: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Send on nil client: unexpected error %q", err)
	}
}

// TestSend_NilInner verifies the same guard when the receiver is non-nil
// but its inner transport is unset (e.g. a zero-value Client).
func TestSend_NilInner(t *testing.T) {
	t.Parallel()

	c := &Client{}
	_, err := c.Send(context.Background(), "hi", nil)
	if err == nil {
		t.Fatal("Send on zero-value client: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Send on zero-value client: unexpected error %q", err)
	}
}

// TestClose_ZeroValue documents that Close is safe on a nil/zero Client.
func TestClose_ZeroValue(t *testing.T) {
	t.Parallel()

	var c *Client
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
	if err := (&Client{}).Close(); err != nil {
		t.Fatalf("Close on zero-value client: %v", err)
	}
}

// TestExtractText covers the text concatenation helper, including the
// nil-message guard that protects the result type-switch in drive.
func TestExtractText(t *testing.T) {
	t.Parallel()

	if got := extractText(nil); got != "" {
		t.Fatalf("extractText(nil) = %q, want empty", got)
	}

	msg := a2a.NewMessage(a2a.MessageRoleAgent,
		a2a.TextPart{Text: "first"}, a2a.TextPart{Text: "second"})
	if got, want := extractText(msg), "first\nsecond"; got != want {
		t.Fatalf("extractText = %q, want %q", got, want)
	}
}

// TestFinalAgentText covers nil-task and nil-history-entry guards plus the
// status-message-preferred-over-history precedence.
func TestFinalAgentText(t *testing.T) {
	t.Parallel()

	if got := finalAgentText(nil); got != "" {
		t.Fatalf("finalAgentText(nil) = %q, want empty", got)
	}

	// Status message wins over history.
	statusMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "from-status"})
	taskWithStatus := &a2a.Task{
		Status:  a2a.TaskStatus{State: a2a.TaskStateCompleted, Message: statusMsg},
		History: []*a2a.Message{a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "from-history"})},
	}
	if got, want := finalAgentText(taskWithStatus), "from-status"; got != want {
		t.Fatalf("finalAgentText status-precedence = %q, want %q", got, want)
	}

	// Falls back to the last agent message in history, skipping a nil
	// entry without panicking.
	histMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "from-history"})
	taskWithHistory := &a2a.Task{
		Status:  a2a.TaskStatus{State: a2a.TaskStateCompleted},
		History: []*a2a.Message{nil, histMsg},
	}
	if got, want := finalAgentText(taskWithHistory), "from-history"; got != want {
		t.Fatalf("finalAgentText history-fallback = %q, want %q", got, want)
	}
}
