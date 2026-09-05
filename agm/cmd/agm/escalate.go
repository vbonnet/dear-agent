package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/internal/safegit"
	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
	"github.com/vbonnet/dear-agent/pkg/vroom/escalation"
)

// escalate wires the pure pkg/vroom/escalation engine to AGM's session
// hierarchy (the routing graph), `agm send` (delivery), and the Dispatch trail
// (the human surface). It implements the escalation decision in ADR-031.
//
//	agm escalate ask     <question>   — a worker raises a question/decision
//	agm escalate answer  <id> <text>  — a supervisor (or the human) answers
//	agm escalate forward <id>         — a supervisor escalates one hop further
//	agm escalate list                 — pending escalations (a supervisor inbox)
//	agm escalate show    <id>         — full state of one escalation

var (
	escKind     string
	escMode     string
	escContext  string
	escSession  string
	escRole     string
	escTimeout  time.Duration
	escVROOM    string
	escBy       string
	escNote     string
	escAnswer   string
	escPending  bool
	escListMine bool
)

var escalateCmd = &cobra.Command{
	Use:   "escalate",
	Short: "Escalate a question/decision up the VROOM supervisor chain",
	Long: `Escalate a question or decision an agent cannot resolve to its
supervisor (whoever spawned it). If the supervisor cannot answer confidently it
forwards one hop further; the chain walks up to the three VROOM supervisors,
who confer, and finally to the human via Dispatch.

Self-evident questions ("should I proceed with the task you assigned me?") are
auto-answered and never reach a human. High-stakes decisions (product, pricing,
destructive, legal, …) must reach the human and cannot be answered by any node
below it.

Blocking mode blocks only the asking worker (it polls for the answer);
supervisors are never blocked — they drain escalations with 'list'/'answer'/
'forward' on their own schedule.`,
}

var escalateAskCmd = &cobra.Command{
	Use:   "ask <question>",
	Short: "Raise a question/decision to your supervisor",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEscalateAsk,
}

var escalateAnswerCmd = &cobra.Command{
	Use:   "answer <id> <answer>",
	Short: "Answer an escalation (supervisor or human)",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runEscalateAnswer,
}

var escalateForwardCmd = &cobra.Command{
	Use:   "forward <id>",
	Short: "Escalate one hop further up the chain",
	Args:  cobra.ExactArgs(1),
	RunE:  runEscalateForward,
}

var escalateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List escalations (default: all; --pending / --mine to narrow)",
	Args:  cobra.NoArgs,
	RunE:  runEscalateList,
}

var escalateShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show one escalation in full",
	Args:  cobra.ExactArgs(1),
	RunE:  runEscalateShow,
}

var escalateVoteCmd = &cobra.Command{
	Use:   "vote <id> <approve|reject|abstain>",
	Short: "Cast a VROOM trio vote on a conferring escalation",
	Long: `Cast your ballot in a programmatic VROOM-trio confer. When an
escalation reaches the mesh it is delivered to all three VROOM supervisors at
once; the escalation resolves the moment a quorum (2 of 3 by default) agree.

  approve --answer "<answer>"   endorse resolving with this answer
  reject  --rationale "<why>"   push to the human instead
  abstain --rationale "<why>"   neither; counts toward "everyone voted"

A quorum of matching approvals answers the escalation (and returns the answer to
the asking worker); a quorum of rejections — or every member voting without an
answer quorum — dispatches it to the human via Dispatch. Must-reach-human
escalations are never answered by the trio: the vote only gathers the trio's
recommendation before the human decides.`,
	Args: cobra.ExactArgs(2),
	RunE: runEscalateVote,
}

