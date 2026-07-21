# Decision: Adopt the Spike Pattern Across the VROOM Mesh

**Status:** Accepted
**Date:** 2026-06-20
**Bead:** ce-ynyb
**Companions:** [[ce-228u]] first spike · [[ce-t8kn]] output schema · [[ce-4h06]] implementation-requirements gate · [[ce-90si]] confidence gate · [[ce-kky0]] recursive decomposition

---

## Context

A **spike** — a time-boxed investigation whose sole deliverable is enough signal
to write implementation requirements for a follow-up bead — has emerged
organically across recent work (ce-htwo, ce-90si, ce-kky0, ce-4h06, ce-t8kn,
ce-228u, ce-04cv). Each produced a design doc and stopped before implementation.
The pattern works, but it is informal: nothing says *when* a spike is required,
how it is labeled, or how it hands off. This decision formalizes it.

The pattern is well-known from Scrum/XP. Our adaptation: a spike feeds grounded
implementation requirements and a cost/benefit decision; the implementation bead is created
separately and prioritized on its own merits. `bd` already ships a first-class
`spike` type ("timeboxed investigation to reduce uncertainty before committing
to a story"), so no tooling change is needed.

---

## Decision

### 1. The rule

**Any bead carrying significant design or architecture uncertainty MUST be
preceded by a spike.** "Significant uncertainty" means implementation requirements cannot be
written with testable acceptance criteria without first investigating — multiple
viable approaches, unknown blast radius, or an unproven dependency. If a worker
cannot state what "done" looks like, the work needs a spike first.

### 2. Bead creation protocol

Workers and supervisors **create the spike bead proactively** — do not block on
dispatch. A spike is cheap, reversible, and self-limiting (§5), so the cost of a
spurious spike is low and the cost of skipping a needed one is high. The creator
files it with `bd ... create`, sets the rationale in the description, and notes
it in their end-of-run summary. Dispatch reprioritizes after the fact rather than
gatekeeping creation. Supervisors may *require* a spike on an existing bead by
splitting it.

### 3. Labels and type

Use the **built-in `--type spike`** (not `--type task`). Required labels:
`spike,investigation`. Add a domain label (`process`, `workflow-engine`, etc.)
for triage. We do **not** extend `bd` — the native type already exists and
queries (`bd list --type spike`) work today.

```
bd --db ~/beads/context-engine/.beads create \
  --type spike --labels spike,investigation,<domain> \
  --title "Spike: <question to resolve>"
```

### 4. Handoff convention

A spike completes via a **PR containing a single decision/design doc**
(`docs/design-*.md` or `docs/decision-*.md`). On merge, the spike worker:

1. Creates the implementation bead with `bd create`, linking back with a
   `[[ce-xxxx]]` reference in both directions.
2. Records the handoff as a **trail entry** on the spike bead naming the new
   impl bead ID and the confidence band ([[ce-90si]]).
3. Closes the spike bead only after the PR is **MERGED**.

The doc's final section MUST list **requirements for the implementation bead**
— this is the actual deliverable the handoff carries.

### 5. Scope limit

A spike is bounded on three axes:

- **Time-box:** one worker session. If investigation exceeds it, decompose into
  sub-spikes ([[ce-kky0]]) rather than extending.
- **Output:** one doc, **500–700 words**, single recommendation. No code changes.
- **Single deliverable:** signal for *one* downstream decision. A spike that
  spawns three unrelated implementation beads was scoped too wide.

### 6. Exceptions (no spike required)

- **Mechanical / low-uncertainty work** — typo fixes, dependency bumps, config,
  docs, rename refactors with a known shape.
- **Bugs with a known root cause** — the diagnosis *is* the investigation.
- **Work with an existing spike** — do not re-spike a question already answered;
  cite the prior doc.
- **Trivial features** where acceptance criteria are already writable.

When in doubt, the test is operational: *can you write testable implementation acceptance
criteria right now?* If yes, skip the spike; if no, spike first.

---

## Consequences

**Positive:** implementation beads start with grounded requirements; investigation
cost is paid in a cheap, isolated bead; the spike→impl chain is auditable via bead
links and trail entries; native `bd` tooling needs no change.

**Negative / trade-offs:** a two-bead chain adds latency for genuinely uncertain
work (intended — the alternative is a bounced implementation). Proactive creation
risks spike sprawl; the §5 scope limits and dispatch's after-the-fact reprioritization
are the controls. The "significant uncertainty" test is judgment-based; the implementation-readiness
operational test (§6) is the tie-breaker.

**Awareness:** this pattern propagates via AGENTS.md and worker bead templates so
new workers inherit it by default.
