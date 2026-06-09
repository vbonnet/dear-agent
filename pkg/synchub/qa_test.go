package synchub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/a2a"
)

func newTestHub(t *testing.T, opts Options) *Hub {
	t.Helper()
	if opts.SessionID == "" {
		opts.SessionID = "test-session"
	}
	h, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// TestQA_FirstWriterWins is the headline guarantee: when two surfaces
// race to answer the same round, exactly one wins and the other gets
// ClosedError carrying the winner.
func TestQA_FirstWriterWins(t *testing.T) {
	h := newTestHub(t, Options{Bus: a2a.NewInMemoryBus()})
	ctx := context.Background()

	qid, err := h.AskQuestion(ctx, "color?")
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}

	const N = 50
	var wins int32
	var losses int32
	var loserErr error
	var loserMu sync.Mutex
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.Answer(ctx, qid, SurfaceTerminal, []byte{byte(i)})
			if err == nil {
				atomic.AddInt32(&wins, 1)
				return
			}
			atomic.AddInt32(&losses, 1)
			loserMu.Lock()
			if loserErr == nil {
				loserErr = err
			}
			loserMu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("wins=%d, want 1", wins)
	}
	if losses != N-1 {
		t.Fatalf("losses=%d, want %d", losses, N-1)
	}
	var ce *ClosedError
	if !errors.As(loserErr, &ce) {
		t.Fatalf("expected *ClosedError, got %T: %v", loserErr, loserErr)
	}
	if !errors.Is(loserErr, ErrClosed) {
		t.Fatalf("ClosedError should satisfy errors.Is(ErrClosed)")
	}
}

// TestQA_DoubleAnswerRejected: a single surface answering twice gets
// rejected on the second attempt.
func TestQA_DoubleAnswerRejected(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	qid, _ := h.AskQuestion(ctx, "?")
	if _, err := h.Answer(ctx, qid, SurfaceDesktop, []byte("first")); err != nil {
		t.Fatalf("first answer: %v", err)
	}
	_, err := h.Answer(ctx, qid, SurfaceDesktop, []byte("second"))
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("second answer err=%v, want ErrClosed", err)
	}
}

// TestQA_NewQuestionNewRound: the rule that follow-up questions are
// distinct rounds. Tests two consecutive AskQuestion calls produce
// distinct IDs and both accept answers independently.
func TestQA_NewQuestionNewRound(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	q1, _ := h.AskQuestion(ctx, "first")
	q2, _ := h.AskQuestion(ctx, "second")
	if q1 == q2 {
		t.Fatal("consecutive questions got same ID")
	}
	if _, err := h.Answer(ctx, q1, SurfaceTerminal, []byte("A")); err != nil {
		t.Fatalf("answer q1: %v", err)
	}
	// q2 must still accept its own answer.
	if _, err := h.Answer(ctx, q2, SurfaceDiscord, []byte("B")); err != nil {
		t.Fatalf("answer q2: %v", err)
	}
}

// TestQA_AnswerForUnknownRound returns ErrNotFound, not ErrClosed.
func TestQA_AnswerForUnknownRound(t *testing.T) {
	h := newTestHub(t, Options{})
	_, err := h.Answer(context.Background(), QuestionID("q-unknown"), SurfaceTerminal, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v, want ErrNotFound", err)
	}
}

// TestQA_Expiry: a round past TTL rejects new answers as expired, and
// the lazy-expiry path inside Answer triggers without waiting for the
// sweeper.
func TestQA_Expiry(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{t: now}
	h := newTestHub(t, Options{
		RoundTTL: time.Second,
		now:      clock.Now,
	})
	ctx := context.Background()
	qid, _ := h.AskQuestion(ctx, "?")

	clock.Advance(2 * time.Second)

	_, err := h.Answer(ctx, qid, SurfaceTerminal, []byte("late"))
	var ee *ExpiredError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *ExpiredError, got %T: %v", err, err)
	}
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("ExpiredError should satisfy errors.Is(ErrExpired)")
	}
}

// TestQA_Cancel: idempotent, returns nil for repeats.
func TestQA_Cancel(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	qid, _ := h.AskQuestion(ctx, "?")
	if err := h.Cancel(ctx, qid); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := h.Cancel(ctx, qid); err != nil {
		t.Fatalf("second cancel should be nil, got %v", err)
	}
	_, err := h.Answer(ctx, qid, SurfaceTerminal, nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("answer after cancel: err=%v, want ErrClosed", err)
	}
}

// TestQA_IDsAreUnguessable verifies the suffix has enough entropy that
// two consecutive IDs cannot be predicted. A weak check — we just look
// at suffix uniqueness across many calls — but enough to catch a
// regression where someone replaces crypto/rand with math/rand.
func TestQA_IDsAreUnguessable(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	seen := make(map[QuestionID]bool)
	for i := 0; i < 1000; i++ {
		id, _ := h.AskQuestion(ctx, "?")
		if seen[id] {
			t.Fatalf("duplicate ID after %d: %s", i, id)
		}
		seen[id] = true
	}
}

// TestQA_Inspect returns the round's last-known state.
func TestQA_Inspect(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	qid, _ := h.AskQuestion(ctx, "favorite")
	snap, err := h.Inspect(qid)
	if err != nil {
		t.Fatalf("Inspect open: %v", err)
	}
	if snap.State != "open" {
		t.Fatalf("state=%s want open", snap.State)
	}
	if _, err := h.Answer(ctx, qid, SurfaceTerminal, []byte("blue")); err != nil {
		t.Fatal(err)
	}
	snap, _ = h.Inspect(qid)
	if snap.State != "answered" || snap.Winner != SurfaceTerminal {
		t.Fatalf("after answer: %+v", snap)
	}
}

// TestQA_SweeperGCsTombstones: after TombstoneTTL elapses, the round is
// purged and a late answer gets ErrNotFound rather than ErrClosed.
func TestQA_SweeperGCsTombstones(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{t: now}
	h := newTestHub(t, Options{
		RoundTTL:     500 * time.Millisecond,
		TombstoneTTL: 500 * time.Millisecond,
		now:          clock.Now,
	})
	ctx := context.Background()
	qid, _ := h.AskQuestion(ctx, "?")
	_, _ = h.Answer(ctx, qid, SurfaceTerminal, []byte("done"))

	// Advance past tombstone window, then run sweep directly.
	clock.Advance(2 * time.Second)
	h.sweep()

	_, err := h.Inspect(qid)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("after tombstone GC: err=%v, want ErrNotFound", err)
	}
}

// TestQA_OpensQAClosesEvents: surfaces subscribed via A2A see opened +
// answered topics in order.
func TestQA_OpensQAClosesEvents(t *testing.T) {
	bus := a2a.NewInMemoryBus()
	h := newTestHub(t, Options{Bus: bus, SessionID: "evt"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	openCh, _ := bus.Subscribe(ctx, "synchub.evt.qa.opened")
	ansCh, _ := bus.Subscribe(ctx, "synchub.evt.qa.answered")

	qid, _ := h.AskQuestion(ctx, "?")
	if _, err := h.Answer(ctx, qid, SurfaceDiscord, []byte("ok")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-openCh:
	case <-time.After(time.Second):
		t.Fatal("no opened event")
	}
	select {
	case <-ansCh:
	case <-time.After(time.Second):
		t.Fatal("no answered event")
	}
}

// fakeClock is used by tests that need to drive time forward
// deterministically. We accept it loses monotonic-clock semantics in
// the test path; production uses time.Now which preserves them.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