func init() {
	escalateAskCmd.Flags().StringVar(&escKind, "kind", "question", "question | decision | blocked-action")
	escalateAskCmd.Flags().StringVar(&escMode, "mode", "async", "blocking | async (blocking waits for the answer)")
	escalateAskCmd.Flags().StringVar(&escContext, "context", "", "supporting context for the question")
	escalateAskCmd.Flags().StringVar(&escSession, "session", "", "acting session id/name (default: current)")
	escalateAskCmd.Flags().StringVar(&escRole, "role", "", "acting session role label")
	escalateAskCmd.Flags().DurationVar(&escTimeout, "timeout", 10*time.Minute, "blocking-mode wait timeout")
	escalateAskCmd.Flags().StringVar(&escVROOM, "vroom-entry", "",
		"session to route orphan chains into. Omit to discover a live VROOM node; "+
			"pass an empty value, \"none\", or \"off\" to keep orphan chains local")

	escalateAnswerCmd.Flags().StringVar(&escBy, "by", "", "answering session id/name (use 'human' for the human; default: current)")
	escalateAnswerCmd.Flags().StringVar(&escRole, "role", "", "answering session role label")

	escalateForwardCmd.Flags().StringVar(&escSession, "from", "", "forwarding session id/name (default: current)")
	escalateForwardCmd.Flags().StringVar(&escRole, "role", "", "forwarding session role label")
	escalateForwardCmd.Flags().StringVar(&escNote, "note", "", "why you cannot answer / recommendation to pass up")

	escalateListCmd.Flags().BoolVar(&escPending, "pending", false, "only unresolved escalations")
	escalateListCmd.Flags().BoolVar(&escListMine, "mine", false, "only escalations currently held by this session")

	escalateVoteCmd.Flags().StringVar(&escBy, "by", "", "voting session id/name (default: current)")
	escalateVoteCmd.Flags().StringVar(&escRole, "role", "", "voting session role label")
	escalateVoteCmd.Flags().StringVar(&escAnswer, "answer", "", "proposed answer (required for approve)")
	escalateVoteCmd.Flags().StringVar(&escNote, "rationale", "", "why you voted this way (recorded for audit)")

	escalateCmd.AddCommand(escalateAskCmd, escalateAnswerCmd, escalateForwardCmd, escalateListCmd, escalateShowCmd, escalateVoteCmd)
	rootCmd.AddCommand(escalateCmd)
}

// --- engine construction (adapters over AGM) -------------------------------

// escalationEnv bundles the engine with the adapter it needs to resolve
// session identifiers to canonical UUIDs.
type escalationEnv struct {
	eng     *escalation.Engine
	adapter *dolt.Adapter
	logger  *escalation.Logger
}

