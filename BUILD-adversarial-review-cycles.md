---
phase: BUILD
phase_name: BUILD
wayfinder_session_id: feedback-loop-event-architecture
created_at: 2026-09-02T23:33:00-07:00
---

# Build

Two full adversarial review-and-revision cycles on the design, following the
research-pipeline skill's cross-model discipline (never let one model mark its own
homework).

**Round one**: Fable and Opus 5 (substituting for an unidentifiable "Astra," later
corrected by the user to GPT 5.6 Sol). Both returned BLOCK. Key finding both converged
on independently: `pkg/eventbus.LocalBus` is documented as in-process only and does
not cross the process boundaries the design's own flows require, invalidating the
original cross-process transport proposal. Also found: several "EXISTING" claims were
actually unwired code, a real live bug (Wayfinder emits `wayfinder.phase.started` but
the trigger registry's exact-match lookup expects the bare `phase.started`), and
roughly 80 em dashes violating the explicit "no em dashes" instruction. Fixed by
rerouting cross-process delivery through `agm-bus` and proposing to extend
`pkg/workflow` instead of a new package.

**Round two**: GPT-5.6 Sol (`codex exec -m gpt-5.6-sol`) and Fable 5.1
(`claude -p --model claude-fable-5-1`), the correct pair per the user's explicit
correction. Both again returned BLOCK, with sharper and largely convergent findings:
`agm-bus` has no publish/subscribe frame type at all, and its `EmitEvent` broadcast
convenience is rejected by the server's own frame validation; `pkg/trigger`, outside
the `agm/` import prefix, cannot import `agm/internal/bus` under Go's internal-package
visibility rule (the repo already has the answer, `agm/workflowbus/bridge.go`); a
Mermaid syntax bug (a semicolon inside a `Note over` string); Flow B bypassing the
research-pipeline skill's real Stage 4 output (bead decomposition) in favor of a raw
worker spawn; `pkg/workflow` already having non-CLI invocation surfaces; and a sharp
conceptual critique that most of the flows shown are event-triggered choreography, not
closed feedback loops. Every surprising claim from both rounds was independently
verified against the real source before being accepted. Both rounds' fixes are folded
into the current, final revision of the design documents; see
[docs/architecture/feedback-loop-pipelines.md](docs/architecture/feedback-loop-pipelines.md).
