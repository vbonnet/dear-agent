package synchub

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"time"
)

// QuestionID is the unique identifier of a Q&A round. It is the only key
// surfaces use when submitting an answer — text is never matched, so a
// follow-up question with identical wording is a brand-new round.
type QuestionID string

// roundState is the state machine for a Q&A round. Linear: Open ->
// (Answered | Expired | Canceled). Once left, never re-entered.
type roundState int

const (
	roundOpen roundState = iota
	roundAnswered
	roundExpired
	roundCanceled
)

func (s roundState) String() string {
	switch s {
	case roundOpen:
		return "open"
	case roundAnswered:
		return "answered"
	case roundExpired:
		return "expired"
	case roundCanceled:
		return "canceled"
	}
	return "unknown"
}

type round struct {
	id        QuestionID
	prompt    string
	openedAt  time.Time
	ttl       time.Duration // per-round TTL; 0 means use Hub.RoundTTL
	state     roundState
	winner    Surface
	answer    []byte
	closedAt  time.Time
	closeNote string // why it closed (for diagnostics)
}

// effectiveTTL returns the round's TTL or the hub's default.
func (r *round) effectiveTTL(hubDefault time.Duration) time.Duration {
	if r.ttl > 0 {
		return r.ttl
	}
	return hubDefault
}

// Answer is what AwaitAnswer returns: who won, what they sent, and when.
type Answer struct {
	QuestionID QuestionID
	Winner     Surface
	Payload    []byte
	At         time.Time
}

// ClosedError is returned by Answer() when the round is already closed.
// It carries the winner so the caller's surface can show "X already
// answered" instead of a generic failure.
type ClosedError struct {
	QuestionID QuestionID
	Winner     Surface
	At         time.Time
}

func (e *ClosedError) Error() string {
	return fmt.Sprintf("synchub: round %s already answered by %s at %s",
		e.QuestionID, e.Winner, e.At.Format(time.RFC3339Nano))
}

func (e *ClosedError) Is(target error) bool { return target == ErrClosed }

// ExpiredError is returned when a round timed out before any answer landed.
type ExpiredError struct {
	QuestionID QuestionID
	At         time.Time
}

func (e *ExpiredError) Error() string {
	return fmt.Sprintf("synchub: round %s expired at %s",
		e.QuestionID, e.At.Format(time.RFC3339Nano))
}

func (e *ExpiredError) Is(target error) bool { return target == ErrExpired }

// AskOption tunes a single AskQuestion call. Variadic so the call site
// stays clean for the common "use defaults" case (F-U8 from the
// multi-persona review).
type AskOption func(*askConfig)

type askConfig struct {
	ttl time.Duration // 0 = use hub default
}

// WithTTL overrides RoundTTL for this round only. Useful for
// agent-asked "Deploy?" (want minutes) vs "Continue?" (want seconds).
func WithTTL(d time.Duration) AskOption { return func(c *askConfig) { c.ttl = d } }

// AskQuestion opens a new Q&A round and returns its ID. The caller is
// the agent (terminal-side), not a surface. Two consecutive calls produce
// two distinct IDs — surfaces that have not yet noticed the first
// question is closed will be safely rejected when they try to answer it.
func (h *Hub) AskQuestion(ctx context.Context, prompt string, opts ...AskOption) (QuestionID, error) {
	var cfg askConfig
	for _, o := range opts {
		o(&cfg)
	}
	ttl := cfg.ttl
	if ttl == 0 {
		ttl = h.opts.RoundTTL
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return "", ErrLockClosed
	}
	h.qaSeq++
	seq := h.qaSeq
	id := newQuestionID(h.opts.SessionID, seq)
	r := &round{
		id:       id,
		prompt:   prompt,
		openedAt: h.opts.now(),
		ttl:      ttl,
		state:    roundOpen,
	}
	h.rounds[id] = r
	h.mu.Unlock()

	h.publish(ctx, topicQAOpened(h.opts.SessionID), mustJSON(map[string]any{
		"id":     id,
		"prompt": prompt,
		"seq":    seq,
		"ttl_ms": ttl.Milliseconds(),
	}))
	return id, nil
}