func newEscalationEnv(vroomEntry vroomEntrySpec) (*escalationEnv, func(), error) {
	adapter, err := getStorage()
	if err != nil {
		return nil, nil, fmt.Errorf("escalate needs the session store: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		_ = adapter.Close()
		return nil, nil, err
	}
	base := filepath.Join(home, ".agm", "escalation")
	store, err := escalation.NewFileStore(filepath.Join(base, "store"))
	if err != nil {
		_ = adapter.Close()
		return nil, nil, err
	}
	sink, err := escalation.OpenJSONLSink(filepath.Join(base, "events.jsonl"))
	if err != nil {
		_ = adapter.Close()
		return nil, nil, err
	}
	logger := escalation.NewLogger(sink)

	var entry *escalation.ParentRef
	switch {
	case vroomEntry.discover:
		entry = discoverEscalationEntry(adapter)
	case vroomEntry.name != "":
		if m, err := adapter.ResolveIdentifier(vroomEntry.name); err == nil && m != nil {
			entry = &escalation.ParentRef{
				SessionID: m.SessionID, Role: m.Name, Kind: nodeKindFor(m),
			}
		}
	}

	// The must-reach-human boundary is the declarative auto-approve taxonomy. By
	// default it is the built-in DefaultClassifier; when the repo's
	// .safe-merge.yml carries an approval_policy block, gate on that instead so
	// the boundary is tuned by reviewable policy, not code (bead ce-onj5). A
	// missing file, no approval_policy, or a load error all fall back to the
	// built-in default — the gate never silently disappears.
	var classifier escalation.Classifier = escalation.DefaultClassifier{}
	if smCfg, cerr := safegit.LoadRepoConfig(); cerr == nil && smCfg != nil && smCfg.ApprovalPolicy != nil {
		if pc, perr := escalation.NewPolicyClassifier(smCfg.EscalationPolicy()); perr == nil {
			classifier = pc
		}
	}

	eng, err := escalation.NewEngine(escalation.Config{
		Graph:      &doltGraph{adapter: adapter},
		Classifier: classifier,
		Messenger:  &agmMessenger{},
		Human:      &dispatchHuman{home: home},
		Store:      store,
		Logger:     logger,
		VROOMEntry: entry,
		Registry:   &doltVROOMRegistry{adapter: adapter},
	})
	if err != nil {
		_ = adapter.Close()
		_ = logger.Close()
		return nil, nil, err
	}

	cleanup := func() { _ = logger.Close(); _ = adapter.Close() }
	return &escalationEnv{eng: eng, adapter: adapter, logger: logger}, cleanup, nil
}

// resolveSessionID turns an id-or-name (or "" → current session) into a
// canonical session UUID. The sentinel "human" passes through unchanged.
func (e *escalationEnv) resolveSessionID(identifier string) (sessionID, role string, err error) {
	if identifier == escalation.HumanSessionID {
		return escalation.HumanSessionID, "human", nil
	}
	if identifier == "" {
		name, derr := session.GetCurrentSessionName(cfg.SessionsDir, e.adapter)
		if derr != nil || name == "" {
			return "", "", fmt.Errorf("could not detect current session; pass --session/--from/--by explicitly: %w", derr)
		}
		identifier = name
	}
	m, err := e.adapter.ResolveIdentifier(identifier)
	if err != nil || m == nil {
		return "", "", fmt.Errorf("unknown session %q: %w", identifier, err)
	}
	return m.SessionID, m.Name, nil
}

// nodeKindFor maps a session manifest to its position in the escalation
// hierarchy. The three VROOM supervisors are recognised by name; everything
// else with children is treated as an intermediate supervisor for routing.
func nodeKindFor(m *manifest.Manifest) escalation.NodeKind {
	switch m.Name {
	case "vroom-meta-orchestrator":
		return escalation.NodeVROOMMetaOrchestrator
	case "vroom-orchestrator":
		return escalation.NodeVROOMOrchestrator
	case "vroom-overseer":
		return escalation.NodeVROOMOverseer
	default:
		return escalation.NodeSupervisor
	}
}

// vroomEntrySpec says how the orphan escalation entry is chosen. It is a
// type rather than a magic string because "not requested here", "discover
// one", and "explicitly disabled" are three different intents that an empty
// string cannot tell apart.
type vroomEntrySpec struct {
	discover bool
	name     string
}

// vroomEntryDisabled keeps orphan chains local: no entry is resolved.
var vroomEntryDisabled = vroomEntrySpec{}

// vroomEntryFromFlag reads --vroom-entry. An omitted flag discovers a live
// VROOM node; an explicitly empty value, or the "none"/"off" sentinels,
// disable orphan routing, which is the behavior the flag help has always
// advertised and which scripts rely on to keep chains local.
func vroomEntryFromFlag(cmd *cobra.Command, value string) vroomEntrySpec {
	if !cmd.Flags().Changed("vroom-entry") {
		return vroomEntrySpec{discover: true}
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none", "off":
		return vroomEntryDisabled
	}
	return vroomEntrySpec{name: strings.TrimSpace(value)}
}

// discoverEscalationEntry finds a live VROOM node to enter an orphan chain
// at.
//
// It considers only the VROOM trio. Any other live supervisor, Dispatch
// included, is classified NodeSupervisor by nodeKindFor, which bypasses the
// programmatic trio confer this command contracts for and forwards straight
// to the human. Discovering no VROOM node therefore leaves the chain local,
// exactly as a dead pinned vroom-orchestrator used to.
func discoverEscalationEntry(adapter *dolt.Adapter) *escalation.ParentRef {
	return discoverEscalationEntryWithTmux(adapter, tmuxClient)
}

// discoverEscalationEntryWithTmux is discoverEscalationEntry with its tmux
// dependency injected, so the liveness-verification behavior below can be
// tested without the package-global tmuxClient.
func discoverEscalationEntryWithTmux(adapter *dolt.Adapter, tmux session.TmuxInterface) *escalation.ParentRef {
	sessions, err := adapter.ListSessions(&dolt.SessionFilter{ExcludeArchived: true, Limit: 1000})
	if err != nil {
		return nil
	}
	for _, preferred := range vroomEntryPreference {
		for _, m := range sessions {
			if m == nil || m.Name != preferred {
				continue
			}
			// ops.SessionIsLiveConfirmed, not the plain SessionIsLive: a
			// manifest state of DONE is ambiguous between "finished a task,
			// idle at composer" and "harness process exited, pane
			// lingering" (its state-detector "safe default" origin — see
			// ops.SessionIsLiveConfirmed's doc). This PR made completion
			// routing treat DONE as reachable, which is correct there
			// because that DONE always comes from a confirmed-idle
			// completion watcher observation; it is not something
			// discoverEscalationEntry can assume for an arbitrary VROOM
			// node, so an escalation must additionally verify the harness is
			// actually still there before committing the chain to it. A
			// candidate that fails verification is skipped, not treated as
			// absent: the loop still falls through to the next preferred
			// VROOM node.
			if !ops.SessionIsLiveConfirmed(m, tmux) {
				continue
			}
			return &escalation.ParentRef{SessionID: m.SessionID, Role: m.Name, Kind: nodeKindFor(m)}
		}
	}
	return nil
}

// vroomEntryPreference ranks the VROOM trio for orphan entry, highest
// authority first.
var vroomEntryPreference = []string{
	"vroom-meta-orchestrator", "vroom-orchestrator", "vroom-overseer",
}

// doltGraph implements escalation.SessionGraph over AGM's Dolt hierarchy.
type doltGraph struct{ adapter *dolt.Adapter }

func (g *doltGraph) Parent(_ context.Context, sessionID string) (escalation.ParentRef, error) {
	parent, err := g.adapter.GetParent(sessionID)
	if err != nil {
		return escalation.ParentRef{}, err
	}
	if parent == nil { // root session or orphaned child
		return escalation.ParentRef{Kind: escalation.NodeNone}, nil
	}
	return escalation.ParentRef{
		SessionID: parent.SessionID,
		Role:      parent.Name,
		Kind:      nodeKindFor(parent),
	}, nil
}

// vroomTrioNames are the three apex VROOM supervisors a programmatic confer
// fans out to. They are the well-known session names the mesh always spawns;
// the registry resolves them to whatever live session currently holds each role.
var vroomTrioNames = []string{
	"vroom-meta-orchestrator",
	"vroom-orchestrator",
	"vroom-overseer",
}

// doltVROOMRegistry implements escalation.VROOMRegistry over AGM's session
// store: it resolves the three well-known VROOM names to their current live
// session IDs. Members not currently resolvable are skipped (a confer runs over
// whoever is live); if none resolve the engine falls back to single-node
// conferring. This is the "registry of the live VROOM session IDs" the
// programmatic confer needs (ce-es7z).
type doltVROOMRegistry struct{ adapter *dolt.Adapter }

func (r *doltVROOMRegistry) Members(_ context.Context) ([]escalation.ParentRef, error) {
	out := make([]escalation.ParentRef, 0, len(vroomTrioNames))
	for _, name := range vroomTrioNames {
		m, err := r.adapter.ResolveIdentifier(name)
		if err != nil || m == nil {
			continue // trio member not currently live — skip it
		}
		out = append(out, escalation.ParentRef{
			SessionID: m.SessionID, Role: m.Name, Kind: nodeKindFor(m),
		})
	}
	return out, nil
}

// agmMessenger delivers escalation notices by shelling out to `agm send`. It is
// best-effort: a delivery failure does not fail the escalation, because the
// Store + event log are the source of truth and supervisors also poll via
// `agm escalate list`.
type agmMessenger struct{}

func (m *agmMessenger) Deliver(ctx context.Context, to string, msg escalation.EscalationMessage) error {
	self, err := os.Executable()
	if err != nil {
		self = "agm"
	}
	prompt := renderDelivery(msg)
	cmd := exec.CommandContext(ctx, self, "send", "msg", to,
		"--sender", "escalation", "--prompt", prompt)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Surface as a warning, not a hard failure.
		fmt.Fprintf(os.Stderr, "escalate: warning: delivery to %s failed (escalation is still recorded): %v: %s\n", to, err, string(out))
	}
	return nil
}

