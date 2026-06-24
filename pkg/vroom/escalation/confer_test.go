package escalation

import (
	"bytes"
	"context"
	"testing"
)

// trio is the standard three-member registry used across confer tests.
func trio() *StaticRegistry {
	return NewStaticRegistry(
		ParentRef{SessionID: "meta-o", Role: "meta-orchestrator", Kind: NodeVROOMMetaOrchestrator},
		ParentRef{SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
		ParentRef{SessionID: "overseer", Role: "overseer", Kind: NodeVROOMOverseer},
	)
}

// newConferEngine builds an Engine wired with a registry so reaching the mesh
// opens a programmatic confer.
func newConferEngine(t *testing.T, parents map[string]ParentRef, reg VROOMRegistry, entry *ParentRef) (*Engine, *fakeMessenger, *fakeHuman) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := NewLogger(NewJSONLSink(nopCloser{buf}))
	msg := &fakeMessenger{}
	human := &fakeHuman{}
	eng, err := NewEngine(Config{
		Graph:      fakeGraph{parents: parents},
		Messenger:  msg,
		Human:      human,
		Store:      NewMemStore(),
		Logger:     logger,
		Registry:   reg,
		VROOMEntry: entry,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng, msg, human
}

// raiseToConfer raises an escalation whose origin's parent is a VROOM node, so
// the first hop opens the confer.
func raiseToConfer(t *testing.T, eng *Engine, question string, kind Kind) *Escalation {
	t.Helper()
	esc, err := eng.Raise(context.Background(), RaiseRequest{
		OriginSessionID: "worker", OriginRole: "coder", Kind: kind, Mode: ModeAsync,
		Question: question,
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	return esc
}

func TestConfer_OpensAcrossWholeTrio(t *testing.T) {
	eng, msg, _ := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	esc := raiseToConfer(t, eng, "Which deploy target for the nightly job?", KindQuestion)

	if esc.Phase != PhaseConferring {
		t.Fatalf("phase = %q, want conferring", esc.Phase)
	}
	if esc.CurrentSessionID != VROOMTrioSessionID {
		t.Errorf("current holder = %q, want %q", esc.CurrentSessionID, VROOMTrioSessionID)
	}
	if esc.Confer == nil || len(esc.Confer.Members) != 3 {
		t.Fatalf("confer not opened across 3 members: %+v", esc.Confer)
	}
	if esc.Confer.Quorum != 2 {
		t.Errorf("quorum = %d, want 2", esc.Confer.Quorum)
	}
	// Delivered to all three trio members.
	if msg.count() != 3 {
		t.Errorf("deliveries = %d, want 3 (one per member)", msg.count())
	}
}

func TestConfer_QuorumApproveResolves(t *testing.T) {
	eng, msg, human := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which logging library is preferred here?", KindQuestion)

	// First approval: not yet quorum.
	got, err := eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteApprove, "Use log/slog.", "stdlib")
	if err != nil {
		t.Fatalf("CastVote 1: %v", err)
	}
	if got.Phase != PhaseConferring {
		t.Fatalf("after 1 vote phase = %q, want still conferring", got.Phase)
	}
	// Second approval with the same answer → quorum → answered.
	got, err = eng.CastVote(ctx, esc.ID, "orch", "orchestrator", VoteApprove, "Use log/slog.", "agreed")
	if err != nil {
		t.Fatalf("CastVote 2: %v", err)
	}
	if got.Phase != PhaseAnswered {
		t.Fatalf("after quorum phase = %q, want answered", got.Phase)
	}
	if got.Answer != "Use log/slog." {
		t.Errorf("answer = %q, want %q", got.Answer, "Use log/slog.")
	}
	if got.AnsweredBy != VROOMTrioSessionID {
		t.Errorf("answered_by = %q, want %q", got.AnsweredBy, VROOMTrioSessionID)
	}
	if human.count() != 0 {
		t.Errorf("quorum answer must not reach the human")
	}
	// 3 confer-ask deliveries + 1 answer-to-origin delivery.
	if msg.count() != 4 {
		t.Errorf("deliveries = %d, want 4", msg.count())
	}
}

func TestConfer_QuorumRejectGoesToHuman(t *testing.T) {
	eng, _, human := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which vendor should we standardise on?", KindQuestion)

	if _, err := eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteReject, "", "needs human"); err != nil {
		t.Fatalf("CastVote 1: %v", err)
	}
	got, err := eng.CastVote(ctx, esc.ID, "orch", "orchestrator", VoteReject, "", "agree, human")
	if err != nil {
		t.Fatalf("CastVote 2: %v", err)
	}
	if got.Phase != PhaseDispatchedToHuman {
		t.Fatalf("phase = %q, want dispatched-to-human", got.Phase)
	}
	if human.count() != 1 {
		t.Errorf("human count = %d, want 1", human.count())
	}
}

func TestConfer_SplitVoteNoQuorumGoesToHuman(t *testing.T) {
	eng, _, human := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which of three approaches is best?", KindQuestion)

	// Three different answers — no answer reaches quorum; once all have voted the
	// trio is deadlocked and it dispatches to the human.
	_, _ = eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteApprove, "Approach A", "")
	_, _ = eng.CastVote(ctx, esc.ID, "orch", "orchestrator", VoteApprove, "Approach B", "")
	got, err := eng.CastVote(ctx, esc.ID, "overseer", "overseer", VoteApprove, "Approach C", "")
	if err != nil {
		t.Fatalf("CastVote 3: %v", err)
	}
	if got.Phase != PhaseDispatchedToHuman {
		t.Fatalf("phase = %q, want dispatched-to-human (deadlock)", got.Phase)
	}
	if human.count() != 1 {
		t.Errorf("human count = %d, want 1", human.count())
	}
}

func TestConfer_MustReachHuman_TrioOnlyAdvises(t *testing.T) {
	eng, _, human := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Should we make this product decision to sunset the API?", KindDecision)
	if !esc.MustReachHuman {
		t.Fatalf("expected MustReachHuman")
	}

	// Even a quorum of approvals must not answer a must-reach-human escalation;
	// after a quorum weighs in it dispatches to the human with the recommendation.
	if _, err := eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteApprove, "Sunset it", "rec"); err != nil {
		t.Fatalf("CastVote 1: %v", err)
	}
	got, err := eng.CastVote(ctx, esc.ID, "orch", "orchestrator", VoteApprove, "Sunset it", "rec")
	if err != nil {
		t.Fatalf("CastVote 2: %v", err)
	}
	if got.Phase != PhaseDispatchedToHuman {
		t.Fatalf("phase = %q, want dispatched-to-human", got.Phase)
	}
	if got.Answer != "" {
		t.Errorf("must-reach-human escalation must not carry a trio answer, got %q", got.Answer)
	}
	if human.count() != 1 {
		t.Errorf("human count = %d, want 1", human.count())
	}
}

