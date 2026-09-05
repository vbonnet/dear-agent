package escalation

import (
	"bytes"
	"context"
	"errors"
	"strings"
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
	err       error
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
	return m.err
}

func (m *fakeMessenger) count() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.delivered) }

func (m *fakeMessenger) resolvedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, delivery := range m.delivered {
		if delivery.msg.Resolved {
			count++
		}
	}
	return count
}

// fakeHuman records human dispatches.
type fakeHuman struct {
	mu  sync.Mutex
	hit int
	err error
}

func (h *fakeHuman) ToHuman(_ context.Context, _ *Escalation) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hit++
	return h.err
}

func (h *fakeHuman) count() int { h.mu.Lock(); defer h.mu.Unlock(); return h.hit }

type recordingSink struct {
	mu     sync.Mutex
	events []EscalationEvent
}

type contextCheckingSink struct{ recordingSink }

func (s *contextCheckingSink) Record(ctx context.Context, ev EscalationEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.recordingSink.Record(ctx, ev)
}

type contextCheckingMessenger struct{ fakeMessenger }

func (m *contextCheckingMessenger) Deliver(ctx context.Context, to string, msg EscalationMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.fakeMessenger.Deliver(ctx, to, msg)
}

type deadlineMessenger struct{}

func (deadlineMessenger) Deliver(ctx context.Context, _ string, _ EscalationMessage) error {
	<-ctx.Done()
	return ctx.Err()
}

type cancelAfterCreateStore struct {
	Store
	cancel context.CancelFunc
}

func (s *cancelAfterCreateStore) Create(ctx context.Context, esc *Escalation) error {
	err := s.Store.Create(ctx, esc)
	if err == nil {
		s.cancel()
	}
	return err
}

type blockingGraph struct {
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
	parent  ParentRef
}

func (g *blockingGraph) Parent(ctx context.Context, _ string) (ParentRef, error) {
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return g.parent, nil
	case <-ctx.Done():
		return ParentRef{}, ctx.Err()
	}
}

func (s *recordingSink) Record(_ context.Context, ev EscalationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

func (*recordingSink) Close() error { return nil }

func (s *recordingSink) phaseCount(phase Phase) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, ev := range s.events {
		if ev.Phase == phase {
			count++
		}
	}
	return count
}

func (s *recordingSink) voteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, ev := range s.events {
		if ev.Vote != "" {
			count++
		}
	}
	return count
}

// updateBarrierStore makes two independent Store adapters arrive at Update
// before either delegates to its FileStore. This is a scheduling barrier, not
// the serialization mechanism under test; the two FileStores must coordinate.
type updateBarrierStore struct {
	Store
	ready   chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

func (s *updateBarrierStore) Update(ctx context.Context, id string, mutate Mutation) (*Escalation, error) {
	var waitErr error
	s.once.Do(func() {
		select {
		case s.ready <- struct{}{}:
		case <-ctx.Done():
			waitErr = ctx.Err()
			return
		}
		select {
		case <-s.release:
		case <-ctx.Done():
			waitErr = ctx.Err()
		}
	})
	if waitErr != nil {
		return nil, waitErr
	}
	return s.Store.Update(ctx, id, mutate)
}

type engineCallResult struct {
	esc *Escalation
	err error
}

type concurrentEngineHarness struct {
	engines   [2]*Engine
	store     *FileStore
	messenger *fakeMessenger
	human     *fakeHuman
	sink      *recordingSink
	ready     <-chan struct{}
	release   chan struct{}
}

func newConcurrentEngineHarness(t *testing.T, initial *Escalation, parents map[string]ParentRef) *concurrentEngineHarness {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	seed, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore seed: %v", err)
	}
	if err := seed.Create(ctx, initial); err != nil {
		t.Fatalf("Create seed: %v", err)
	}

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	msg := &fakeMessenger{}
	human := &fakeHuman{}
	sink := &recordingSink{}
	var engines [2]*Engine
	for i := range engines {
		fs, ferr := NewFileStore(dir)
		if ferr != nil {
			t.Fatalf("NewFileStore engine %d: %v", i, ferr)
		}
		engines[i], err = NewEngine(Config{
			Graph:     fakeGraph{parents: parents},
			Messenger: msg,
			Human:     human,
			Store: &updateBarrierStore{
				Store: fs, ready: ready, release: release,
			},
			Logger: NewLogger(sink),
			Clock:  func() time.Time { return time.Unix(100, 0) },
		})
		if err != nil {
			t.Fatalf("NewEngine %d: %v", i, err)
		}
	}
	return &concurrentEngineHarness{
		engines: engines, store: seed, messenger: msg, human: human, sink: sink,
		ready: ready, release: release,
	}
}

