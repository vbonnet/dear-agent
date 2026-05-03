package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// memoryHITLBackend is a tiny in-process backend used by these tests so
// that runner integration can be exercised without a goroutine race against
// real polling.
type memoryHITLBackend struct {
	mu      sync.Mutex
	resolve map[string]HITLResolution
	notify  map[string]chan struct{}
	calls   int
}

func newMemoryHITLBackend() *memoryHITLBackend {
	return &memoryHITLBackend{
		resolve: map[string]HITLResolution{},
		notify:  map[string]chan struct{}{},
	}
}

func (b *memoryHITLBackend) Request(_ context.Context, req HITLRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	if _, ok := b.notify[req.ApprovalID]; !ok {
		b.notify[req.ApprovalID] = make(chan struct{}, 1)
	}
	return nil
}

func (b *memoryHITLBackend) Decide(approvalID string, dec HITLDecision, approver, role string) {
	b.mu.Lock()
	if _, ok := b.notify[approvalID]; !ok {
		b.notify[approvalID] = make(chan struct{}, 1)
	}
	b.resolve[approvalID] = HITLResolution{
		ApprovalID: approvalID,
		Decision:   dec,
		Approver:   approver,
		Role:       role,
		ResolvedAt: time.Now(),
	}
	ch := b.notify[approvalID]
	b.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (b *memoryHITLBackend) Wait(ctx context.Context, approvalID string) (HITLResolution, error) {
	b.mu.Lock()
	if r, ok := b.resolve[approvalID]; ok {
		b.mu.Unlock()
		return r, nil
	}
	if _, ok := b.notify[approvalID]; !ok {
		b.notify[approvalID] = make(chan struct{}, 1)
	}
	ch := b.notify[approvalID]
	b.mu.Unlock()
	select {
	case <-ch:
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.resolve[approvalID], nil
	case <-ctx.Done():
		return HITLResolution{}, ctx.Err()
	}
}

// TestHITL_BlockAlways_ApprovedNodeRuns verifies the canonical happy path:
// a node with block_policy=always pauses, the backend approves, and the
// node transitions to succeeded.
func TestHITL_BlockAlways_ApprovedNodeRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	backend := newMemoryHITLBackend()

	wf := &Workflow{
		Name:    "hitl-approved",
		Version: "1",
		Nodes: []Node{
			{
				ID:   "review",
				Kind: KindBash,
				Bash: &BashNode{Cmd: "echo done"},
				HITL: &HITLPolicy{BlockPolicy: "always", ApproverRole: "reviewer"},
			},
		},
	}

	r := NewRunner(nil)
	r.UseSQLiteState(ss)
	r.HITLBackend = backend

	// Resolve the request as soon as the runner posts it. Polling on the
	// memory backend uses a notify channel, so a small goroutine is fine.
	go func() {
		// Wait until the runner has registered the approval.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pending, _ := ListPendingHITLRequests(context.Background(), ss.DB())
			if len(pending) > 0 {
				backend.Decide(pending[0].ApprovalID, HITLDecisionApprove, "alice", "reviewer")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	rep, err := r.Run(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("run did not succeed: %v", rep)
	}
	if rep.Results[0].Error != nil {
		t.Fatalf("node errored: %v", rep.Results[0].Error)
	}
}

// TestHITL_Reject_FailsNode confirms a rejection flows back into a node
// failure with a hitl-rejected error message.
func TestHITL_Reject_FailsNode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()
	backend := newMemoryHITLBackend()

	wf := &Workflow{
		Name:    "hitl-reject",
		Version: "1",
		Nodes: []Node{
			{
				ID:   "guard",
				Kind: KindBash,
				Bash: &BashNode{Cmd: "true"},
				HITL: &HITLPolicy{BlockPolicy: "always"},
			},
		},
	}
	r := NewRunner(nil)
	r.UseSQLiteState(ss)
	r.HITLBackend = backend

	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pending, _ := ListPendingHITLRequests(context.Background(), ss.DB())
			if len(pending) > 0 {
				backend.Decide(pending[0].ApprovalID, HITLDecisionReject, "bob", "")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	_, err = r.Run(context.Background(), wf, nil)
	if err == nil {
		t.Fatal("expected run error from rejection, got nil")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("error %q does not mention rejection", err.Error())
	}
}

// TestHITL_Timeout_AppliesPolicy walks the three OnTimeout values and
// confirms each produces the right outcome.
func TestHITL_Timeout_AppliesPolicy(t *testing.T) {
	cases := []struct {
		name      string
		onTimeout string
		wantErr   bool
	}{
		{"timeout-default-fails", "", true},
		{"timeout-reject-fails", "reject", true},
		{"timeout-escalate-fails-v1", "escalate", true},
		{"timeout-approve-resumes", "approve", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "runs.db")
			ss, err := OpenSQLiteState(dbPath)
			if err != nil {
				t.Fatalf("OpenSQLiteState: %v", err)
			}
			defer ss.Close()

			// Backend that never resolves — the runner must rely on Timeout.
			backend := newMemoryHITLBackend()

			wf := &Workflow{
				Name:    "hitl-timeout-" + tc.onTimeout,
				Version: "1",
				Nodes: []Node{
					{
						ID:   "deferred",
						Kind: KindBash,
						Bash: &BashNode{Cmd: "true"},
						HITL: &HITLPolicy{
							BlockPolicy: "always",
							Timeout:     50 * time.Millisecond,
							OnTimeout:   tc.onTimeout,
						},
					},
				},
			}
			r := NewRunner(nil)
			r.UseSQLiteState(ss)
			r.HITLBackend = backend

			rep, err := r.Run(context.Background(), wf, nil)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Errorf("err=%v want_err=%v rep=%+v", err, tc.wantErr, rep)
			}
			if !tc.wantErr && !rep.Succeeded {
				t.Errorf("expected success, got %+v", rep)
			}
		})
	}
}