func TestConfer_NonMemberCannotVote(t *testing.T) {
	eng, _, _ := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which config layout do we use?", KindQuestion)

	if _, err := eng.CastVote(ctx, esc.ID, "stranger", "x", VoteApprove, "this", ""); err == nil {
		t.Errorf("expected vote by non-member to be refused")
	}
}

func TestConfer_DoubleVoteRefused(t *testing.T) {
	eng, _, _ := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which timeout default is right?", KindQuestion)

	if _, err := eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteApprove, "30s", ""); err != nil {
		t.Fatalf("CastVote 1: %v", err)
	}
	if _, err := eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteReject, "", "changed mind"); err == nil {
		t.Errorf("expected a second vote from the same member to be refused")
	}
}

func TestConfer_ApproveRequiresAnswer(t *testing.T) {
	eng, _, _ := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which port should the service bind?", KindQuestion)

	if _, err := eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteApprove, "  ", ""); err == nil {
		t.Errorf("expected an approve with no answer to be refused")
	}
}

func TestConfer_MemberMayNotAnswerUnilaterally(t *testing.T) {
	eng, _, _ := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which retry policy applies?", KindQuestion)

	// A trio member must vote, not Answer directly, while a confer is in flight.
	if _, err := eng.Answer(ctx, esc.ID, "meta-o", "meta-orchestrator", "3 retries"); err == nil {
		t.Errorf("expected a trio member's unilateral Answer to be refused during confer")
	}
	// The human may still answer (override).
	if _, err := eng.Answer(ctx, esc.ID, HumanSessionID, "human", "3 retries"); err != nil {
		t.Errorf("human override Answer should succeed: %v", err)
	}
}

