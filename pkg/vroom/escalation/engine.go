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

const defaultPostCommitEffectTimeout = 10 * time.Second

// ErrAwaitTimeout is returned by Await when the escalation did not resolve in
// time. The escalation remains in-flight; only the worker's wait ended.
var ErrAwaitTimeout = errors.New("escalation: await timed out")

// CommittedEffectError reports that state committed but a required
// post-commit notification or dispatch failed. Callers must not retry the state
// transition as though it were rolled back; inspect the returned Escalation and
// retry/reconcile the external effect instead.
type CommittedEffectError struct {
	EscalationID string
	Phase        Phase
	Err          error
}

func (e *CommittedEffectError) Error() string {
	return fmt.Sprintf(
		"escalation %s committed phase %s but a post-commit effect failed: %v",
		e.EscalationID, e.Phase, e.Err,
	)
}

// Unwrap exposes the underlying effect failure.
func (e *CommittedEffectError) Unwrap() error { return e.Err }

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
	// PostCommitEffectTimeout bounds each individual delivery, human dispatch,
	// and audit write after state commits. Default 10 seconds per effect.
	PostCommitEffectTimeout time.Duration
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

// transitionEffect is one externally visible action to perform after a state
// transition has committed. Store mutation callbacks only build these plans;
// they never message, dispatch, or log, so a losing concurrent mutation has no
// effects to undo.
type transitionEffect struct {
	kind       transitionEffectKind
	to         string
	message    EscalationMessage
	event      EscalationEvent
	bestEffort bool
}

type transitionEffectKind uint8

const (
	transitionDeliver transitionEffectKind = iota
	transitionDispatchHuman
	transitionRecordEvent
)

type transitionPlan struct {
	effects []transitionEffect
}

func (p *transitionPlan) deliver(to string, msg EscalationMessage, bestEffort bool) {
	p.effects = append(p.effects, transitionEffect{
		kind: transitionDeliver, to: to, message: msg, bestEffort: bestEffort,
	})
}

func (p *transitionPlan) dispatchHuman() {
	p.effects = append(p.effects, transitionEffect{kind: transitionDispatchHuman})
}

func (p *transitionPlan) record(ev EscalationEvent) {
	p.effects = append(p.effects, transitionEffect{kind: transitionRecordEvent, event: ev})
}

