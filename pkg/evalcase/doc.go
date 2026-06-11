// Package evalcase implements the production trace → eval case pipeline.
//
// This is the "close the loop" half of the telemetry/eval flywheel described in
// the deep-research telemetry report (Arize/Braintrust 2026): production traces
// that fail online scoring or end in an error/stall are converted into eval
// cases, the eval suite grows from real behaviour, and CI can later block merges
// that drop quality. It builds directly on the four-pillar trace schema from
// pkg/agenttrace (tool-call, reasoning, state-transition, memory spans) and the
// eval-as-span-attribute infra in internal/telemetry.
//
// The flow:
//
//	completed traces ─► Classify ─► (problematic?) ─► Extract ─► Store
//	   (DEAR Audit)      verdict        yes            EvalCase    evals/
//
//  1. A Trace is the serializable, pillar-tagged view of one completed agent run
//     that the DEAR Audit phase has on hand — its spans, terminal outcome, and
//     any eval scores. Live OTel spans emitted by pkg/agenttrace can be turned
//     into Traces with FromReadOnlySpans.
//  2. A ClassifierConfig inspects a Trace and decides whether it is problematic
//     (an errored tool call, an error/stall outcome, a low eval score, a stale
//     memory read, …) and what the failure class is.
//  3. Extract turns a problematic Trace plus its Verdict into an EvalCase — a
//     self-contained regression fixture carrying the task, the expected vs.
//     actual behaviour, the failure classification, and span excerpts from the
//     four pillars.
//  4. A FileStore writes EvalCases to a discoverable evals/ dataset, one JSON
//     file per case, idempotently (a case is never silently overwritten).
//
// Pipeline ties the four steps together for a batch of traces.
package evalcase
