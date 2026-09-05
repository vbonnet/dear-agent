package supervisor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/escalation"
)

// fakeInbox is a minimal EscalationInbox: it returns a fixed pending list and
// records the answer/forward calls made against it.
type fakeInbox struct {
	pending    []*escalation.Escalation
	listErr    error
	answerErr  error
	forwardErr error
	answerEsc  *escalation.Escalation
	forwardEsc *escalation.Escalation

	answered  []string
	forwarded []string
}

func (f *fakeInbox) List(_ context.Context, _ escalation.Filter) ([]*escalation.Escalation, error) {
	return f.pending, f.listErr
}

func (f *fakeInbox) Answer(_ context.Context, id, _, _, _ string) (*escalation.Escalation, error) {
	if f.answerErr != nil {
		return f.answerEsc, f.answerErr
	}
	f.answered = append(f.answered, id)
	return &escalation.Escalation{ID: id}, nil
}

func (f *fakeInbox) Forward(_ context.Context, id, _, _, _ string) (*escalation.Escalation, error) {
	if f.forwardErr != nil {
		return f.forwardEsc, f.forwardErr
	}
	f.forwarded = append(f.forwarded, id)
	return &escalation.Escalation{ID: id}, nil
}

func escNamed(ids ...string) []*escalation.Escalation {
	out := make([]*escalation.Escalation, 0, len(ids))
	for _, id := range ids {
		out = append(out, &escalation.Escalation{ID: id})
	}
	return out
}

func TestNewInboxDrainer_Validation(t *testing.T) {
	if _, err := NewInboxDrainer(nil, "sup", "overseer", nil); err == nil {
		t.Error("nil inbox should error")
	}
	if _, err := NewInboxDrainer(&fakeInbox{}, "", "overseer", nil); err == nil {
		t.Error("empty sessionID should error")
	}
	d, err := NewInboxDrainer(&fakeInbox{}, "sup", "overseer", nil)
	if err != nil {
		t.Fatalf("valid args: %v", err)
	}
	if d.policy == nil {
		t.Error("nil policy should default to a forwarding policy")
	}
}

func TestDrain_DefaultPolicyForwardsAll(t *testing.T) {
	inbox := &fakeInbox{pending: escNamed("e1", "e2", "e3")}
	d, err := NewInboxDrainer(inbox, "sup", "overseer", nil) // nil → ForwardingDrainPolicy
	if err != nil {
		t.Fatalf("NewInboxDrainer: %v", err)
	}
	res, err := d.Drain(context.Background())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if res.Listed != 3 || res.Forwarded != 3 || res.Answered != 0 || res.Failed != 0 {
		t.Errorf("result = %+v, want listed=3 forwarded=3", res)
	}
	if len(inbox.forwarded) != 3 {
		t.Errorf("forwarded %d escalations, want 3", len(inbox.forwarded))
	}
	if len(inbox.answered) != 0 {
		t.Errorf("default policy must never answer, answered %d", len(inbox.answered))
	}
}

func TestDrain_PolicyActionsDispatch(t *testing.T) {
	inbox := &fakeInbox{pending: escNamed("ans", "fwd", "skip")}
	policy := func(_ context.Context, e *escalation.Escalation) DrainDecision {
		switch e.ID {
		case "ans":
			return DrainDecision{Action: DrainAnswer, Text: "42"}
		case "skip":
			return DrainDecision{Action: DrainSkip}
		default:
			return DrainDecision{Action: DrainForward, Text: "up"}
		}
	}
	d, err := NewInboxDrainer(inbox, "sup", "overseer", policy)
	if err != nil {
		t.Fatalf("NewInboxDrainer: %v", err)
	}
	res, err := d.Drain(context.Background())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if res.Answered != 1 || res.Forwarded != 1 || res.Skipped != 1 || res.Failed != 0 {
		t.Errorf("result = %+v, want answered=1 forwarded=1 skipped=1", res)
	}
	if len(inbox.answered) != 1 || inbox.answered[0] != "ans" {
		t.Errorf("answered = %v, want [ans]", inbox.answered)
	}
	if len(inbox.forwarded) != 1 || inbox.forwarded[0] != "fwd" {
		t.Errorf("forwarded = %v, want [fwd]", inbox.forwarded)
	}
}

