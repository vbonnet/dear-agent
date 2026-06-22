package escalation

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// VROOMTrioSessionID is the sentinel "holder" of an escalation while the three
// VROOM supervisors confer. During a programmatic confer no single session
// holds the escalation; the ballot in [Escalation.Confer] does. Forward/vote
// holder checks treat any confer member as a holder (see [Escalation.isHolder]).
const VROOMTrioSessionID = "vroom-trio"

// Vote is one VROOM supervisor's ballot in a confer.
type Vote string

const (
	// VoteApprove endorses resolving the escalation with the voter's proposed
	// Answer. A quorum of approvals carrying the same answer resolves the
	// escalation with that answer.
	VoteApprove Vote = "approve"
	// VoteReject says the trio should not answer — push it to the human. A quorum
	// of rejections dispatches to the human.
	VoteReject Vote = "reject"
	// VoteAbstain neither approves nor rejects: it counts toward "everyone has
	// voted" but toward no quorum.
	VoteAbstain Vote = "abstain"
)

// Valid reports whether v is a recognised vote.
func (v Vote) Valid() bool {
	switch v {
	case VoteApprove, VoteReject, VoteAbstain:
		return true
	default:
		return false
	}
}

// Ballot is one trio member's cast vote in a confer.
type Ballot struct {
	SessionID string    `json:"session_id"`
	Role      string    `json:"role,omitempty"`
	Vote      Vote      `json:"vote"`
	Answer    string    `json:"answer,omitempty"` // proposed answer (approve only)
	Rationale string    `json:"rationale,omitempty"`
	At        time.Time `json:"at"`
}