func renderDelivery(msg escalation.EscalationMessage) string {
	if msg.Resolved {
		return fmt.Sprintf("[ESCALATION ANSWERED] id=%s\nYour question: %s\nAnswer (from %s): %s",
			msg.EscalationID, msg.Question, msg.FromRole, msg.Answer)
	}
	human := ""
	if msg.MustReachHuman {
		human = "\n⚠ This is flagged must-reach-human: you may NOT answer it — `agm escalate forward " + msg.EscalationID + " --note \"<recommendation>\"` so the human decides."
	}
	note := ""
	if msg.Note != "" {
		note = "\nForwarder note: " + msg.Note
	}
	return fmt.Sprintf("[ESCALATION %s from %s] id=%s\nQuestion: %s\nContext: %s%s%s\n\nIf you can answer confidently: agm escalate answer %s \"<answer>\"\nOtherwise: agm escalate forward %s --note \"<why>\"",
		msg.Kind, msg.FromRole, msg.EscalationID, msg.Question, msg.Context, note, human, msg.EscalationID, msg.EscalationID)
}

// dispatchHuman surfaces an escalation to the human by appending to the VROOM
// Dispatch trail (~/.agm/vroom/dispatch-trail.jsonl) that the Dispatch daemon
// and the human already monitor.
type dispatchHuman struct{ home string }

