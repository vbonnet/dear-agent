package escalation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HumanSessionID is the sentinel "session" used as the answerer when the human
// resolves an escalation. A node whose ID equals this is treated as the human,
// the only authority permitted to answer a must-reach-human escalation.
const HumanSessionID = "human"

// ErrAwaitTimeout is returned by Await when the escalation did not resolve in
// time. The escalation remains in-flight; only the worker's wait ended.
var ErrAwaitTimeout = errors.New("escalation: await timed out")

// SessionGraph answers "who should this session escalate to" — the spawn
// hierarchy, supplied by AGM's Dolt session adapter. A ParentRef with Kind
// NodeNone (and nil error) means the session is a root with no parent.
type SessionGraph interface {
	Parent(ctx context.Context, sessionID string) (ParentRef, error)
}

// EscalationMessage is what a Messenger delivers to a session: enough for the
// receiving agent to understand the ask and act (answer / forward).
type EscalationMessage struct {
	EscalationID   string
	Kind           Kind
	Mode           Mode
	Question       string
	Context        string
	FromSessionID  string
	FromRole       string
	Note           string // forwarder's note or resolution answer
	MustReachHuman bool
	Resolved       bool // true ⇒ this is the answer being returned to the origin
	Answer         string
}

// Messenger delivers an EscalationMessage to a session (the next hop up the
// chain, or the originator when an answer comes back). Backed by `agm send`.
type Messenger interface {
	Deliver(ctx context.Context, toSessionID string, msg EscalationMessage) error
}

// HumanDispatch surfaces an escalation to the human (the final hop), backed by
// `agm ask` + pkg/notify. It does not block; the human answers later via
// Engine.Answer with HumanSessionID.
type HumanDispatch interface {
	ToHuman(ctx context.Context, e *Escalation) error
}

// Config wires an Engine's collaborators. Graph, Messenger, Human, Store and
// Logger are required; Classifier defaults to DefaultClassifier and Clock to
// time.Now.
type Config struct {
	Graph      SessionGraph
	Classifier Classifier
	Messenger  Messenger
	Human      HumanDispatch
	Store      Store
	Logger     *Logger
	Clock      func() time.Time
	// MaxHops bounds chain length; once exceeded the escalation is dispatched to
	// the human rather than routed further. Default 16.
	MaxHops int
	// VROOMEntry, if set, is where an orphaned chain (a session with no parent
	// that is not itself a VROOM node) is routed so it still reaches the mesh.
	// If nil, orphaned chains go straight to the human.
	VROOMEntry *ParentRef
	// Registry, if set, enables the programmatic VROOM-trio confer (ce-es7z):
	// when an escalation reaches the mesh it is delivered to every live trio
	// member and resolved by quorum vote. If nil, reaching a VROOM node falls
	// back to single-node conferring (the node confers via peer messaging).
	Registry VROOMRegistry
	// ConferQuorum overrides the votes needed to resolve a confer. Default (0 or
	// out of range): a strict majority of the live members (2 of 3).
	ConferQuorum int
}

// Engine routes and records escalations. It is safe for concurrent use to the
// extent its Store and Sink are.
type Engine struct {
	cfg Config
}

