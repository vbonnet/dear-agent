# Design Spike: Repeat-Back Protocol for the VROOM Mesh

**Bead:** ce-04cv · **Status:** Investigation only (no implementation) · **Date:** 2026-06-20

## Problem

VROOM dispatch instructions are free-form natural-language messages
(`agm send msg "worker-<bead>" --prompt "You are a worker session assigned to
bead <id>…"`). The orchestrator emits the prompt; the worker silently begins
acting on its *interpretation* of it. There is no point at which the worker's
understanding is checked against the dispatcher's intent before irreversible
work (worktree, commits, PR, bead-close) begins. This is the classic
single-acknowledgement gap that readback protocols exist to close.

## 1. Pattern Survey

**Aviation (ATC readback/hearback).** Controller issues a clearance; pilot reads
back the *salient* parameters (callsign, altitude, heading, runway); controller
listens for errors and corrects ("hearback"). Mandatory items are constrained to
the few that kill people if wrong. Failure modes prevented: *mishearing*
(callsign confusion), *expectation bias* (hearing the clearance you expected),
and *transposition* (FL210 vs FL120).

**Military (radio confirmation / "say again").** Full readback of orders for
fire/movement; explicit "WILCO" (will comply) vs "ROGER" (received, not a
commitment). Separates *receipt* from *commitment* — a distinction VROOM
currently collapses.

**Human factors (closed-loop communication, e.g. surgical/CRM teams).** Sender
states → receiver repeats back → sender confirms ("check-back"). The third step
(sender confirms) is what makes the loop *closed*; a repeat-back nobody verifies
is theater.

Common thread: readback covers **only the high-consequence, drift-prone fields**,
and the loop is closed by the *originator*, not the receiver.

## 2. Mesh-Specific Risk Analysis

Ranked by drift consequence × ambiguity:

| Message field | Drift risk | Why |
|---|---|---|
| **DoD / closure criteria** | **High** | "Done = PR MERGED" is repeatedly violated (ce-6f1b, ce-mcw2, ce-1onr closed against unmerged PRs). A worker that misreads this closes prematurely — the exact failure the retro found. |
| **Bead identity / scope** | **High** | Worker acts on the wrong bead or a broader scope than intended; cheap to confirm, expensive to undo (stray commits/PRs). |
| **Escalation triggers** | Medium | "Escalate vs decide yourself" boundary (ADR-032) is judgment-laden; drift causes either silent wrong decisions or escalation spam. |
| **Workflow constraint** (wayfinder, not raw exec) | Medium | Easy to ignore under time pressure; low immediate signal. |
| **Dependency provenance** | Low | Already gated mechanically at the orchestrator (DoD dispatch gate), not reliant on worker understanding. |

Escalation *answers* delivered back to workers are a second, symmetric drift
surface: a one-line answer to a forwarded question is interpreted without
confirmation.

## 3. Implementation Options

- **A. Structured ACK fields in the dispatch prompt.** Worker's first action is
  to emit a fixed JSON readback to the trail: `{bead, scope_summary,
  dod_understood, closure_gate, will_comply}`. Cheap, async, greppable. No
  orchestrator round-trip — closer to military "WILCO" than true hearback.
- **B. Worker repeat-back before execution (closed loop).** Worker sends a
  paraphrase of scope + DoD back to the orchestrator and *waits* for confirm
  before creating the worktree. True closed loop, but adds a tick of latency per
  worker and load on the orchestrator's already-busy dispatch loop.
- **C. Orchestrator confirmation step (hearback only).** Orchestrator re-reads
  the worker's first trail entry next tick and corrects if it diverges. No worker
  blocking; catches drift one tick later (cheap, but reactive).

## 4. Recommendation

**Adopt Option A (structured ACK) scoped to the two high-risk fields only —
bead/scope and the DoD closure gate — with Option C as the verification half.**

Rationale, against the survey's lesson that readback must (a) cover only
high-consequence fields and (b) be closed by the originator:

- Full closed-loop blocking (B) buys little: the dispatch prompt already states
  the DoD verbatim and the worker runs unattended, so a blocking handshake mostly
  adds latency and orchestrator load without a human in the loop to benefit.
- A bare ACK with nobody reading it (A alone) is the "theater" failure — hence
  pairing it with C. The orchestrator already walks `worker-*` sessions every
  tick (Step 7); having it diff the worker's ACK against the dispatched scope/DoD
  is a near-zero-cost addition to an existing loop, and it closes the loop the way
  hearback does — at the originator.

**Concrete shape.** Add to the dispatch prompt: *"Before creating your worktree,
append one line to `~/.agm/vroom/trail.jsonl` with `kind:
'worker.dispatch.ack'` and payload `{bead, scope, closure_gate:'PR-merged'}`."*
Orchestrator Step 7 greps for the matching ack; on mismatch or absence past one
tick, it re-sends a one-line correction. **Cost:** ~3 lines of worker prompt, one
grep in an existing orchestrator step, one new trail kind. No new infrastructure,
no blocking, fully auditable.

**Do not** apply readback to every field — that reproduces aviation's
over-readback problem (cognitive load drowning the salient items). Constrain it
to the two fields that actually cause irreversible harm when they drift.
