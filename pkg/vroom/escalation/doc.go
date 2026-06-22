// Package escalation implements the "Escalate To Supervisor" mechanism for the
// VROOM mesh: an agent that cannot resolve a question or decision raises it to
// whoever spawned it, and the question walks up the spawn hierarchy until it is
// answered — terminating, for genuinely human-only decisions, at the human via
// Dispatch.
//
// # The chain
//
//	worker ──raise──▶ spawning supervisor ──forward──▶ … ──▶ VROOM trio (confer)
//	                       │ answer                              │ forward
//	                       ▼                                     ▼
//	                 originator notified                    Dispatch ──▶ human
//
// # What lives here vs. what wraps it
//
// This package is pure domain logic with interface seams; it imports no AGM
// internals (which are not importable from the root module anyway). The agm
// binary supplies the adapters:
//
//   - [SessionGraph] — "who spawned this session", backed by agm's Dolt
//     session hierarchy (GetParent).
//   - [Messenger] — deliver a message to another session, backed by `agm send`.
//   - [HumanDispatch] — surface to the human, backed by `agm ask` + pkg/notify.
//   - [Store] — mutable current state of in-flight escalations (a [FileStore]
//     so a worker and a supervisor in different processes share it).
//   - [Sink] — the append-only, analysis-ready event log (a [JSONLSink]).
//
// # Blocking vs async
//
// Only the asking worker may block: it calls [Engine.Raise] then [Engine.Await]
// (which polls the Store). Supervisors are never blocked — they drain
// escalations on their own tick via List/Answer/Forward. This is deliberate:
// "supervisors managing dozens of agents should not stop their loop for a single
// question."
//
// # The routing brain
//
// [Classifier] decides, before any routing, whether an escalation is obvious
// enough to auto-answer (e.g. "should I do the task you assigned me?" → yes,
// never reaching a human) or high-stakes enough that it must reach the human
// (e.g. a product decision, which no supervisor may answer below the human). The
// [DefaultClassifier] is deterministic and explainable, with a seam for a future
// model classifier — the same shape as internal/override's Judge.
//
// # The log
//
// Every state transition emits one [EscalationEvent] (one JSONL line). The
// schema is designed so three analyses fall out by grouping alone: incorrect or
// misaligned answers (outcome field, backfilled by the [Adjudicator] pass —
// see [Backfill] / [AnalyzeMisaligned]), frequent question types (group by
// question_hash / topic / kind — [AnalyzeFrequentQuestions]), and many agents
// asking the same question — a signal of missing prompt context — (group by
// question_hash, count distinct origin sessions — [AnalyzeManyAgents]). See
// [EscalationEvent].
//
// # The adjudicator
//
// [Adjudicator] renders the after-the-fact outcome verdict that fills the
// otherwise-empty Outcome/Misalignment columns. [DefaultAdjudicator] is the
// deterministic floor; [ClaudeAdjudicator] layers a model classifier on top —
// the same seam shape as internal/override's Judge. [Backfill] applies it over
// a log; the agm binary exposes it as `agm escalate adjudicate`.
package escalation
