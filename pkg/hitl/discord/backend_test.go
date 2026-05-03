package discord

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

type fakeSender struct {
	mu       sync.Mutex
	calls    int
	channel  string
	messages []string
}

func (f *fakeSender) SendMessage(_ context.Context, ch, content string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.channel = ch
	f.messages = append(f.messages, content)
	return "msg-" + content[:1], nil // synthetic id, deterministic across calls
}

func TestParseDecision(t *testing.T) {
	cases := []struct {
		in    string
		want  workflow.HITLDecision
		valid bool
	}{
		{"approve", workflow.HITLDecisionApprove, true},
		{"approve please", workflow.HITLDecisionApprove, true},
		{"LGTM!", workflow.HITLDecisionApprove, true},
		{"y", workflow.HITLDecisionApprove, true},
		{"reject", workflow.HITLDecisionReject, true},
		{"no thanks", workflow.HITLDecisionReject, true},
		{"deny", workflow.HITLDecisionReject, true},
		{"   maybe", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := parseDecision(tc.in)
		if ok != tc.valid || (ok && got != tc.want) {
			t.Errorf("parseDecision(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.valid)
		}
	}
}

func TestRenderRequest_IncludesKeyFields(t *testing.T) {
	r := workflow.HITLRequest{
		WorkflowName: "wf",
		NodeID:       "n1",
		ApproverRole: "reviewer",
		Reason:       "needs human eyes",
		Confidence:   0.42,
		NodeOutput:   "draft text",
	}
	out := renderRequest(r)
	for _, want := range []string{"wf/n1", "reviewer", "needs human eyes", "0.42", "draft text", "approve", "reject"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered request missing %q\nGOT %q", want, out)
		}
	}
}

func TestBackend_RequestThenReply_Approve(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	ctx := context.Background()
	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID: "r1", WorkflowName: "wf", State: workflow.RunStateRunning, InputsJSON: "{}", StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := ss.UpsertNode(ctx, workflow.NodeRecord{RunID: "r1", NodeID: "n1", State: workflow.NodeStateRunning}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	approvalID, err := workflow.CreateHITLRequest(ctx, ss.DB(), "r1", "n1", "reviewer", "lgtm?", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}

	sender := &fakeSender{}
	b := NewBackend(sender, "C123", ss.DB())
	if err := b.Request(ctx, workflow.HITLRequest{ApprovalID: approvalID, RunID: "r1", NodeID: "n1", WorkflowName: "wf", ApproverRole: "reviewer"}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if sender.calls != 1 {
		t.Errorf("send calls=%d", sender.calls)
	}

	// Drive the reply in a goroutine; main test goroutine blocks on Wait.
	go func() {
		// The fake sender returns "msg-*", so we know the parent id.
		// Pull it from the byMessage map.
		var parent string
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			b.mu.Lock()
			for k := range b.byMessage {
				parent = k
			}
			b.mu.Unlock()
			if parent != "" {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		_ = b.OnReply(ctx, ReplyEvent{
			ParentMessageID: parent,
			AuthorID:        "u",
			AuthorName:      "alice",
			Role:            "reviewer",
			Content:         "approve",
			OccurredAt:      time.Now(),
		})
	}()

	res, err := b.Wait(ctx, approvalID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Decision != workflow.HITLDecisionApprove {
		t.Errorf("decision=%q, want approve", res.Decision)
	}
	if res.Approver != "alice" {
		t.Errorf("approver=%q, want alice", res.Approver)
	}
}

func TestBackend_OnReply_RoleNotAllowed(t *testing.T) {
	sender := &fakeSender{}
	b := NewBackend(sender, "C", nil)
	b.AllowedRoles = []string{"reviewer"}
	if _, err := b.Sender.SendMessage(context.Background(), "C", "ignored"); err != nil {
		t.Fatal(err)
	}
	// Manually wire the correlation map to simulate an in-flight request.
	b.mu.Lock()
	b.byMessage["msg-i"] = "approval-1"
	b.pending["approval-1"] = make(chan workflow.HITLResolution, 1)
	b.mu.Unlock()

	err := b.OnReply(context.Background(), ReplyEvent{
		ParentMessageID: "msg-i",
		AuthorName:      "eve",
		Role:            "intern",
		Content:         "approve",
	})
	if err == nil {
		t.Fatal("expected error for non-allowed role, got nil")
	}
}

func TestBackend_OnReply_UnknownParent_Ignored(t *testing.T) {
	b := NewBackend(&fakeSender{}, "C", nil)
	if err := b.OnReply(context.Background(), ReplyEvent{ParentMessageID: "nope", Content: "approve"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