// TestHITL_OnLowConfidence_BlocksWhenBelow verifies the on_low_confidence
// policy fires when the node reports a confidence under the threshold and
// passes through cleanly when the confidence is high.
func TestHITL_OnLowConfidence_BlocksWhenBelow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	// Bash node with HITL on_low_confidence; we inject confidence via a
	// hook that mutates Result.Meta after the body runs.
	r := NewRunner(nil)
	r.UseSQLiteState(ss)
	r.HITLBackend = newMemoryHITLBackend()

	// Direct test of shouldBlockOnHITL — the runner integration is
	// covered by the always/reject/timeout cases above.
	cases := []struct {
		name       string
		conf       float64
		threshold  float64
		wantBlock  bool
	}{
		{"low-blocks", 0.3, 0.5, true},
		{"high-passes", 0.9, 0.5, false},
		{"equal-passes", 0.5, 0.5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := &Node{HITL: &HITLPolicy{
				BlockPolicy:         "on_low_confidence",
				ConfidenceThreshold: tc.threshold,
			}}
			res := &Result{Meta: map[string]any{"confidence": tc.conf}}
			gotBlock, _ := shouldBlockOnHITL(node, res)
			if gotBlock != tc.wantBlock {
				t.Errorf("blocked=%v want=%v (conf=%.2f thresh=%.2f)", gotBlock, tc.wantBlock, tc.conf, tc.threshold)
			}
		})
	}
}

// TestHITL_NoBackend_TimesOut confirms the substrate guarantee: with no
// backend, an awaiting_hitl node times out immediately. on_timeout=approve
// resumes; default fails.
func TestHITL_NoBackend_TimesOut(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	wf := &Workflow{
		Name:    "hitl-no-backend",
		Version: "1",
		Nodes: []Node{
			{
				ID:   "approve-on-timeout",
				Kind: KindBash,
				Bash: &BashNode{Cmd: "true"},
				HITL: &HITLPolicy{BlockPolicy: "always", OnTimeout: "approve"},
			},
		},
	}
	r := NewRunner(nil)
	r.UseSQLiteState(ss)
	// HITLBackend left nil intentionally
	rep, err := r.Run(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("expected success via on_timeout=approve fallback, got %+v", rep)
	}
}

// TestRecordHITLDecision_RoleMismatch verifies the approver_role gate
// rejects a decision from the wrong role.
func TestRecordHITLDecision_RoleMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	ctx := context.Background()
	if err := ss.BeginRun(ctx, RunRecord{RunID: "r1", WorkflowName: "wf", State: RunStateRunning, InputsJSON: "{}", StartedAt: time.Now()}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := ss.UpsertNode(ctx, NodeRecord{RunID: "r1", NodeID: "n1", State: NodeStateRunning}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	approvalID, err := CreateHITLRequest(ctx, ss.DB(), "r1", "n1", "reviewer", "needs human", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}
	err = RecordHITLDecision(ctx, ss.DB(), approvalID, HITLDecisionApprove, "alice", "scribe", "lgtm", time.Now())
	if !errors.Is(err, ErrApproverRoleMismatch) {
		t.Errorf("expected ErrApproverRoleMismatch, got %v", err)
	}
	if err := RecordHITLDecision(ctx, ss.DB(), approvalID, HITLDecisionApprove, "bob", "reviewer", "lgtm", time.Now()); err != nil {
		t.Fatalf("RecordHITLDecision: %v", err)
	}
	// Second call must error — already resolved.
	err = RecordHITLDecision(ctx, ss.DB(), approvalID, HITLDecisionReject, "bob", "reviewer", "", time.Now())
	if !errors.Is(err, ErrApprovalAlreadyResolved) {
		t.Errorf("expected ErrApprovalAlreadyResolved, got %v", err)
	}
}