// Confer is the programmatic trio quorum-vote record. It is created when an
// escalation reaches the VROOM mesh with a [VROOMRegistry] configured: the
// escalation is delivered to all members at once and resolves the moment a
// quorum of ballots agree (or, failing that, dispatches to the human).
type Confer struct {
	// Members are the VROOM session IDs invited to vote (the live trio).
	Members []string `json:"members"`
	// Quorum is the number of like votes needed to resolve. Default: a strict
	// majority of Members (2 of 3).
	Quorum    int       `json:"quorum"`
	Ballots   []Ballot  `json:"ballots,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

func (c *Confer) isMember(id string) bool { return slices.Contains(c.Members, id) }

func (c *Confer) hasVoted(id string) bool {
	for _, b := range c.Ballots {
		if b.SessionID == id {
			return true
		}
	}
	return false
}

// VROOMRegistry yields the live VROOM trio — the three apex supervisors a
// confer fans out to. It is the "registry of the live VROOM session IDs" the
// programmatic confer needs; the agm binary implements it over the session
// store (resolving the three well-known supervisor names to current sessions).
type VROOMRegistry interface {
	Members(ctx context.Context) ([]ParentRef, error)
}

// StaticRegistry is a fixed-membership VROOMRegistry for tests and
// config-driven deployments.
type StaticRegistry struct{ members []ParentRef }

// NewStaticRegistry builds a StaticRegistry over the given members.
func NewStaticRegistry(members ...ParentRef) *StaticRegistry {
	return &StaticRegistry{members: members}
}

// Members implements VROOMRegistry.
func (r *StaticRegistry) Members(context.Context) ([]ParentRef, error) {
	return append([]ParentRef(nil), r.members...), nil
}

var _ VROOMRegistry = (*StaticRegistry)(nil)

// quorumFor returns a strict majority of n (e.g. 2 of 3, 3 of 5).
func quorumFor(n int) int {
	if n <= 0 {
		return 1
	}
	return n/2 + 1
}

// isHolder reports whether id may act on (forward/vote) the escalation: the
// current single holder, or — during a programmatic confer — any trio member.
func (e *Escalation) isHolder(id string) bool {
	if id == e.CurrentSessionID {
		return true
	}
	if e.Confer != nil && e.Phase == PhaseConferring && e.Confer.isMember(id) {
		return true
	}
	return false
}

// beginConfer opens a programmatic confer: it looks up the live trio from the
// registry, records the ballot on the escalation, and delivers the question to
// every member at once. If the registry is unavailable or empty it falls back
// to single-node conferring (the pre-ce-es7z behaviour) against entry, so a
// missing registry degrades gracefully rather than wedging the escalation.
func (e *Engine) beginConfer(ctx context.Context, esc *Escalation, entry ParentRef, from, fromRole string) error {
	members, err := e.cfg.Registry.Members(ctx)
	if err != nil || len(members) == 0 {
		return e.routeSingleVROOM(ctx, esc, entry, from, fromRole)
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if m.SessionID != "" {
			ids = append(ids, m.SessionID)
		}
	}
	if len(ids) == 0 {
		return e.routeSingleVROOM(ctx, esc, entry, from, fromRole)
	}

	now := e.now()
	quorum := e.cfg.ConferQuorum
	if quorum <= 0 || quorum > len(ids) {
		quorum = quorumFor(len(ids))
	}
	esc.Confer = &Confer{Members: ids, Quorum: quorum, StartedAt: now}
	esc.Chain = append(esc.Chain, VROOMTrioSessionID)
	esc.CurrentSessionID = VROOMTrioSessionID
	esc.CurrentRole = "vroom-trio"
	esc.CurrentKind = entry.Kind // a VROOM kind ⇒ a later Forward dispatches to the human
	esc.Phase = PhaseConferring
	esc.UpdatedAt = now

	// Deliver the confer ask to every member. Best-effort: the Store is the
	// source of truth and members also poll via `escalate list`, so one failed
	// delivery must not abort the confer.
	for _, m := range members {
		if m.SessionID == "" {
			continue
		}
		_ = e.cfg.Messenger.Deliver(ctx, m.SessionID, e.message(esc, from, fromRole, conferAskNote(esc)))
	}
	e.emit(ctx, esc, PhaseConferring, eventFields{
		from: from, fromRole: fromRole, to: VROOMTrioSessionID, toRole: "vroom-trio",
		note: fmt.Sprintf("confer opened across %d trio members, quorum %d", len(ids), quorum),
	})
	return nil
}

// routeSingleVROOM is the legacy path: deliver the escalation to a single VROOM
// node and mark it conferring (the node then confers via peer messaging). Used
// when no registry is configured or the registry yields no live members.
func (e *Engine) routeSingleVROOM(ctx context.Context, esc *Escalation, pref ParentRef, from, fromRole string) error {
	esc.Chain = append(esc.Chain, pref.SessionID)
	esc.CurrentSessionID = pref.SessionID
	esc.CurrentRole = pref.Role
	esc.CurrentKind = pref.Kind
	esc.Phase = PhaseConferring
	esc.UpdatedAt = e.now()
	if err := e.cfg.Messenger.Deliver(ctx, pref.SessionID, e.message(esc, from, fromRole, "")); err != nil {
		return fmt.Errorf("escalation: deliver to %q: %w", pref.SessionID, err)
	}
	e.emit(ctx, esc, PhaseConferring, eventFields{
		from: from, fromRole: fromRole, to: pref.SessionID, toRole: pref.Role,
	})
	return nil
}

// CastVote records one trio member's ballot on a conferring escalation and, if
// the ballot completes a quorum, resolves the escalation: a quorum of approvals
// on the same answer answers it (unless it is must-reach-human, where the trio
// may only advise); a quorum of rejections — or every member having voted
// without an answer quorum — dispatches it to the human. Only invited members
// may vote, and each may vote once. Supervisors are never blocked: a member
// votes and returns; the asking worker's Await observes the resolution.
func (e *Engine) CastVote(ctx context.Context, id, bySessionID, byRole string, v Vote, answer, rationale string) (*Escalation, error) {
	if !v.Valid() {
		return nil, fmt.Errorf("escalation: invalid vote %q (want approve|reject|abstain)", v)
	}
	if v == VoteApprove && strings.TrimSpace(answer) == "" {
		return nil, fmt.Errorf("escalation: an approve vote must carry a proposed answer (--answer)")
	}
	esc, err := e.cfg.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if esc.resolved() {
		return nil, fmt.Errorf("escalation %s is already %s", id, esc.Phase)
	}
	if esc.Confer == nil || esc.Phase != PhaseConferring {
		return nil, fmt.Errorf("escalation %s is not in a programmatic confer (phase %s)", id, esc.Phase)
	}
	if bySessionID == HumanSessionID {
		return nil, fmt.Errorf("the human resolves with `answer --by human`, not a trio vote")
	}
	if !esc.Confer.isMember(bySessionID) {
		return nil, fmt.Errorf("session %q is not a member of this confer", bySessionID)
	}
	if esc.Confer.hasVoted(bySessionID) {
		return nil, fmt.Errorf("session %q has already voted on escalation %s", bySessionID, id)
	}

	now := e.now()
	esc.Confer.Ballots = append(esc.Confer.Ballots, Ballot{
		SessionID: bySessionID, Role: byRole, Vote: v, Answer: answer, Rationale: rationale, At: now,
	})
	esc.UpdatedAt = now
	e.emit(ctx, esc, PhaseConferring, eventFields{
		from: bySessionID, fromRole: byRole, vote: string(v), note: rationale,
	})

	//nolint:exhaustive // conferPending is the implicit no-op (fall through to Put below).
	switch outcome, detail := e.decideConfer(esc); outcome {
	case conferAnswer:
		return e.resolveConferAnswer(ctx, esc, detail)
	case conferHuman:
		if err := e.dispatchToHuman(ctx, esc, VROOMTrioSessionID, "vroom-trio", e.conferNote(esc, detail)); err != nil {
			return nil, err
		}
	}
	if err := e.cfg.Store.Put(ctx, esc); err != nil {
		return nil, err
	}
	return esc, nil
}

// conferOutcome is the result of tallying a confer's ballots after a vote.
type conferOutcome int

const (
	conferPending conferOutcome = iota // not enough votes yet — keep waiting
	conferAnswer                       // a quorum agreed on an answer
	conferHuman                        // resolve by dispatching to the human
)

// decideConfer tallies the ballots and decides what happens next. For
// conferAnswer the second return is the winning answer; for conferHuman it is a
// short human-readable reason.
func (e *Engine) decideConfer(esc *Escalation) (conferOutcome, string) {
	c := esc.Confer
	total := len(c.Ballots)
	remaining := len(c.Members) - total

	var rejects int
	approveCount := map[string]int{}   // normalised answer → vote count
	approveText := map[string]string{} // normalised answer → first verbatim answer
	var order []string                 // answer keys in first-seen order (stable tie-break)
	for _, b := range c.Ballots {
		switch b.Vote {
		case VoteReject:
			rejects++
		case VoteApprove:
			key := NormalizeQuestion(b.Answer)
			if approveCount[key] == 0 {
				order = append(order, key)
				approveText[key] = b.Answer
			}
			approveCount[key]++
		case VoteAbstain:
			// counts toward "everyone voted" only
		}
	}
	bestKey, bestCount := "", 0
	for _, k := range order {
		if approveCount[k] > bestCount {
			bestKey, bestCount = k, approveCount[k]
		}
	}

	// 1. An answer reached quorum — but the trio may not answer a must-reach-human
	//    escalation; for those it only advises, and the human decides.
	if !esc.MustReachHuman && bestCount >= c.Quorum {
		return conferAnswer, approveText[bestKey]
	}
	// 2. Rejections reached quorum → the trio voted to hand it up.
	if rejects >= c.Quorum {
		return conferHuman, "trio reached quorum to escalate to the human"
	}
	// 3. Must-reach-human: once a quorum has weighed in, dispatch with the trio's
	//    recommendation rather than waiting for unanimity.
	if esc.MustReachHuman && total >= c.Quorum {
		return conferHuman, "must-reach-human: trio recommendation gathered"
	}
	// 4. Everyone voted but no answer reached quorum → the trio is deadlocked.
	if remaining == 0 {
		return conferHuman, "trio could not reach quorum on an answer"
	}
	// Otherwise wait for the remaining members.
	return conferPending, ""
}

// resolveConferAnswer records the trio's quorum answer as the terminal answer
// and returns it to the origin worker.
func (e *Engine) resolveConferAnswer(ctx context.Context, esc *Escalation, answer string) (*Escalation, error) {
	now := e.now()
	esc.Answer = answer
	esc.AnsweredBy = VROOMTrioSessionID
	esc.AnsweredByRole = "vroom-trio"
	esc.Phase = PhaseAnswered
	esc.ResolvedAt = now
	esc.UpdatedAt = now
	if esc.OriginSessionID != "" && esc.OriginSessionID != VROOMTrioSessionID {
		_ = e.cfg.Messenger.Deliver(ctx, esc.OriginSessionID, e.message(esc, VROOMTrioSessionID, "vroom-trio", answer))
	}
	e.emit(ctx, esc, PhaseAnswered, eventFields{
		from: VROOMTrioSessionID, fromRole: "vroom-trio", answer: answer,
		latency: now.Sub(esc.CreatedAt).Milliseconds(),
	})
	if err := e.cfg.Store.Put(ctx, esc); err != nil {
		return nil, err
	}
	return esc, nil
}

// conferAskNote is the instruction delivered to each trio member when a confer
// opens, telling them how to cast a vote.
func conferAskNote(esc *Escalation) string {
	return "VROOM trio confer — vote with `agm escalate vote " + esc.ID +
		" approve --answer \"<answer>\"` or `agm escalate vote " + esc.ID +
		" reject --rationale \"<why>\"`. Quorum: " + strconv.Itoa(esc.Confer.Quorum) +
		" of " + strconv.Itoa(len(esc.Confer.Members)) + " members."
}

// conferNote summarises the ballot for the human when a confer dispatches up,
// so the dispatch record carries the trio's reasoning and recommendations.
func (e *Engine) conferNote(esc *Escalation, detail string) string {
	var b strings.Builder
	b.WriteString("VROOM trio confer → human: ")
	b.WriteString(detail)
	if esc.Confer != nil {
		for _, bl := range esc.Confer.Ballots {
			fmt.Fprintf(&b, "; %s voted %s", roleOr(bl.Role, bl.SessionID), bl.Vote)
			switch {
			case bl.Answer != "":
				fmt.Fprintf(&b, " (%s)", bl.Answer)
			case bl.Rationale != "":
				fmt.Fprintf(&b, " (%s)", bl.Rationale)
			}
		}
	}
	return b.String()
}

func roleOr(role, fallback string) string {
	if role != "" {
		return role
	}
	return fallback
}