func (h *dispatchHuman) ToHuman(ctx context.Context, e *escalation.Escalation) error {
	trail, err := decisiontrail.OpenJSONL(filepath.Join(h.home, ".agm", "vroom", "dispatch-trail.jsonl"))
	if err != nil {
		return err
	}
	defer trail.Close()
	return trail.Append(ctx, decisiontrail.Record{
		Role: "dispatch",
		Kind: "dispatch.escalation.to_human",
		Payload: map[string]any{
			"escalation_id":    e.ID,
			"kind":             string(e.Kind),
			"question":         e.Question,
			"origin_session":   e.OriginSessionID,
			"chain":            e.Chain,
			"must_reach_human": e.MustReachHuman,
			"topic":            e.Topic,
			"answer_with":      "agm escalate answer " + e.ID + " --by human \"<decision>\"",
		},
	})
}

// --- command handlers ------------------------------------------------------

func runEscalateAsk(cmd *cobra.Command, args []string) error {
	question := joinArgs(args)
	env, cleanup, err := newEscalationEnv(vroomEntryFromFlag(cmd, escVROOM))
	if err != nil {
		return err
	}
	defer cleanup()

	originID, originRole, err := env.resolveSessionID(escSession)
	if err != nil {
		return err
	}
	if escRole != "" {
		originRole = escRole
	}

	ctx := cmd.Context()
	esc, err := env.eng.Raise(ctx, escalation.RaiseRequest{
		OriginSessionID: originID,
		OriginRole:      originRole,
		Kind:            escalation.Kind(escKind),
		Mode:            escalation.Mode(escMode),
		Question:        question,
		Context:         escContext,
	})
	if err != nil {
		return err
	}

	fmt.Printf("escalation %s — %s\n", esc.ID, esc.Phase)
	if esc.Phase == escalation.PhaseAutoResolved {
		fmt.Printf("auto-answer: %s\n", esc.Answer)
		return nil
	}
	fmt.Printf("routed to: %s (%s)\n", esc.CurrentSessionID, esc.CurrentRole)

	if escalation.Mode(escMode) == escalation.ModeBlocking {
		fmt.Printf("waiting up to %s for an answer…\n", escTimeout)
		final, werr := env.eng.Await(ctx, esc.ID, escTimeout, 2*time.Second)
		if werr != nil {
			return werr
		}
		fmt.Printf("answer (from %s): %s\n", final.AnsweredByRole, final.Answer)
	}
	return nil
}