func (p *transitionPlan) append(other transitionPlan) {
	p.effects = append(p.effects, other.effects...)
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
	if cfg.PostCommitEffectTimeout <= 0 {
		cfg.PostCommitEffectTimeout = defaultPostCommitEffectTimeout
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

	var plan transitionPlan
	plan.record(e.event(esc, PhaseRaised, eventFields{
		from: esc.OriginSessionID, fromRole: esc.OriginRole,
	}))

	if verdict.Disposition == DispAutoResolve {
		esc.Answer = verdict.Answer
		esc.AnsweredBy = "auto"
		esc.AnsweredByRole = "classifier"
		esc.Phase = PhaseAutoResolved
		esc.ResolvedAt = now
		esc.UpdatedAt = now
		plan.record(e.event(esc, PhaseAutoResolved, eventFields{
			from: "auto", answer: verdict.Answer, latency: 0,
		}))
		if err := e.cfg.Store.Create(ctx, esc); err != nil {
			return nil, err
		}
		return e.finishTransition(ctx, esc, plan)
	}

	// Route the first hop: from the origin to its supervisor.
	routePlan, err := e.planAdvance(ctx, esc)
	if err != nil {
		return nil, err
	}
	plan.append(routePlan)
	if err := e.cfg.Store.Create(ctx, esc); err != nil {
		return nil, err
	}
	return e.finishTransition(ctx, esc, plan)
}

type advanceTarget struct {
	parent          ParentRef
	humanReason     string
	registryMembers []ParentRef
	useRegistry     bool
}

// resolveAdvanceTarget performs hierarchy and registry reads before a Store
// mutation begins. The resulting proposal is revalidated against the latest
// committed holder before it is applied.
func (e *Engine) resolveAdvanceTarget(ctx context.Context, esc *Escalation) (advanceTarget, error) {
	if len(esc.Chain) > e.cfg.MaxHops {
		return advanceTarget{humanReason: "hop limit reached"}, nil
	}

	pref, err := e.cfg.Graph.Parent(ctx, esc.CurrentSessionID)
	if err != nil {
		return advanceTarget{}, fmt.Errorf("escalation: lookup parent of %q: %w", esc.CurrentSessionID, err)
	}

	// No parent, or a cycle: fall back to the VROOM entry if configured, else
	// straight to the human.
	if pref.Kind == NodeNone || pref.SessionID == "" || inChain(esc.Chain, pref.SessionID) {
		if e.cfg.VROOMEntry != nil && !esc.CurrentKind.IsVROOM() && !inChain(esc.Chain, e.cfg.VROOMEntry.SessionID) {
			pref = *e.cfg.VROOMEntry
		} else {
			return advanceTarget{humanReason: "no further supervisor in the chain"}, nil
		}
	}

	target := advanceTarget{parent: pref}
	if pref.Kind.IsVROOM() && e.cfg.Registry != nil {
		target.useRegistry = true
		// Registry membership is an optional liveness hint. Failure degrades to
		// the durable single-node route.
		target.registryMembers, _ = e.cfg.Registry.Members(ctx)
	}
	return target, nil
}

// planAdvance moves the escalation one hop up from its current holder and
// returns the effects to perform only after the caller commits the new state.
func (e *Engine) planAdvance(ctx context.Context, esc *Escalation) (transitionPlan, error) {
	target, err := e.resolveAdvanceTarget(ctx, esc)
	if err != nil {
		return transitionPlan{}, err
	}
	return e.planAdvanceTo(esc, target), nil
}

// planAdvanceTo applies a pre-resolved routing target without calling any
// external collaborator. It is safe to call from a Store mutation.
func (e *Engine) planAdvanceTo(esc *Escalation, target advanceTarget) transitionPlan {
	from := esc.CurrentSessionID
	fromRole := esc.CurrentRole
	if target.humanReason != "" {
		return e.planDispatchToHuman(esc, from, fromRole, target.humanReason)
	}
	pref := target.parent

	// Reaching the VROOM mesh runs a programmatic confer across the live trio
	// when a registry is configured; otherwise it falls back to delivering to the
	// single node, which confers via peer messaging (pre-ce-es7z behaviour).
	if pref.Kind.IsVROOM() {
		if target.useRegistry {
			return e.planBeginConfer(esc, pref, from, fromRole, target.registryMembers)
		}
		return e.planRouteSingleVROOM(esc, pref, from, fromRole)
	}

	esc.Chain = append(esc.Chain, pref.SessionID)
	esc.CurrentSessionID = pref.SessionID
	esc.CurrentRole = pref.Role
	esc.CurrentKind = pref.Kind
	esc.UpdatedAt = e.now()
	esc.Phase = PhaseRouted

	var plan transitionPlan
	plan.deliver(pref.SessionID, e.message(esc, from, fromRole, ""), false)
	plan.record(e.event(esc, PhaseRouted, eventFields{
		from: from, fromRole: fromRole, to: pref.SessionID, toRole: pref.Role,
	}))
	return plan
}

// planDispatchToHuman moves an escalation to the human and defers dispatch and
// logging until after the state transition commits. It is not a terminal
// phase: the human answers later via Answer(HumanSessionID, ...).
func (e *Engine) planDispatchToHuman(esc *Escalation, from, fromRole, reason string) transitionPlan {
	esc.CurrentSessionID = HumanSessionID
	esc.CurrentRole = "human"
	esc.CurrentKind = NodeHuman
	esc.Phase = PhaseDispatchedToHuman
	esc.UpdatedAt = e.now()
	var plan transitionPlan
	plan.dispatchHuman()
	plan.record(e.event(esc, PhaseDispatchedToHuman, eventFields{
		from: from, fromRole: fromRole, to: HumanSessionID, toRole: "human", note: reason,
	}))
	return plan
}

// Answer records a terminal answer and returns it to the origin session. It is
// refused if the escalation must reach the human and the answerer is not the
// human — a supervisor faced with a product decision must Forward (adding a
// recommendation), not decide it.
func (e *Engine) Answer(ctx context.Context, id, bySessionID, byRole, answer string) (*Escalation, error) {
	if strings.TrimSpace(answer) == "" {
		return nil, fmt.Errorf("escalation: answer text is required")
	}
	if strings.TrimSpace(bySessionID) == "" {
		return nil, fmt.Errorf("escalation: answerer session id is required")
	}
	var plan transitionPlan
	esc, err := e.cfg.Store.Update(ctx, id, func(current *Escalation) error {
		if current.resolved() {
			return fmt.Errorf("escalation %s is already %s", id, current.Phase)
		}
		// The human may override an in-flight escalation. Every non-human actor
		// must still hold the latest committed state; this closes the race where
		// a supervisor forwards and then answers from its stale view.
		if bySessionID != HumanSessionID && !current.isHolder(bySessionID) {
			return fmt.Errorf(
				"escalation %s is currently held by %q, not %q — only the holder or human may answer it",
				id, current.CurrentSessionID, bySessionID)
		}
		// During a programmatic confer, a trio member must vote rather than answer
		// unilaterally — the quorum, not a single supervisor, decides. The human may
		// still answer directly (an override of the in-flight confer).
		if current.Confer != nil && current.Phase == PhaseConferring &&
			bySessionID != HumanSessionID && current.Confer.isMember(bySessionID) {
			return fmt.Errorf(
				"escalation %s is in a VROOM trio confer — cast a vote (`escalate vote %s approve|reject`), "+
					"do not answer unilaterally", id, id)
		}
		if current.MustReachHuman && bySessionID != HumanSessionID {
			return fmt.Errorf(
				"escalation %s must reach the human (%s): a node below the human may not answer it.\n"+
					"The right way: `escalate forward` with your recommendation so the human decides.\n"+
					"Because: %s", id, current.Topic, current.ClassifierReason)
		}

		now := e.now()
		current.Answer = answer
		current.AnsweredBy = bySessionID
		current.AnsweredByRole = byRole
		current.Phase = PhaseAnswered
		current.ResolvedAt = now
		current.UpdatedAt = now

		// Answer delivery remains best-effort, matching the pre-atomic behavior.
		if current.OriginSessionID != "" && current.OriginSessionID != bySessionID {
			plan.deliver(current.OriginSessionID, e.message(current, bySessionID, byRole, answer), true)
		}
		plan.record(e.event(current, PhaseAnswered, eventFields{
			from: bySessionID, fromRole: byRole, answer: answer,
			latency: now.Sub(current.CreatedAt).Milliseconds(),
		}))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e.finishTransition(ctx, esc, plan)
}

// Forward escalates one more hop up the chain. From a VROOM node (already
// conferring) it dispatches to the human; otherwise it advances to the next
// supervisor. Only the node currently holding the escalation may forward it.
func (e *Engine) Forward(ctx context.Context, id, fromSessionID, fromRole, note string) (*Escalation, error) {
	if strings.TrimSpace(fromSessionID) == "" {
		return nil, fmt.Errorf("escalation: forwarding session id is required")
	}
	snapshot, err := e.cfg.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if snapshot.resolved() {
		return nil, fmt.Errorf("escalation %s is already %s", id, snapshot.Phase)
	}
	if !snapshot.isHolder(fromSessionID) {
		return nil, fmt.Errorf(
			"escalation %s is currently held by %q, not %q — only the holder may forward it",
			id, snapshot.CurrentSessionID, fromSessionID)
	}
	var target advanceTarget
	if !snapshot.CurrentKind.IsVROOM() {
		target, err = e.resolveAdvanceTarget(ctx, snapshot)
		if err != nil {
			return nil, err
		}
	}

	var plan transitionPlan
	esc, err := e.cfg.Store.Update(ctx, id, func(current *Escalation) error {
		if current.resolved() {
			return fmt.Errorf("escalation %s is already %s", id, current.Phase)
		}
		if !current.isHolder(fromSessionID) {
			return fmt.Errorf(
				"escalation %s is currently held by %q, not %q — only the holder may forward it",
				id, current.CurrentSessionID, fromSessionID)
		}

		if current.CurrentKind.IsVROOM() {
			// The trio conferred and could not answer → escalate to the human.
			plan = e.planDispatchToHuman(current, fromSessionID, fromRole, note)
			return nil
		}
		if !sameAdvanceBase(snapshot, current) {
			return fmt.Errorf("escalation %s changed while its route was being planned; retry", id)
		}
		plan = e.planAdvanceTo(current, target)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e.finishTransition(ctx, esc, plan)
}

func sameAdvanceBase(snapshot, current *Escalation) bool {
	return snapshot.ID == current.ID &&
		snapshot.Phase == current.Phase &&
		snapshot.CurrentSessionID == current.CurrentSessionID &&
		snapshot.CurrentRole == current.CurrentRole &&
		snapshot.CurrentKind == current.CurrentKind &&
		snapshot.UpdatedAt.Equal(current.UpdatedAt) &&
		slices.Equal(snapshot.Chain, current.Chain)
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

// event snapshots one transition before its post-commit effects run. Keeping
// the snapshot in the transition plan preserves raised/routed field values even
// when the committed record has already advanced to a later phase.
func (e *Engine) event(esc *Escalation, phase Phase, f eventFields) EscalationEvent {
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
	return ev
}

// emit builds and records an EscalationEvent for a transition. Logging failures
// are intentionally swallowed: the escalation must proceed even if the audit
// sink is temporarily unwritable (the OTel span still fires). A dropped audit
// line is a known, bounded loss; a wedged escalation is not.
func (e *Engine) emit(ctx context.Context, esc *Escalation, phase Phase, f eventFields) {
	_ = e.cfg.Logger.Record(ctx, e.event(esc, phase, f))
}

// finishTransition performs only the effects owned by a committed state
// transition. Required effect failures return the committed state together
// with an explicit error so callers never mistake a successful retry for safe.
func (e *Engine) finishTransition(ctx context.Context, esc *Escalation, plan transitionPlan) (*Escalation, error) {
	// Once state commits, request cancellation must not silently suppress its
	// notification or audit. Preserve trace values and detach cancellation;
	// applyTransition gives every effect its own fresh bound.
	effectParent := context.Background()
	if ctx != nil {
		effectParent = context.WithoutCancel(ctx)
	}
	if err := e.applyTransition(effectParent, esc, plan); err != nil {
		return esc, &CommittedEffectError{EscalationID: esc.ID, Phase: esc.Phase, Err: err}
	}
	return esc, nil
}

func (e *Engine) applyTransition(ctx context.Context, esc *Escalation, plan transitionPlan) error {
	var effectErr error
	for _, effect := range plan.effects {
		effectCtx, cancel := context.WithTimeout(ctx, e.cfg.PostCommitEffectTimeout)
		switch effect.kind {
		case transitionDeliver:
			if err := e.cfg.Messenger.Deliver(effectCtx, effect.to, effect.message); err != nil && !effect.bestEffort {
				effectErr = errors.Join(effectErr, fmt.Errorf("deliver to %q: %w", effect.to, err))
			}
		case transitionDispatchHuman:
			// HumanDispatch is an external trust boundary and may retain or mutate
			// its input. Give it a private copy so the caller's committed snapshot
			// remains stable after finishTransition returns.
			if err := e.cfg.Human.ToHuman(effectCtx, cloneEscalation(esc)); err != nil {
				effectErr = errors.Join(effectErr, fmt.Errorf("dispatch to human: %w", err))
			}
		case transitionRecordEvent:
			_ = e.cfg.Logger.Record(effectCtx, effect.event)
		default:
			effectErr = errors.Join(effectErr, fmt.Errorf("unknown transition effect kind %d", effect.kind))
		}
		cancel()
	}
	return effectErr
}

func inChain(chain []string, id string) bool {
	return slices.Contains(chain, id)
}
