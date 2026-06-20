package escalation

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeGraph is a static parent map for tests.
type fakeGraph struct{ parents map[string]ParentRef }

func (g fakeGraph) Parent(_ context.Context, id string) (ParentRef, error) {
	if p, ok := g.parents[id]; ok {
		return p, nil
	}
	return ParentRef{Kind: NodeNone}, nil
}

// fakeMessenger records deliveries.
type fakeMessenger struct {
	mu        sync.Mutex
	delivered []struct {
		to  string
		msg EscalationMessage
	}
}

func (m *fakeMessenger) Deliver(_ context.Context, to string, msg EscalationMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delivered = append(m.delivered, struct {
		to  string
		msg EscalationMessage
	}{to, msg})
	return nil
}

func (m *fakeMessenger) count() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.delivered) }

// fakeHuman records human dispatches.
type fakeHuman struct {
	mu  sync.Mutex
	hit int
}

func (h *fakeHuman) ToHuman(_ context.Context, _ *Escalation) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hit++
	return nil
}

func (h *fakeHuman) count() int { h.mu.Lock(); defer h.mu.Unlock(); return h.hit }

// newTestEngine builds an Engine with fakes and returns its parts.
func newTestEngine(t *testing.T, parents map[string]ParentRef, vroomEntry *ParentRef) (*Engine, *fakeMessenger, *fakeHuman, *bytes.Buffer) {
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
		VROOMEntry: vroomEntry,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng, msg, human, buf
}

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

func TestRaise_AutoResolve_NeverRoutes(t *testing.T) {
	eng, msg, human, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
	}, nil)
	esc, err := eng.Raise(context.Background(), RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeBlocking,
		Question: "Should I proceed with the task you asked me to do?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	if esc.Phase != PhaseAutoResolved {
		t.Errorf("phase = %q, want auto-resolved", esc.Phase)
	}
	if esc.Answer == "" {
		t.Errorf("expected an auto answer")
	}
	if msg.count() != 0 {
		t.Errorf("auto-resolved escalation should not deliver any message, got %d", msg.count())
	}
	if human.count() != 0 {
		t.Errorf("auto-resolved escalation must never reach the human")
	}
}

func TestRaise_RoutesToSupervisor_ThenAnswer(t *testing.T) {
	eng, msg, _, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
	}, nil)
	ctx := context.Background()
	esc, err := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Which logging library does this repo prefer?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	if esc.Phase != PhaseRouted || esc.CurrentSessionID != "sup" {
		t.Fatalf("want routed to sup, got phase=%q current=%q", esc.Phase, esc.CurrentSessionID)
	}
	if msg.count() != 1 {
		t.Errorf("expected 1 delivery to supervisor, got %d", msg.count())
	}

	got, err := eng.Answer(ctx, esc.ID, "sup", "lead", "Use log/slog.")
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if got.Phase != PhaseAnswered || got.Answer != "Use log/slog." {
		t.Errorf("unexpected answered state: %+v", got)
	}
	// One delivery routing up + one delivery of the answer back to origin.
	if msg.count() != 2 {
		t.Errorf("expected 2 deliveries total, got %d", msg.count())
	}
}

func TestForward_WalksChain_ToVROOM_ToHuman(t *testing.T) {
	eng, _, human, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
		"sup":    {SessionID: "orch", Role: "orchestrator", Kind: NodeVROOMOrchestrator},
	}, nil)
	ctx := context.Background()
	esc, err := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Which deployment target should the nightly job use?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	// sup can't answer → forward to its parent (a VROOM node) → conferring.
	esc, err = eng.Forward(ctx, esc.ID, "sup", "lead", "out of my scope")
	if err != nil {
		t.Fatalf("Forward to VROOM: %v", err)
	}
	if esc.Phase != PhaseConferring || esc.CurrentSessionID != "orch" {
		t.Fatalf("want conferring at orch, got phase=%q current=%q", esc.Phase, esc.CurrentSessionID)
	}
	// VROOM trio can't answer → forward → human.
	esc, err = eng.Forward(ctx, esc.ID, "orch", "orchestrator", "trio could not resolve")
	if err != nil {
		t.Fatalf("Forward to human: %v", err)
	}
	if esc.Phase != PhaseDispatchedToHuman {
		t.Fatalf("want dispatched-to-human, got %q", esc.Phase)
	}
	if human.count() != 1 {
		t.Errorf("human dispatch count = %d, want 1", human.count())
	}
	// Human answers.
	esc, err = eng.Answer(ctx, esc.ID, HumanSessionID, "human", "Use staging.")
	if err != nil {
		t.Fatalf("human Answer: %v", err)
	}
	if esc.Phase != PhaseAnswered {
		t.Errorf("want answered, got %q", esc.Phase)
	}
}