func TestConfer_MemberMayForwardToHuman(t *testing.T) {
	eng, _, human := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, trio(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which region for the new cluster?", KindQuestion)

	// Any trio member holding the confer may forward (give up) → human.
	got, err := eng.Forward(ctx, esc.ID, "overseer", "overseer", "trio cannot resolve")
	if err != nil {
		t.Fatalf("Forward by member: %v", err)
	}
	if got.Phase != PhaseDispatchedToHuman {
		t.Fatalf("phase = %q, want dispatched-to-human", got.Phase)
	}
	if human.count() != 1 {
		t.Errorf("human count = %d, want 1", human.count())
	}
}

func TestConfer_EmptyRegistryFallsBackToSingleNode(t *testing.T) {
	// A registry that yields no live members must degrade to single-node
	// conferring against the graph-walk's VROOM parent.
	eng, msg, _ := newConferEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
		"sup":    {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, NewStaticRegistry(), nil)
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which migration order is safe?", KindQuestion)
	// Reached sup (ordinary hop) first.
	if esc.Phase != PhaseRouted || esc.CurrentSessionID != "sup" {
		t.Fatalf("want routed to sup, got phase=%q current=%q", esc.Phase, esc.CurrentSessionID)
	}
	esc, err := eng.Forward(ctx, esc.ID, "sup", "lead", "out of scope")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if esc.Phase != PhaseConferring || esc.CurrentSessionID != "orch" {
		t.Fatalf("empty registry should fall back to single-node confer at orch, got phase=%q current=%q", esc.Phase, esc.CurrentSessionID)
	}
	if esc.Confer != nil {
		t.Errorf("single-node fallback should not open a Confer ballot")
	}
	// 1 hop to sup + 1 hop to orch.
	if msg.count() != 2 {
		t.Errorf("deliveries = %d, want 2", msg.count())
	}
}

func TestConfer_OrphanViaVROOMEntryOpensConfer(t *testing.T) {
	// An orphan routed via VROOMEntry should also open the full trio confer.
	entry := &ParentRef{SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator}
	eng, msg, _ := newConferEngine(t, map[string]ParentRef{}, trio(), entry)
	esc := raiseToConfer(t, eng, "Which queue should I pull from?", KindQuestion)
	if esc.Phase != PhaseConferring || esc.Confer == nil || len(esc.Confer.Members) != 3 {
		t.Fatalf("orphan via VROOMEntry should open trio confer, got phase=%q confer=%+v", esc.Phase, esc.Confer)
	}
	if msg.count() != 3 {
		t.Errorf("deliveries = %d, want 3", msg.count())
	}
}

func TestConfer_CustomQuorum(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(NewJSONLSink(nopCloser{buf}))
	msg := &fakeMessenger{}
	human := &fakeHuman{}
	eng, err := NewEngine(Config{
		Graph:        fakeGraph{parents: map[string]ParentRef{"worker": {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator}}},
		Messenger:    msg,
		Human:        human,
		Store:        NewMemStore(),
		Logger:       logger,
		Registry:     trio(),
		ConferQuorum: 3, // unanimity
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	ctx := context.Background()
	esc := raiseToConfer(t, eng, "Which timeout value should the gateway use?", KindQuestion)
	if esc.Confer.Quorum != 3 {
		t.Fatalf("quorum = %d, want 3", esc.Confer.Quorum)
	}
	// Two approvals are not enough under unanimity.
	_, _ = eng.CastVote(ctx, esc.ID, "meta-o", "meta-orchestrator", VoteApprove, "X", "")
	got, _ := eng.CastVote(ctx, esc.ID, "orch", "orchestrator", VoteApprove, "X", "")
	if got.Phase != PhaseConferring {
		t.Fatalf("2/3 under unanimity should still confer, got %q", got.Phase)
	}
	// Third approval reaches unanimity.
	got, _ = eng.CastVote(ctx, esc.ID, "overseer", "overseer", VoteApprove, "X", "")
	if got.Phase != PhaseAnswered || got.Answer != "X" {
		t.Fatalf("unanimous approve should answer, got phase=%q answer=%q", got.Phase, got.Answer)
	}
}