// Answer submits an answer for a round. The first caller to acquire the
// hub mutex with the round still Open wins; everyone else gets
// *ClosedError (which satisfies errors.Is(err, ErrClosed)) or
// *ExpiredError. The round's mutex IS the hub's mutex — Q&A operations
// are O(1) under the lock, so a global mutex is simpler and faster than
// per-round locking, and the arbitration becomes trivially deterministic.
func (h *Hub) Answer(ctx context.Context, qid QuestionID, surface Surface, payload []byte) (Answer, error) {
	h.mu.Lock()
	r, ok := h.rounds[qid]
	if !ok {
		h.mu.Unlock()
		return Answer{}, ErrNotFound
	}
	switch r.state {
	case roundAnswered:
		err := &ClosedError{QuestionID: qid, Winner: r.winner, At: r.closedAt}
		h.mu.Unlock()
		return Answer{}, err
	case roundExpired:
		err := &ExpiredError{QuestionID: qid, At: r.closedAt}
		h.mu.Unlock()
		return Answer{}, err
	case roundCanceled:
		h.mu.Unlock()
		return Answer{}, ErrClosed
	}

	// Lazy expiry check: if we've passed the (per-round or hub-default)
	// TTL, close the round now rather than accept a stale answer.
	// Uses h.opts.now() (set to time.Now in production, injectable in
	// tests). The Time values returned by time.Now carry monotonic
	// readings, so .Sub is monotonic-safe in production.
	if h.opts.now().Sub(r.openedAt) > r.effectiveTTL(h.opts.RoundTTL) {
		r.state = roundExpired
		r.closedAt = h.opts.now()
		r.closeNote = "lazy-expiry"
		err := &ExpiredError{QuestionID: qid, At: r.closedAt}
		h.mu.Unlock()
		h.publish(ctx, topicQAExpired(h.opts.SessionID), mustJSON(map[string]any{
			"id": qid,
		}))
		return Answer{}, err
	}

	r.state = roundAnswered
	r.winner = surface
	r.answer = payload
	r.closedAt = h.opts.now()
	ans := Answer{
		QuestionID: qid,
		Winner:     surface,
		Payload:    append([]byte(nil), payload...),
		At:         r.closedAt,
	}
	h.mu.Unlock()

	h.publish(ctx, topicQAAnswered(h.opts.SessionID), mustJSON(map[string]any{
		"id":      qid,
		"winner":  surface,
		"payload": payload,
	}))
	return ans, nil
}

// Cancel closes a round without an answer. Used when the agent times out
// internally or the user aborts via /clear etc. Idempotent.
func (h *Hub) Cancel(ctx context.Context, qid QuestionID) error {
	h.mu.Lock()
	r, ok := h.rounds[qid]
	if !ok {
		h.mu.Unlock()
		return ErrNotFound
	}
	if r.state != roundOpen {
		h.mu.Unlock()
		return nil
	}
	r.state = roundCanceled
	r.closedAt = h.opts.now()
	r.closeNote = "canceled"
	h.mu.Unlock()
	h.publish(ctx, topicQAExpired(h.opts.SessionID), mustJSON(map[string]any{
		"id":     qid,
		"reason": "canceled",
	}))
	return nil
}

// Inspect returns a copy of a round's state, for diagnostics and tests.
// Returns ErrNotFound if the round (including its tombstone window) is
// gone.
func (h *Hub) Inspect(qid QuestionID) (RoundSnapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rounds[qid]
	if !ok {
		return RoundSnapshot{}, ErrNotFound
	}
	return RoundSnapshot{
		ID:       r.id,
		Prompt:   r.prompt,
		OpenedAt: r.openedAt,
		State:    r.state.String(),
		Winner:   r.winner,
		ClosedAt: r.closedAt,
	}, nil
}

// RoundSnapshot is the read-only view returned by Inspect.
type RoundSnapshot struct {
	ID       QuestionID
	Prompt   string
	OpenedAt time.Time
	State    string
	Winner   Surface
	ClosedAt time.Time
}

// sweepRounds expires open rounds past TTL and drops tombstones older
// than TombstoneTTL. Collects expirations under the lock, publishes
// after releasing — F-Sc3 from the multi-persona review: holding the
// hub mutex while talking to the bus stalls concurrent Q&A work if the
// bus is slow.
func (h *Hub) sweepRounds(now time.Time) {
	var expired []QuestionID
	h.mu.Lock()
	for id, r := range h.rounds {
		switch r.state {
		case roundOpen:
			if now.Sub(r.openedAt) > r.effectiveTTL(h.opts.RoundTTL) {
				r.state = roundExpired
				r.closedAt = now
				r.closeNote = "sweeper-expiry"
				expired = append(expired, id)
			}
		case roundAnswered, roundExpired, roundCanceled:
			if !r.closedAt.IsZero() && now.Sub(r.closedAt) > h.opts.TombstoneTTL {
				delete(h.rounds, id)
			}
		}
	}
	h.mu.Unlock()

	for _, id := range expired {
		h.publish(context.Background(), topicQAExpired(h.opts.SessionID), mustJSON(map[string]any{
			"id": id,
		}))
	}
}

// newQuestionID builds an unguessable, sortable-ish round ID.
// Format: q-<session>-<seq>-<rand6>. crypto/rand for the suffix means a
// hostile process on the same Tailnet cannot enumerate round IDs.
func newQuestionID(session string, seq uint64) QuestionID {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand on darwin/linux essentially cannot fail; if it does
		// we'd rather panic than emit a guessable ID.
		panic(fmt.Sprintf("synchub: crypto/rand failed: %v", err))
	}
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return QuestionID(fmt.Sprintf("q-%s-%d-%s", session, seq, suffix))
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// All the values we marshal are plain maps with string keys and
		// JSON-native values; a marshal error is a bug, not a runtime
		// condition. Better to crash loudly in tests.
		panic(fmt.Sprintf("synchub: json marshal: %v", err))
	}
	return b
}