func TestDrain_ListErrorPropagates(t *testing.T) {
	inbox := &fakeInbox{listErr: errors.New("store down")}
	d, _ := NewInboxDrainer(inbox, "sup", "overseer", nil)
	res, err := d.Drain(context.Background())
	if err == nil {
		t.Fatal("expected list error to propagate")
	}
	if res.Listed != 0 {
		t.Errorf("nothing should be listed on list error, got %d", res.Listed)
	}
}

func TestDrain_ContinuesPastPerEscalationError(t *testing.T) {
	// Forward fails for every escalation: each is counted as failed, but the
	// drainer must still attempt all of them (not stop at the first).
	inbox := &fakeInbox{pending: escNamed("e1", "e2"), forwardErr: errors.New("deliver failed")}
	d, _ := NewInboxDrainer(inbox, "sup", "overseer", nil)
	res, err := d.Drain(context.Background())
	if err == nil {
		t.Fatal("expected the first per-escalation error to be returned")
	}
	if res.Listed != 2 || res.Failed != 2 || res.Forwarded != 0 {
		t.Errorf("result = %+v, want listed=2 failed=2", res)
	}
}

func TestDrain_CountsCommittedTransitionWhoseEffectFailed(t *testing.T) {
	committed := &escalation.Escalation{ID: "e1", Phase: escalation.PhaseRouted}
	inbox := &fakeInbox{
		pending:    escNamed("e1"),
		forwardEsc: committed,
		forwardErr: &escalation.CommittedEffectError{
			EscalationID: "e1", Phase: escalation.PhaseRouted, Err: errors.New("delivery failed"),
		},
	}
	d, _ := NewInboxDrainer(inbox, "sup", "overseer", nil)
	res, err := d.Drain(context.Background())
	if err == nil {
		t.Fatal("expected post-commit effect error")
	}
	if res.Listed != 1 || res.Forwarded != 1 || res.Failed != 1 {
		t.Fatalf("result=%+v, want listed=1 forwarded=1 failed=1", res)
	}
}

func TestDrain_StopsWhenContextCancelledBetweenEscalations(t *testing.T) {
	inbox := &fakeInbox{pending: escNamed("e1", "e2")}
	ctx, cancel := context.WithCancel(context.Background())
	policy := func(_ context.Context, _ *escalation.Escalation) DrainDecision {
		cancel()
		return DrainDecision{Action: DrainForward, Text: "up"}
	}
	d, _ := NewInboxDrainer(inbox, "sup", "overseer", policy)
	res, err := d.Drain(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Drain error = %v, want context.Canceled", err)
	}
	if res.Listed != 2 || res.Forwarded != 1 {
		t.Errorf("result = %+v, want listed=2 forwarded=1", res)
	}
	if len(inbox.forwarded) != 1 || inbox.forwarded[0] != "e1" {
		t.Errorf("forwarded = %v, want [e1]", inbox.forwarded)
	}
}