// NewEngine validates cfg and returns an Engine.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.Graph == nil {
		return nil, fmt.Errorf("escalation: Config.Graph is required")
	}
	if cfg.Messenger == nil {
		return nil, fmt.Errorf("escalation: Config.Messenger is required")
	}
	if cfg.Human == nil {
		return nil, fmt.Errorf("escalation: Config.Human is required")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("escalation: Config.Store is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("escalation: Config.Logger is required")
	}
	if cfg.Classifier == nil {
		cfg.Classifier = DefaultClassifier{}
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.MaxHops <= 0 {
		cfg.MaxHops = 16
	}
	return &Engine{cfg: cfg}, nil
}

// RaiseRequest is the input a worker supplies to raise an escalation.
type RaiseRequest struct {
	OriginSessionID string
	OriginRole      string
	Kind            Kind
	Mode            Mode
	Question        string
	Context         string
}

func (r RaiseRequest) validate() error {
	if strings.TrimSpace(r.OriginSessionID) == "" {
		return fmt.Errorf("escalation: OriginSessionID is required")
	}
	if strings.TrimSpace(r.Question) == "" {
		return fmt.Errorf("escalation: Question is required")
	}
	if !r.Kind.Valid() {
		return fmt.Errorf("escalation: invalid kind %q (want question|decision|blocked-action)", r.Kind)
	}
	if !r.Mode.Valid() {
		return fmt.Errorf("escalation: invalid mode %q (want blocking|async)", r.Mode)
	}
	return nil
}

// Raise creates an escalation, classifies it, and either auto-resolves it (for
// self-evident questions) or routes it to the origin's supervisor. It returns
// the escalation in its post-routing state. For blocking mode the caller then
// invokes Await.
func (e *Engine) Raise(ctx context.Context, req RaiseRequest) (*Escalation, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	now := e.now()
	esc := &Escalation{
		ID:               uuid.New().String(),
		Kind:             req.Kind,
		Mode:             req.Mode,
		Question:         req.Question,
		Context:          req.Context,
		OriginSessionID:  req.OriginSessionID,
		OriginRole:       req.OriginRole,
		CurrentSessionID: req.OriginSessionID,
		CurrentRole:      req.OriginRole,
		CurrentKind:      NodeWorker,
		Chain:            []string{req.OriginSessionID},
		Phase:            PhaseRaised,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	verdict, err := e.cfg.Classifier.Classify(ctx, esc)
	if err != nil {
		return nil, fmt.Errorf("escalation: classify: %w", err)
	}
	esc.Disposition = verdict.Disposition
	esc.MustReachHuman = verdict.MustReachHuman
	esc.ClassifierReason = verdict.Reason
	esc.Topic = verdict.Topic

	e.emit(ctx, esc, PhaseRaised, eventFields{from: esc.OriginSessionID, fromRole: esc.OriginRole})

	if verdict.Disposition == DispAutoResolve {
		esc.Answer = verdict.Answer
		esc.AnsweredBy = "auto"
		esc.AnsweredByRole = "classifier"
		esc.Phase = PhaseAutoResolved
		esc.ResolvedAt = now
		esc.UpdatedAt = now
		e.emit(ctx, esc, PhaseAutoResolved, eventFields{
			from: "auto", answer: verdict.Answer, latency: 0,
		})
		if err := e.cfg.Store.Put(ctx, esc); err != nil {
			return nil, err
		}
		return esc, nil
	}

	// Route the first hop: from the origin to its supervisor.
	if err := e.advance(ctx, esc); err != nil {
		return nil, err
	}
	if err := e.cfg.Store.Put(ctx, esc); err != nil {
		return nil, err
	}
	return esc, nil
}

// advance moves the escalation one hop up from its current holder, delivering it
// to the next node (or to the human when the chain ends or loops). It mutates
// esc and emits the transition event but does not persist (callers Put).
func (e *Engine) advance(ctx context.Context, esc *Escalation) error {
	from := esc.CurrentSessionID
	fromRole := esc.CurrentRole

	if len(esc.Chain) > e.cfg.MaxHops {
		return e.dispatchToHuman(ctx, esc, from, fromRole, "hop limit reached")
	}

	pref, err := e.cfg.Graph.Parent(ctx, from)
	if err != nil {
		return fmt.Errorf("escalation: lookup parent of %q: %w", from, err)
	}

	// No parent, or a cycle: fall back to the VROOM entry if configured, else
	// straight to the human.
	if pref.Kind == NodeNone || pref.SessionID == "" || inChain(esc.Chain, pref.SessionID) {
		if e.cfg.VROOMEntry != nil && !esc.CurrentKind.IsVROOM() && !inChain(esc.Chain, e.cfg.VROOMEntry.SessionID) {
			pref = *e.cfg.VROOMEntry
		} else {
			return e.dispatchToHuman(ctx, esc, from, fromRole, "no further supervisor in the chain")
		}
	}

	// Reaching the VROOM mesh runs a programmatic confer across the live trio
	// when a registry is configured; otherwise it falls back to delivering to the
	// single node, which confers via peer messaging (pre-ce-es7z behaviour).
	if pref.Kind.IsVROOM() {
		if e.cfg.Registry != nil {
			return e.beginConfer(ctx, esc, pref, from, fromRole)
		}
		return e.routeSingleVROOM(ctx, esc, pref, from, fromRole)
	}

	esc.Chain = append(esc.Chain, pref.SessionID)
	esc.CurrentSessionID = pref.SessionID
	esc.CurrentRole = pref.Role
	esc.CurrentKind = pref.Kind
	esc.UpdatedAt = e.now()
	esc.Phase = PhaseRouted

	if err := e.cfg.Messenger.Deliver(ctx, pref.SessionID, e.message(esc, from, fromRole, "")); err != nil {
		return fmt.Errorf("escalation: deliver to %q: %w", pref.SessionID, err)
	}
	e.emit(ctx, esc, PhaseRouted, eventFields{
		from: from, fromRole: fromRole, to: pref.SessionID, toRole: pref.Role,
	})
	return nil
}

// dispatchToHuman surfaces the escalation to the human. It is not a terminal
// phase: the human answers later via Answer(HumanSessionID, ...).
func (e *Engine) dispatchToHuman(ctx context.Context, esc *Escalation, from, fromRole, reason string) error {
	esc.CurrentSessionID = HumanSessionID
	esc.CurrentRole = "human"
	esc.CurrentKind = NodeHuman
	esc.Phase = PhaseDispatchedToHuman
	esc.UpdatedAt = e.now()
	if err := e.cfg.Human.ToHuman(ctx, esc); err != nil {
		return fmt.Errorf("escalation: dispatch to human: %w", err)
	}
	e.emit(ctx, esc, PhaseDispatchedToHuman, eventFields{
		from: from, fromRole: fromRole, to: HumanSessionID, toRole: "human", note: reason,
	})
	return nil
}

// Answer records a terminal answer and returns it to the origin session. It is
// refused if the escalation must reach the human and the answerer is not the
// human — a supervisor faced with a product decision must Forward (adding a
// recommendation), not decide it.
func (e *Engine) Answer(ctx context.Context, id, bySessionID, byRole, answer string) (*Escalation, error) {
	if strings.TrimSpace(answer) == "" {
		return nil, fmt.Errorf("escalation: answer text is required")
	}
	esc, err := e.cfg.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if esc.resolved() {
		return nil, fmt.Errorf("escalation %s is already %s", id, esc.Phase)
	}
	// During a programmatic confer, a trio member must vote rather than answer
	// unilaterally — the quorum, not a single supervisor, decides. The human may
	// still answer directly (an override of the in-flight confer).
	if esc.Confer != nil && esc.Phase == PhaseConferring &&
		bySessionID != HumanSessionID && esc.Confer.isMember(bySessionID) {
		return nil, fmt.Errorf(
			"escalation %s is in a VROOM trio confer — cast a vote (`escalate vote %s approve|reject`), "+
				"do not answer unilaterally", id, id)
	}
	if esc.MustReachHuman && bySessionID != HumanSessionID {
		return nil, fmt.Errorf(
			"escalation %s must reach the human (%s): a node below the human may not answer it.\n"+
				"The right way: `escalate forward` with your recommendation so the human decides.\n"+
				"Because: %s", id, esc.Topic, esc.ClassifierReason)
	}

	now := e.now()
	esc.Answer = answer
	esc.AnsweredBy = bySessionID
	esc.AnsweredByRole = byRole
	esc.Phase = PhaseAnswered
	esc.ResolvedAt = now
	esc.UpdatedAt = now

	// Return the answer to the worker that raised it (async delivery; harmless
	// for blocking mode, whose Await reads the Store).
	if esc.OriginSessionID != "" && esc.OriginSessionID != bySessionID {
		_ = e.cfg.Messenger.Deliver(ctx, esc.OriginSessionID, e.message(esc, bySessionID, byRole, answer))
	}

	e.emit(ctx, esc, PhaseAnswered, eventFields{
		from: bySessionID, fromRole: byRole, answer: answer,
		latency: now.Sub(esc.CreatedAt).Milliseconds(),
	})
	if err := e.cfg.Store.Put(ctx, esc); err != nil {
		return nil, err
	}
	return esc, nil
}

// Forward escalates one more hop up the chain. From a VROOM node (already
// conferring) it dispatches to the human; otherwise it advances to the next
// supervisor. Only the node currently holding the escalation may forward it.
func (e *Engine) Forward(ctx context.Context, id, fromSessionID, fromRole, note string) (*Escalation, error) {
	esc, err := e.cfg.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if esc.resolved() {
		return nil, fmt.Errorf("escalation %s is already %s", id, esc.Phase)
	}
	if fromSessionID != "" && !esc.isHolder(fromSessionID) {
		return nil, fmt.Errorf(
			"escalation %s is currently held by %q, not %q — only the holder may forward it",
			id, esc.CurrentSessionID, fromSessionID)
	}

	if esc.CurrentKind.IsVROOM() {
		// The trio conferred and could not answer → escalate to the human.
		if err := e.dispatchToHuman(ctx, esc, fromSessionID, fromRole, note); err != nil {
			return nil, err
		}
	} else {
		if err := e.advance(ctx, esc); err != nil {
			return nil, err
		}
	}
	if err := e.cfg.Store.Put(ctx, esc); err != nil {
		return nil, err
	}
	return esc, nil
}

// Await blocks (polling the Store) until the escalation reaches a terminal phase
// or timeout elapses. Only the asking worker calls this; supervisors never
// block. On timeout it records a PhaseTimedOut event and returns ErrAwaitTimeout
// with the latest escalation state.
func (e *Engine) Await(ctx context.Context, id string, timeout, poll time.Duration) (*Escalation, error) {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	deadline := e.now().Add(timeout)
	for {
		esc, err := e.cfg.Store.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if esc.resolved() {
			return esc, nil
		}
		if !e.now().Before(deadline) {
			e.emit(ctx, esc, PhaseTimedOut, eventFields{from: esc.OriginSessionID})
			return esc, ErrAwaitTimeout
		}
		select {
		case <-ctx.Done():
			return esc, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// Get returns the current state of an escalation.
func (e *Engine) Get(ctx context.Context, id string) (*Escalation, error) {
	return e.cfg.Store.Get(ctx, id)
}

// List returns escalations matching f (e.g. a supervisor's pending inbox).
func (e *Engine) List(ctx context.Context, f Filter) ([]*Escalation, error) {
	return e.cfg.Store.List(ctx, f)
}

func (e *Engine) now() time.Time { return e.cfg.Clock().UTC() }

func (e *Engine) message(esc *Escalation, from, fromRole, noteOrAnswer string) EscalationMessage {
	msg := EscalationMessage{
		EscalationID:   esc.ID,
		Kind:           esc.Kind,
		Mode:           esc.Mode,
		Question:       esc.Question,
		Context:        esc.Context,
		FromSessionID:  from,
		FromRole:       fromRole,
		MustReachHuman: esc.MustReachHuman,
	}
	if esc.Phase == PhaseAnswered {
		msg.Resolved = true
		msg.Answer = noteOrAnswer
	} else {
		msg.Note = noteOrAnswer
	}
	return msg
}

// eventFields carries the per-transition extras for emit.
type eventFields struct {
	from     string
	fromRole string
	to       string
	toRole   string
	answer   string
	note     string
	vote     string
	latency  int64
}

// emit builds and records an EscalationEvent for a transition. Logging failures
// are intentionally swallowed: the escalation must proceed even if the audit
// sink is temporarily unwritable (the OTel span still fires). A dropped audit
// line is a known, bounded loss; a wedged escalation is not.
func (e *Engine) emit(ctx context.Context, esc *Escalation, phase Phase, f eventFields) {
	ev := EscalationEvent{
		EscalationID:     esc.ID,
		Timestamp:        e.now(),
		Phase:            phase,
		Kind:             esc.Kind,
		Mode:             esc.Mode,
		OriginSessionID:  esc.OriginSessionID,
		OriginRole:       esc.OriginRole,
		FromSessionID:    f.from,
		FromRole:         f.fromRole,
		ToSessionID:      f.to,
		ToRole:           f.toRole,
		ChainDepth:       len(esc.Chain) - 1,
		Chain:            append([]string(nil), esc.Chain...),
		Question:         esc.Question,
		Topic:            esc.Topic,
		Answer:           f.answer,
		AnsweredBy:       esc.AnsweredBy,
		AnsweredByRole:   esc.AnsweredByRole,
		Disposition:      esc.Disposition,
		MustReachHuman:   esc.MustReachHuman,
		ClassifierReason: esc.ClassifierReason,
		Vote:             f.vote,
		LatencyMs:        f.latency,
	}
	if f.note != "" {
		// Fold a routing note into the classifier-reason column so it is visible
		// in the log without widening the schema.
		if ev.ClassifierReason != "" {
			ev.ClassifierReason += "; " + f.note
		} else {
			ev.ClassifierReason = f.note
		}
	}
	_ = e.cfg.Logger.Record(ctx, ev)
}

func inChain(chain []string, id string) bool {
	return slices.Contains(chain, id)
}