func runEscalateAnswer(cmd *cobra.Command, args []string) error {
	id := args[0]
	answer := joinArgs(args[1:])
	env, cleanup, err := newEscalationEnv(vroomEntryDisabled)
	if err != nil {
		return err
	}
	defer cleanup()

	byID, byRole, err := env.resolveSessionID(escBy)
	if err != nil {
		return err
	}
	if escRole != "" {
		byRole = escRole
	}
	esc, err := env.eng.Answer(cmd.Context(), id, byID, byRole, answer)
	if err != nil {
		return err
	}
	fmt.Printf("escalation %s — %s (answered by %s)\n", esc.ID, esc.Phase, esc.AnsweredByRole)
	return nil
}

func runEscalateForward(cmd *cobra.Command, args []string) error {
	id := args[0]
	env, cleanup, err := newEscalationEnv(vroomEntryFromFlag(cmd, escVROOM))
	if err != nil {
		return err
	}
	defer cleanup()

	fromID, fromRole, err := env.resolveSessionID(escSession)
	if err != nil {
		return err
	}
	if escRole != "" {
		fromRole = escRole
	}
	esc, err := env.eng.Forward(cmd.Context(), id, fromID, fromRole, escNote)
	if err != nil {
		return err
	}
	fmt.Printf("escalation %s — %s", esc.ID, esc.Phase)
	if esc.Phase == escalation.PhaseDispatchedToHuman {
		fmt.Printf(" (surfaced to the human via Dispatch)")
	} else {
		fmt.Printf(" → %s (%s)", esc.CurrentSessionID, esc.CurrentRole)
	}
	fmt.Println()
	return nil
}

func runEscalateVote(cmd *cobra.Command, args []string) error {
	id := args[0]
	vote := escalation.Vote(args[1])
	env, cleanup, err := newEscalationEnv(vroomEntryDisabled)
	if err != nil {
		return err
	}
	defer cleanup()

	byID, byRole, err := env.resolveSessionID(escBy)
	if err != nil {
		return err
	}
	if escRole != "" {
		byRole = escRole
	}
	esc, err := env.eng.CastVote(cmd.Context(), id, byID, byRole, vote, escAnswer, escNote)
	if err != nil {
		return err
	}

	//nolint:exhaustive // only the resolved phases get bespoke text; the rest fall to default.
	switch esc.Phase {
	case escalation.PhaseAnswered:
		fmt.Printf("escalation %s — answered by the trio (quorum reached)\nanswer: %s\n", esc.ID, esc.Answer)
	case escalation.PhaseDispatchedToHuman:
		fmt.Printf("escalation %s — no quorum; surfaced to the human via Dispatch\n", esc.ID)
	default:
		voted, total := 0, 0
		if esc.Confer != nil {
			voted, total = len(esc.Confer.Ballots), len(esc.Confer.Members)
			fmt.Printf("escalation %s — vote recorded (%d/%d cast, quorum %d); still conferring\n",
				esc.ID, voted, total, esc.Confer.Quorum)
		} else {
			fmt.Printf("escalation %s — %s\n", esc.ID, esc.Phase)
		}
	}
	return nil
}

func runEscalateList(cmd *cobra.Command, _ []string) error {
	env, cleanup, err := newEscalationEnv(vroomEntryDisabled)
	if err != nil {
		return err
	}
	defer cleanup()

	f := escalation.Filter{Pending: escPending}
	if escListMine {
		selfID, _, derr := env.resolveSessionID("")
		if derr != nil {
			return derr
		}
		f.CurrentSessionID = selfID
		f.Pending = true
	}
	list, err := env.eng.List(cmd.Context(), f)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("no escalations")
		return nil
	}
	for _, e := range list {
		fmt.Printf("%s  %-18s  %-9s  held-by=%s  %q\n",
			e.ID, e.Phase, e.Kind, e.CurrentSessionID, truncate(e.Question, 60))
	}
	return nil
}

func runEscalateShow(cmd *cobra.Command, args []string) error {
	env, cleanup, err := newEscalationEnv(vroomEntryDisabled)
	if err != nil {
		return err
	}
	defer cleanup()

	esc, err := env.eng.Get(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return printJSON(esc)
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}
