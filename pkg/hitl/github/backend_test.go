package github

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// fakeCommenter is a test double that records PostComment calls and can
// have canned ListComments replies queued by the test.
type fakeCommenter struct {
	mu       sync.Mutex
	nextID   int64
	posts    []string          // bodies posted, in order
	comments map[int][]Comment // queued replies by prNumber
}

func newFakeCommenter() *fakeCommenter {
	return &fakeCommenter{comments: map[int][]Comment{}}
}

func (f *fakeCommenter) PostComment(_ context.Context, _, _ string, prNum int, body string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.posts = append(f.posts, body)
	return f.nextID, nil
}

func (f *fakeCommenter) ListComments(_ context.Context, _, _ string, prNum int, sinceID int64) ([]Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Comment
	for _, c := range f.comments[prNum] {
		if c.ID > sinceID {
			out = append(out, c)
		}
	}
	return out, nil
}

// addReply queues a reply comment that ListComments will return.
func (f *fakeCommenter) addReply(prNum int, id int64, body, user string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments[prNum] = append(f.comments[prNum], Comment{
		ID:        id,
		Body:      body,
		UserLogin: user,
		CreatedAt: time.Now(),
	})
}

func newTestBackend(c Commenter) *Backend {
	b := NewBackend(c, "owner", "repo", 42)
	b.PollInterval = 5 * time.Millisecond // fast polling for tests
	return b
}

func newTestRequest(id string) workflow.HITLRequest {
	return workflow.HITLRequest{
		ApprovalID:   id,
		WorkflowName: "test-workflow",
		NodeID:       "gate",
		ApproverRole: "engineer",
		Reason:       "needs human check",
	}
}

func TestRequest_PostsComment(t *testing.T) {
	fc := newFakeCommenter()
	b := newTestBackend(fc)
	if err := b.Request(context.Background(), newTestRequest("abc-123")); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(fc.posts) != 1 {
		t.Fatalf("want 1 posted comment, got %d", len(fc.posts))
	}
	body := fc.posts[0]
	if !strings.Contains(body, "abc-123") {
		t.Errorf("comment body should contain approvalID abc-123; got:\n%s", body)
	}
	if !strings.Contains(body, "test-workflow") {
		t.Errorf("comment body should contain workflow name; got:\n%s", body)
	}
}

func TestWait_ApproveReply(t *testing.T) {
	fc := newFakeCommenter()
	b := newTestBackend(fc)
	req := newTestRequest("approve-me")
	if err := b.Request(context.Background(), req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	// Queue an approve reply after our request comment (id=1).
	fc.addReply(42, 2, "approve looks good", "alice")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := b.Wait(ctx, "approve-me")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Decision != workflow.HITLDecisionApprove {
		t.Errorf("want Approve, got %s", res.Decision)
	}
	if res.Approver != "alice" {
		t.Errorf("want approver alice, got %s", res.Approver)
	}
}

func TestWait_RejectReply(t *testing.T) {
	fc := newFakeCommenter()
	b := newTestBackend(fc)
	req := newTestRequest("reject-me")
	if err := b.Request(context.Background(), req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	fc.addReply(42, 2, "reject too risky", "bob")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := b.Wait(ctx, "reject-me")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Decision != workflow.HITLDecisionReject {
		t.Errorf("want Reject, got %s", res.Decision)
	}
}

func TestWait_IgnoresNonDecisionComments(t *testing.T) {
	fc := newFakeCommenter()
	b := newTestBackend(fc)
	req := newTestRequest("wait-for-it")
	if err := b.Request(context.Background(), req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	// Non-decision comment first, then approve.
	fc.addReply(42, 2, "can you clarify what this does?", "charlie")
	fc.addReply(42, 3, "lgtm", "dave")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := b.Wait(ctx, "wait-for-it")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Decision != workflow.HITLDecisionApprove {
		t.Errorf("want Approve (from lgtm), got %s", res.Decision)
	}
}

func TestWait_ContextCancellation(t *testing.T) {
	fc := newFakeCommenter()
	b := newTestBackend(fc)
	req := newTestRequest("never-replied")
	if err := b.Request(context.Background(), req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := b.Wait(ctx, "never-replied")
	if err == nil {
		t.Error("expected context-timeout error, got nil")
	}
}

func TestParseDecision(t *testing.T) {
	cases := []struct {
		input  string
		want   workflow.HITLDecision
		wantOK bool
	}{
		{"approve", workflow.HITLDecisionApprove, true},
		{"LGTM", workflow.HITLDecisionApprove, true},
		{"ok looks fine", workflow.HITLDecisionApprove, true},
		{"yes!", workflow.HITLDecisionApprove, true},
		{"reject this is broken", workflow.HITLDecisionReject, true},
		{"deny", workflow.HITLDecisionReject, true},
		{"no", workflow.HITLDecisionReject, true},
		{"can you clarify?", "", false},
		{"", "", false},
		{"maybe", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := parseDecision(tc.input)
			if ok != tc.wantOK {
				t.Errorf("parseDecision(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("parseDecision(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRenderRequest_ContainsKeyFields(t *testing.T) {
	req := workflow.HITLRequest{
		ApprovalID:   "test-id-42",
		WorkflowName: "deploy-prod",
		NodeID:       "final-gate",
		ApproverRole: "sre",
		Reason:       "production deployment",
		Confidence:   0.87,
		NodeOutput:   "Output: success",
	}
	body := renderRequest(req)
	for _, want := range []string{"test-id-42", "deploy-prod", "final-gate", "sre", "production deployment"} {
		if !strings.Contains(body, want) {
			t.Errorf("renderRequest: body missing %q\nbody:\n%s", want, body)
		}
	}
}

func TestRequest_NilClient(t *testing.T) {
	b := &Backend{pending: map[string]*pendingApproval{}}
	err := b.Request(context.Background(), newTestRequest("noop"))
	if err == nil {
		t.Error("Request with nil client should fail")
	}
}

func TestBackend_ImplementsHITLBackend(t *testing.T) {
	// Compile-time check that *Backend satisfies workflow.HITLBackend.
	var _ workflow.HITLBackend = (*Backend)(nil)
}