func TestMustReachHuman_SupervisorCannotAnswer(t *testing.T) {
	eng, _, _, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
	}, nil)
	ctx := context.Background()
	esc, err := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindDecision, Mode: ModeAsync,
		Question: "Should we make this product decision to sunset the API?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	if !esc.MustReachHuman {
		t.Fatalf("expected MustReachHuman")
	}
	// A supervisor must not be able to answer it.
	if _, err := eng.Answer(ctx, esc.ID, "sup", "lead", "Yes, sunset it."); err == nil {
		t.Errorf("expected Answer by supervisor to be refused for must-reach-human escalation")
	}
	// The human may.
	if _, err := eng.Answer(ctx, esc.ID, HumanSessionID, "human", "No — keep the API."); err != nil {
		t.Errorf("human Answer should succeed: %v", err)
	}
}

func TestRaise_OrphanGoesToHuman(t *testing.T) {
	// worker has no parent and no VROOM entry → straight to human.
	eng, _, human, _ := newTestEngine(t, map[string]ParentRef{}, nil)
	esc, err := eng.Raise(context.Background(), RaiseRequest{
		OriginSessionID: "lonely", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Where is the deployment config stored?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	if esc.Phase != PhaseDispatchedToHuman {
		t.Errorf("orphan want dispatched-to-human, got %q", esc.Phase)
	}
	if human.count() != 1 {
		t.Errorf("human count = %d, want 1", human.count())
	}
}

func TestRaise_OrphanUsesVROOMEntry(t *testing.T) {
	entry := &ParentRef{SessionID: "meta-o", Role: "meta-orchestrator", Kind: NodeVROOMMetaOrchestrator}
	eng, _, human, _ := newTestEngine(t, map[string]ParentRef{}, entry)
	esc, err := eng.Raise(context.Background(), RaiseRequest{
		OriginSessionID: "lonely", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Which queue should I pull from?",
	})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	if esc.Phase != PhaseConferring || esc.CurrentSessionID != "meta-o" {
		t.Errorf("want conferring at meta-o, got phase=%q current=%q", esc.Phase, esc.CurrentSessionID)
	}
	if human.count() != 0 {
		t.Errorf("should route to VROOM entry, not human")
	}
}

func TestForward_OnlyHolderMayForward(t *testing.T) {
	eng, _, _, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
	}, nil)
	ctx := context.Background()
	esc, _ := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Which config flag toggles verbose mode?",
	})
	if _, err := eng.Forward(ctx, esc.ID, "someone-else", "x", "note"); err == nil {
		t.Errorf("expected forward from non-holder to be refused")
	}
}

func TestAwait_BlockingResolves(t *testing.T) {
	eng, _, _, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
	}, nil)
	ctx := context.Background()
	esc, _ := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeBlocking,
		Question: "Which branch is the release branch?",
	})
	// Answer from another goroutine shortly after.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = eng.Answer(context.Background(), esc.ID, "sup", "lead", "release/v2")
	}()
	got, err := eng.Await(ctx, esc.ID, time.Second, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if got.Phase != PhaseAnswered || got.Answer != "release/v2" {
		t.Errorf("await got %+v", got)
	}
}

func TestAwait_Timeout(t *testing.T) {
	eng, _, _, _ := newTestEngine(t, map[string]ParentRef{
		"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
	}, nil)
	ctx := context.Background()
	esc, _ := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeBlocking,
		Question: "Which icon set should the toolbar use?",
	})
	_, err := eng.Await(ctx, esc.ID, 30*time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, ErrAwaitTimeout) {
		t.Errorf("want ErrAwaitTimeout, got %v", err)
	}
}