// TestDrain_RealEngine confirms *escalation.Engine satisfies EscalationInbox and
// a forwarding drain advances a real escalation one hop up the chain.
func TestDrain_RealEngine(t *testing.T) {
	ctx := context.Background()
	// worker → sup(overseer) → human. The supervisor "sup" holds the escalation.
	eng, err := escalation.NewEngine(escalation.Config{
		Graph: staticGraph{parents: map[string]escalation.ParentRef{
			"worker": {SessionID: "sup", Role: "overseer", Kind: escalation.NodeSupervisor},
		}},
		Messenger: noopMessenger{},
		Human:     countingHuman{},
		Store:     escalation.NewMemStore(),
		Logger:    escalation.NewLogger(escalation.NewJSONLSink(&nopWriteCloser{&bytes.Buffer{}})),
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	esc, err := eng.Raise(ctx, escalation.RaiseRequest{
		OriginSessionID: "worker", Kind: escalation.KindQuestion, Mode: escalation.ModeAsync,
		Question: "Which logging library does this repo prefer?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	if esc.CurrentSessionID != "sup" {
		t.Fatalf("escalation should be held by sup, got %q", esc.CurrentSessionID)
	}

	d, err := NewInboxDrainer(eng, "sup", "overseer", nil)
	if err != nil {
		t.Fatalf("NewInboxDrainer: %v", err)
	}
	res, err := d.Drain(ctx)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if res.Listed != 1 || res.Forwarded != 1 {
		t.Errorf("result = %+v, want listed=1 forwarded=1", res)
	}
	// The escalation no longer sits with sup; a second drain finds an empty inbox.
	res2, err := d.Drain(ctx)
	if err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if res2.Listed != 0 {
		t.Errorf("after forwarding, sup's inbox should be empty, got %d", res2.Listed)
	}
}

// recordingDrainer counts Drain calls and returns a fixed result/error.
type recordingDrainer struct {
	calls atomic.Uint64
	res   DrainResult
	err   error
}

func (d *recordingDrainer) Drain(context.Context) (DrainResult, error) {
	d.calls.Add(1)
	return d.res, d.err
}

func TestLoop_Iterate_DrainsEscalationsAfterTick(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOverseer}
	drainer := &recordingDrainer{res: DrainResult{Listed: 2, Forwarded: 2}}
	trail, buf := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Drainer:    drainer,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	l.iterate(context.Background())

	if got := drainer.calls.Load(); got != 1 {
		t.Errorf("Drain called %d times, want 1", got)
	}
	// The drain pass is recorded to the trail with its counts.
	out := buf.String()
	if !strings.Contains(out, "supervisor.escalation.drain") {
		t.Errorf("trail missing drain record:\n%s", out)
	}
	if !strings.Contains(out, `"forwarded":2`) {
		t.Errorf("trail drain record missing forwarded count:\n%s", out)
	}
}

func TestLoop_Iterate_NilDrainerIsNoop(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOverseer}
	l, buf := newLoopForTest(t, sup, &fakePeerLookup{peers: map[Role]LoopStatus{}},
		CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }))

	l.iterate(context.Background())

	if strings.Contains(buf.String(), "supervisor.escalation.drain") {
		t.Errorf("nil drainer should emit no drain record:\n%s", buf.String())
	}
}

func TestLoop_Drain_RecordsErrorButDoesNotPanic(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOverseer}
	drainer := &recordingDrainer{res: DrainResult{Listed: 1, Failed: 1}, err: errors.New("deliver failed")}
	trail, buf := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Drainer:    drainer,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	// A drain error must be recorded but never abort the iteration.
	l.iterate(context.Background())

	if !strings.Contains(buf.String(), "deliver failed") {
		t.Errorf("trail should record the drain error:\n%s", buf.String())
	}
}

func TestLoop_Drain_SkippedOnCancelledContext(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOverseer}
	drainer := &recordingDrainer{}
	trail, _ := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Drainer:    drainer,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	l.drain(ctx)

	if got := drainer.calls.Load(); got != 0 {
		t.Errorf("drain on a cancelled context should be skipped, called %d times", got)
	}
}

// --- minimal escalation collaborators for the real-engine test ---

type staticGraph struct {
	parents map[string]escalation.ParentRef
}

func (g staticGraph) Parent(_ context.Context, id string) (escalation.ParentRef, error) {
	if p, ok := g.parents[id]; ok {
		return p, nil
	}
	return escalation.ParentRef{Kind: escalation.NodeNone}, nil
}

type noopMessenger struct{}

func (noopMessenger) Deliver(_ context.Context, _ string, _ escalation.EscalationMessage) error {
	return nil
}

type countingHuman struct{}

func (countingHuman) ToHuman(_ context.Context, _ *escalation.Escalation) error { return nil }
