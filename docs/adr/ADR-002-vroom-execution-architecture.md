# ADR-002: VROOM Execution Architecture

Status: Accepted (2026-05-17; amended 2026-07-18)

**Supersedes** `agm/docs/adr/ADR-020`…`ADR-025`, which described an
inaccurate five-role mesh (Verifier/Requester/Orchestrator/Overseer/
Meta-Orchestrator) governed by a five-level lexicographic value evaluator,
and were misfiled under `agm/` (VROOM is above AGM, not an AGM-internal
concept). Those superseded files were removed; this is the retained decision.

VROOM is the supervisory mesh **above AGM** that governs how agents do
work. Three supervisors plus a per-task triad; no standing Verifier or
Requester roles; no value function over a five-level order.

### Three supervisors, each in a constant loop

| Supervisor | Analogy | Owns | Secondary | Tertiary |
|---|---|---|---|---|
| **Meta-Orchestrator** | CTO | Roadmap, prioritization, technology consistency, anti-duplication | Overseer | Orchestrator |
| **Orchestrator** | COO | Work enqueue/dequeue, worker monitoring, steady progress | Meta-Orchestrator | Overseer |
| **Overseer** | CRO | Resource usage, leak detection, session cleanup | Orchestrator | Meta-Orchestrator |

### Canonical topology source

`pkg/vroom/supervisor` is the single code source for the exactly three
supervisor identities, compact aliases, roles, and Primary/Tertiary peer
relationships. CLI and runtime surfaces resolve topology through that package;
they do not maintain parallel identity or peer tables.

The topology is intentionally separate from deployment policy. A launcher such
as `cmd/vroom-dispatch` still owns harness, model, skill, tick interval, and tick
prompt selection for each role. AGM owns heartbeat persistence and liveness
checks. Both consume the canonical member identity and peer graph without
moving those adapter responsibilities into the topology package.

Three load-bearing invariants:

- **Typed permission recovery.** Every loop checks peers before role work.
  `agm scan --cross-check` may auto-approve only actions accepted by its RBAC
  classifier. A prose prompt may report, defer, reject, or escalate a remaining
  prompt; it may never add a manual approval fallback.
- **Beads are the roadmap.** Ready Beads, live AGM sessions, and open PRs are the
  operational sources of truth. VROOM does not maintain roadmap, dispatch,
  deploy, or prompt-file projections. The Meta-Orchestrator owns priority and
  scope decisions on the Beads records themselves.
- **Delivery is end to end.** A delivery bead remains open until its change is
  merged, deployed when applicable, and verified. PR creation is an intermediate
  state, not completion.

### Alignment and ownership sources

MISSION.md is canonical for project purpose and the VROOM/AGM ownership
boundary. `VALUES.md` and `GOALS.md` are subordinate, qualitative guidance. No
runtime evaluates a values hierarchy or weighted goal function; presenting
those documents as an executable optimization model would reintroduce the
architecture this ADR supersedes.

The ownership boundary follows the framework/tool split: VROOM decides what to
prioritize and dispatch, supervises the work, and verifies its output. AGM owns
the session lifecycle mechanisms VROOM invokes and observes. AGM session state
is evidence for a VROOM decision, not the decision itself.

### Per-task triad

Every task is owned by three agents: **Primary** (does it), **Secondary**
(verifies the output and ensures it gets done), **Tertiary** (keeps Primary
and Secondary unstuck). Verification is a Secondary *responsibility*, not
a standing role — which is why the old "Verifier" and "Requester" roles
are removed.

### Decision trail and code-vs-spec gap

Consequential VROOM decisions land on an append-only decision trail.
⚠️ `pkg/vroom/vroom/topics.go` still encodes the *superseded* role enum (a
`vroom.decision.evaluated` "Verifier" topic). Renaming exported constants
is a hard-to-reverse API change and is **out of scope** here; it is tracked
as a CONTEXT.md collision and a follow-up.

### Alternatives rejected

- **Keep the five-role model.** Does not match the intended architecture;
  invents Verifier/Requester as standing roles; bolts on an evaluator the
  system does not run.
- **Single orchestrator, no peer supervisors.** A lone supervisor has no
  one to unstick it and no separation between "what to do next", "keep
  work flowing", and "is the system healthy". Mutual-unblock requires ≥3.
- **Flat peer mesh.** No single backlog-priority authority means duplicate work.
- **One ADR per role.** The decision is the mesh shape; per-role text is
  vocabulary and belongs in CONTEXT.md, not five drifting ADRs.
- **Keep identity and peer tables in each adapter.** Synchronization tests only
  detect drift after it is introduced. A pure shared topology lets dispatch,
  AGM heartbeat parsing, and future surfaces use the same identities and graph
  while keeping their execution policies independent.

### Vocabulary lives in [/CONTEXT.md](../../CONTEXT.md), not here

CONTEXT.md is normative for vocabulary, MISSION.md for project purpose and
ownership, and this ADR for the architecture decision and its trade-offs.
Wayfinder plans → VROOM executes → AGM is the tool VROOM drives → DEAR
(Define/Execute/Audit/Retro) is the per-task retrospective loop. See
[/CONTEXT.md § The Four Frameworks](../../CONTEXT.md#the-four-frameworks--and-how-they-relate).
The "VROOM" backronym is formally retired; it is a proper name.

### Cross-references

- [/CONTEXT.md](../../CONTEXT.md), [ADR-001](ADR-001-monorepo-consolidation.md)
- [docs/alignment/MISSION.md](../alignment/MISSION.md) (canonical purpose and
  ownership),
  [VALUES.md](../alignment/VALUES.md),
  [VISION.md](../alignment/VISION.md),
  [GOALS.md](../alignment/GOALS.md)
- Supersedes removed `agm/docs/adr/ADR-020`…`ADR-025` records.