func (h *concurrentEngineHarness) run(
	t *testing.T,
	first func(*Engine) (*Escalation, error),
	second func(*Engine) (*Escalation, error),
) [2]engineCallResult {
	t.Helper()
	results := make(chan struct {
		index int
		engineCallResult
	}, 2)
	calls := [2]func(*Engine) (*Escalation, error){first, second}
	for i := range h.engines {
		go func(index int) {
			esc, err := calls[index](h.engines[index])
			results <- struct {
				index int
				engineCallResult
			}{index: index, engineCallResult: engineCallResult{esc: esc, err: err}}
		}(i)
	}
	for range 2 {
		select {
		case <-h.ready:
		case <-time.After(5 * time.Second):
			close(h.release)
			t.Fatal("concurrent engine calls did not both reach Store.Update")
		}
	}
	close(h.release)
	var out [2]engineCallResult
	for range 2 {
		result := <-results
		out[result.index] = result.engineCallResult
	}
	return out
}

func requireOneSuccessOneError(t *testing.T, results [2]engineCallResult) {
	t.Helper()
	successes := 0
	errorsSeen := 0
	for _, result := range results {
		if result.err == nil {
			successes++
		} else {
			errorsSeen++
		}
	}
	if successes != 1 || errorsSeen != 1 {
		t.Fatalf("successes=%d errors=%d, want one of each: %+v", successes, errorsSeen, results)
	}
}

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
	if _, err := eng.Forward(ctx, esc.ID, "", "", "note"); err == nil {
		t.Errorf("expected forward without an actor to be refused")
	}
	if _, err := eng.Answer(ctx, esc.ID, "someone-else", "x", "answer"); err == nil {
		t.Errorf("expected answer from non-holder to be refused")
	}
	if _, err := eng.Answer(ctx, esc.ID, "", "", "answer"); err == nil {
		t.Errorf("expected answer without an actor to be refused")
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

func concurrentRoutedState(id, holder string) *Escalation {
	created := time.Unix(1, 0).UTC()
	return &Escalation{
		ID:               id,
		Kind:             KindQuestion,
		Mode:             ModeAsync,
		Question:         "Which route should this use?",
		OriginSessionID:  "worker",
		OriginRole:       "coder",
		CurrentSessionID: holder,
		CurrentRole:      "lead",
		CurrentKind:      NodeSupervisor,
		Chain:            []string{"worker", holder},
		Phase:            PhaseRouted,
		CreatedAt:        created,
		UpdatedAt:        created,
	}
}

func TestEngine_FileStoreConcurrentAnswerHasOneTerminalOwner(t *testing.T) {
	ctx := context.Background()
	initial := concurrentRoutedState("concurrent-answer", "sup")
	h := newConcurrentEngineHarness(t, initial, nil)
	answer := func(eng *Engine) (*Escalation, error) {
		return eng.Answer(ctx, initial.ID, "sup", "lead", "Use the durable route.")
	}
	results := h.run(t, answer, answer)
	requireOneSuccessOneError(t, results)

	got, err := h.store.Get(ctx, initial.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Phase != PhaseAnswered || got.Answer != "Use the durable route." {
		t.Fatalf("state=%+v, want one committed answer", got)
	}
	if h.messenger.resolvedCount() != 1 || h.sink.phaseCount(PhaseAnswered) != 1 {
		t.Fatalf("resolved messages=%d answer events=%d, want 1/1",
			h.messenger.resolvedCount(), h.sink.phaseCount(PhaseAnswered))
	}
}

func TestEngine_FileStoreConcurrentForwardAdvancesOnce(t *testing.T) {
	ctx := context.Background()
	initial := concurrentRoutedState("concurrent-forward", "sup")
	h := newConcurrentEngineHarness(t, initial, map[string]ParentRef{
		"sup": {SessionID: "director", Role: "director", Kind: NodeSupervisor},
	})
	forward := func(eng *Engine) (*Escalation, error) {
		return eng.Forward(ctx, initial.ID, "sup", "lead", "needs director")
	}
	results := h.run(t, forward, forward)
	requireOneSuccessOneError(t, results)

	got, err := h.store.Get(ctx, initial.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Phase != PhaseRouted || got.CurrentSessionID != "director" {
		t.Fatalf("phase=%q holder=%q, want routed/director", got.Phase, got.CurrentSessionID)
	}
	if len(got.Chain) != 3 || got.Chain[2] != "director" {
		t.Fatalf("chain=%v, want one director hop", got.Chain)
	}
	if h.messenger.count() != 1 || h.sink.phaseCount(PhaseRouted) != 1 {
		t.Fatalf("messages=%d routed events=%d, want 1/1", h.messenger.count(), h.sink.phaseCount(PhaseRouted))
	}
}

func TestEngine_FileStoreConcurrentAnswerAndForwardHaveOneOwner(t *testing.T) {
	ctx := context.Background()
	initial := concurrentRoutedState("concurrent-answer-forward", "sup")
	h := newConcurrentEngineHarness(t, initial, map[string]ParentRef{
		"sup": {SessionID: "director", Role: "director", Kind: NodeSupervisor},
	})
	results := h.run(t,
		func(eng *Engine) (*Escalation, error) {
			return eng.Answer(ctx, initial.ID, "sup", "lead", "Use the durable route.")
		},
		func(eng *Engine) (*Escalation, error) {
			return eng.Forward(ctx, initial.ID, "sup", "lead", "needs director")
		},
	)
	requireOneSuccessOneError(t, results)

	got, err := h.store.Get(ctx, initial.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	switch got.Phase {
	case PhaseAnswered:
		if got.Answer != "Use the durable route." || h.messenger.resolvedCount() != 1 ||
			h.sink.phaseCount(PhaseAnswered) != 1 || h.sink.phaseCount(PhaseRouted) != 0 {
			t.Fatalf("answered state/effects=%+v resolved=%d answered_events=%d routed_events=%d",
				got, h.messenger.resolvedCount(), h.sink.phaseCount(PhaseAnswered), h.sink.phaseCount(PhaseRouted))
		}
	case PhaseRouted:
		if got.CurrentSessionID != "director" || got.Answer != "" || h.messenger.resolvedCount() != 0 ||
			h.sink.phaseCount(PhaseAnswered) != 0 || h.sink.phaseCount(PhaseRouted) != 1 {
			t.Fatalf("routed state/effects=%+v resolved=%d answered_events=%d routed_events=%d",
				got, h.messenger.resolvedCount(), h.sink.phaseCount(PhaseAnswered), h.sink.phaseCount(PhaseRouted))
		}
	default:
		t.Fatalf("phase=%q, want exactly one answered or routed transition", got.Phase)
	}
	if h.messenger.count() != 1 {
		t.Fatalf("messages=%d, want exactly one winning transition effect", h.messenger.count())
	}
}

func TestAnswer_StaleHolderAfterForwardIsRejected(t *testing.T) {
	ctx := context.Background()
	initial := concurrentRoutedState("stale-answer-after-forward", "sup")
	h := newConcurrentEngineHarness(t, initial, map[string]ParentRef{
		"sup": {SessionID: "director", Role: "director", Kind: NodeSupervisor},
	})
	// Remove only the test entry barrier so this test deterministically exercises
	// the Forward-first serial order through two independent FileStores.
	for i := range h.engines {
		store, err := NewFileStore(h.store.dir)
		if err != nil {
			t.Fatalf("NewFileStore %d: %v", i, err)
		}
		h.engines[i].cfg.Store = store
	}
	if _, err := h.engines[1].Forward(ctx, initial.ID, "sup", "lead", "needs director"); err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if _, err := h.engines[0].Answer(ctx, initial.ID, "sup", "lead", "stale answer"); err == nil {
		t.Fatal("stale former holder answered after forwarding")
	}
	got, err := h.store.Get(ctx, initial.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Phase != PhaseRouted || got.CurrentSessionID != "director" || got.Answer != "" {
		t.Fatalf("state=%+v, want routed to director without answer", got)
	}
	if h.messenger.count() != 1 || h.sink.phaseCount(PhaseRouted) != 1 || h.sink.phaseCount(PhaseAnswered) != 0 {
		t.Fatalf("messages=%d routed=%d answered=%d, want 1/1/0",
			h.messenger.count(), h.sink.phaseCount(PhaseRouted), h.sink.phaseCount(PhaseAnswered))
	}
}

func TestForward_BlockedRouteLookupDoesNotHoldStoreLock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, esc := range []*Escalation{
		concurrentRoutedState("blocked-route", "sup-a"),
		concurrentRoutedState("independent-answer", "sup-b"),
	} {
		if err := seed.Create(ctx, esc); err != nil {
			t.Fatalf("Create %s: %v", esc.ID, err)
		}
	}
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	graph := &blockingGraph{
		entered: make(chan struct{}), release: release,
		parent: ParentRef{SessionID: "director", Role: "director", Kind: NodeSupervisor},
	}
	msg := &fakeMessenger{}
	human := &fakeHuman{}
	sink := &recordingSink{}
	newEngine := func(store Store, sessionGraph SessionGraph) *Engine {
		eng, newErr := NewEngine(Config{
			Graph: sessionGraph, Messenger: msg, Human: human, Store: store, Logger: NewLogger(sink),
		})
		if newErr != nil {
			t.Fatalf("NewEngine: %v", newErr)
		}
		return eng
	}
	storeA, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore A: %v", err)
	}
	storeB, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore B: %v", err)
	}
	engA := newEngine(storeA, graph)
	engB := newEngine(storeB, fakeGraph{})

	forwardDone := make(chan error, 1)
	go func() {
		_, forwardErr := engA.Forward(ctx, "blocked-route", "sup-a", "lead", "needs director")
		forwardDone <- forwardErr
	}()
	select {
	case <-graph.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Forward did not enter hierarchy lookup")
	}

	answerDone := make(chan error, 1)
	go func() {
		_, answerErr := engB.Answer(ctx, "independent-answer", "sup-b", "lead", "independent")
		answerDone <- answerErr
	}()
	select {
	case answerErr := <-answerDone:
		if answerErr != nil {
			t.Fatalf("independent Answer: %v", answerErr)
		}
	case <-time.After(time.Second):
		t.Fatal("independent Answer was blocked by another escalation's hierarchy lookup")
	}
	close(release)
	released = true
	if err := <-forwardDone; err != nil {
		t.Fatalf("Forward: %v", err)
	}
}

func TestRaise_PostCommitEffectsOutliveRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &cancelAfterCreateStore{Store: NewMemStore(), cancel: cancel}
	msg := &contextCheckingMessenger{}
	human := &fakeHuman{}
	sink := &contextCheckingSink{}
	eng, err := NewEngine(Config{
		Graph: fakeGraph{parents: map[string]ParentRef{
			"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
		}},
		Messenger: msg, Human: human, Store: store, Logger: NewLogger(sink),
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	esc, err := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Which logging library should this repository use?",
	})
	if err != nil {
		t.Fatalf("Raise after committed cancellation: %v", err)
	}
	if ctx.Err() == nil || esc.Phase != PhaseRouted || msg.count() != 1 ||
		sink.phaseCount(PhaseRaised) != 1 || sink.phaseCount(PhaseRouted) != 1 {
		t.Fatalf("ctx=%v state=%+v messages=%d raised=%d routed=%d",
			ctx.Err(), esc, msg.count(), sink.phaseCount(PhaseRaised), sink.phaseCount(PhaseRouted))
	}
}

func TestRaise_EachPostCommitEffectGetsFreshDeadline(t *testing.T) {
	sink := &contextCheckingSink{}
	eng, err := NewEngine(Config{
		Graph: fakeGraph{parents: map[string]ParentRef{
			"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
		}},
		Messenger: deadlineMessenger{}, Human: &fakeHuman{}, Store: NewMemStore(), Logger: NewLogger(sink),
		PostCommitEffectTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	esc, err := eng.Raise(context.Background(), RaiseRequest{
		OriginSessionID: "worker", Kind: KindQuestion, Mode: ModeAsync,
		Question: "Which logging library should this repository use?",
	})
	var committedErr *CommittedEffectError
	if esc == nil || !errors.As(err, &committedErr) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("esc=%+v err=%v, want committed state with delivery deadline", esc, err)
	}
	if sink.phaseCount(PhaseRaised) != 1 || sink.phaseCount(PhaseRouted) != 1 {
		t.Fatalf("raised=%d routed=%d, want 1/1 despite prior effect timeout",
			sink.phaseCount(PhaseRaised), sink.phaseCount(PhaseRouted))
	}
}

func TestRaise_RequiredDeliveryFailureReturnsCommittedState(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	msg := &fakeMessenger{err: errors.New("delivery unavailable")}
	human := &fakeHuman{}
	sink := &recordingSink{}
	eng, err := NewEngine(Config{
		Graph: fakeGraph{parents: map[string]ParentRef{
			"worker": {SessionID: "sup", Role: "lead", Kind: NodeSupervisor},
		}},
		Messenger: msg,
		Human:     human,
		Store:     store,
		Logger:    NewLogger(sink),
		Clock:     func() time.Time { return time.Unix(100, 0) },
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	esc, err := eng.Raise(ctx, RaiseRequest{
		OriginSessionID: "worker",
		OriginRole:      "coder",
		Kind:            KindQuestion,
		Mode:            ModeAsync,
		Question:        "Which logging library does this repository use?",
	})
	var committedErr *CommittedEffectError
	if esc == nil || !errors.As(err, &committedErr) || committedErr.Phase != PhaseRouted ||
		!strings.Contains(err.Error(), "committed phase routed") {
		t.Fatalf("Raise esc=%+v err=%v, want committed routed state plus explicit error", esc, err)
	}
	stored, getErr := store.Get(ctx, esc.ID)
	if getErr != nil {
		t.Fatalf("Get committed escalation: %v", getErr)
	}
	if stored.Phase != PhaseRouted || stored.CurrentSessionID != "sup" {
		t.Fatalf("stored state=%+v, want routed to sup", stored)
	}
	if sink.phaseCount(PhaseRaised) != 1 || sink.phaseCount(PhaseRouted) != 1 {
		t.Fatalf("raised events=%d routed events=%d, want 1/1 after failed required delivery",
			sink.phaseCount(PhaseRaised), sink.phaseCount(PhaseRouted))
	}
}

func TestForward_HumanDispatchFailureReturnsCommittedState(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	initial := concurrentRoutedState("human-effect-failure", "orch")
	initial.CurrentRole = "orchestrator"
	initial.CurrentKind = NodeVROOMOrchestrator
	initial.Phase = PhaseConferring
	if err := store.Create(ctx, initial); err != nil {
		t.Fatalf("Create: %v", err)
	}
	human := &fakeHuman{err: errors.New("dispatch trail unavailable")}
	sink := &recordingSink{}
	eng, err := NewEngine(Config{
		Graph:      fakeGraph{},
		Messenger:  &fakeMessenger{},
		Human:      human,
		Store:      store,
		Logger:     NewLogger(sink),
		Clock:      func() time.Time { return time.Unix(100, 0) },
		VROOMEntry: nil,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	esc, err := eng.Forward(ctx, initial.ID, "orch", "orchestrator", "needs human")
	var committedErr *CommittedEffectError
	if esc == nil || !errors.As(err, &committedErr) || committedErr.Phase != PhaseDispatchedToHuman ||
		!strings.Contains(err.Error(), "committed phase dispatched-to-human") {
		t.Fatalf("Forward esc=%+v err=%v, want committed human state plus explicit error", esc, err)
	}
	stored, getErr := store.Get(ctx, initial.ID)
	if getErr != nil {
		t.Fatalf("Get committed escalation: %v", getErr)
	}
	if stored.Phase != PhaseDispatchedToHuman || stored.CurrentSessionID != HumanSessionID {
		t.Fatalf("stored state=%+v, want dispatched-to-human", stored)
	}
	if human.count() != 1 || sink.phaseCount(PhaseDispatchedToHuman) != 1 {
		t.Fatalf("human attempts=%d dispatch events=%d, want 1/1", human.count(), sink.phaseCount(PhaseDispatchedToHuman))
	}
}

func TestAnswer_DeliveryFailureRemainsBestEffort(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	initial := concurrentRoutedState("answer-best-effort", "sup")
	if err := store.Create(ctx, initial); err != nil {
		t.Fatalf("Create: %v", err)
	}
	msg := &fakeMessenger{err: errors.New("worker unavailable")}
	sink := &recordingSink{}
	eng, err := NewEngine(Config{
		Graph:     fakeGraph{},
		Messenger: msg,
		Human:     &fakeHuman{},
		Store:     store,
		Logger:    NewLogger(sink),
		Clock:     func() time.Time { return time.Unix(100, 0) },
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	esc, err := eng.Answer(ctx, initial.ID, "sup", "lead", "Use A")
	if err != nil {
		t.Fatalf("Answer returned best-effort delivery error: %v", err)
	}
	if esc.Phase != PhaseAnswered || msg.count() != 1 || sink.phaseCount(PhaseAnswered) != 1 {
		t.Fatalf("state=%+v messages=%d events=%d, want answered/1/1",
			esc, msg.count(), sink.phaseCount(PhaseAnswered))
	}
}
