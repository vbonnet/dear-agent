# ADR-021: Verifier Role

**Status:** Accepted
**Date:** 2026-04-04
**Context:** Defining the Verifier role in the VROOM architecture (see [ADR-020](ADR-020-vroom-architecture-overview.md))

## Problem

Worker sessions self-report completion, but self-reported completion is the
highest-risk failure mode in the system (see orchestrator-mission.md § False
Completion Prevention Checklist). Quality gates exist (`/engram:bow`,
`/agm:audit-completion`, SHA verification) but they are scattered across
documents and enforced inconsistently. There is no single role whose sole
responsibility is to say "this output is acceptable" or "this output violates
our values and invariants."

## Decision

The **Verifier** is the VROOM role responsible for validating that all outputs
conform to project values (VALUES.md) and system invariants. It acts as a
**hostile auditor** — it assumes outputs are wrong until proven otherwise.

### Single Responsibility

The Verifier answers one question: **"Does this output meet our standards?"**

It does NOT:
- Decide what work to do (Requester)
- Dispatch or manage sessions (Orchestrator)
- Detect cross-session anomalies (Overseer)
- Make system-level governance decisions (Meta-Orchestrator)

### Interface Contract

```
INPUT:
  - work_item: { id, acceptance_criteria, task_type }
  - output: { commit_shas[], session_id, completion_report }

OUTPUT:
  - verdict: PASS | FAIL
  - findings: [{ severity: CRITICAL | WARNING, description, evidence }]
  - values_check: { compliant: bool, violations: string[] }
```

### VALUES.md Integration

The Verifier holds VALUES.md as its primary reference document. Every
verification includes a values compliance check against the four core values:

1. **Shared Knowledge** — Does the output encode reusable knowledge, or is it
   a one-off that will need to be repeated?
2. **Quality Without Compromise** — Does the output reduce waste without
   sacrificing value? Are tests present and meaningful?
3. **Transparent Trust** — Is the output auditable? Are changes visible in
   git with clear commit messages?
4. **Rising Tide** — Does the output benefit the broader system, or does it
   create isolated knowledge?

Values compliance is **P1** in the lexicographic evaluation order (see
[ADR-020](ADR-020-vroom-architecture-overview.md) § Lexicographic Evaluation
Order). A values violation blocks acceptance regardless of all other criteria.

### Deterministic P1 Checks

The following checks are deterministic (no LLM required) and are P1 — they
block acceptance unconditionally:

| # | Check | Implementation | Failure Mode |
|---|-------|---------------|--------------|
| 1 | Commit SHAs exist | `git rev-parse --verify <sha>` | FAIL: phantom commits |
| 2 | Code changes have tests | `*.go` changed → `*_test.go` changed | FAIL: untested code |
| 3 | No deferred markers | grep for "deferred", "TODO test", "skip test" in commits | FAIL: false completion |
| 4 | Code files changed for code tasks | Diff includes non-`.md` files | FAIL: docs-only for code work |
| 5 | Acceptance criteria addressed | Each criterion mapped to evidence | FAIL: requirements dropped |
| 6 | No bypass flags | grep for `--force`, `--no-verify`, `--skip` in session log | FAIL: safety bypass |
| 7 | Session duration plausible | Duration > 5 minutes for non-trivial tasks | WARN: suspiciously fast |

These checks map directly to the existing False Completion Prevention Checklist
in orchestrator-mission.md. The Verifier formalizes them as a role boundary.

### Verification Modes

| Mode | When Used | Checks |
|------|-----------|--------|
| **Gate** | Before archive (synchronous) | All P1 deterministic checks |
| **Audit** | After merge (asynchronous) | Repo-wide invariant checks, values compliance |
| **Spot** | On suspicion (triggered by Overseer) | Targeted re-verification of specific claims |

### Existing Implementation Mapping

| Verifier Function | Current Implementation |
|-------------------|----------------------|
| Gate verification | `/engram:bow` (completion validation) |
| Audit verification | `/agm:audit-completion` (11-point quality audit) |
| SHA verification | orchestrator deterministic phase (`git rev-parse`) |
| Values check | Not yet implemented (new capability) |
| Spot verification | Manual (triggered by meta-orchestrator observation) |

### Decision Trail Integration

Every Verifier verdict produces a decision record (see [ADR-020](ADR-020-vroom-architecture-overview.md)
§ Decision Trail):

```json
{
  "role": "verifier",
  "decision_type": "gate",
  "evaluation": {
    "p1_values": "PASS",
    "p1_deterministic": ["sha_check:PASS", "test_check:PASS", "deferred_check:PASS"],
    "overall": "PASS"
  },
  "outcome": "accept",
  "rationale": "All P1 checks passed. 0 CRITICAL, 1 WARNING (session duration 6min, borderline)."
}
```

## Alternatives Considered

1. **Inline verification in Orchestrator:** Current approach — works but conflates
   dispatch and validation responsibilities. The Orchestrator has incentive to
   "pass" work to clear its queue, creating a conflict of interest.
2. **Human-only verification:** Doesn't scale. The Verifier handles deterministic
   checks; humans handle judgment calls escalated by the Overseer.
3. **Post-hoc-only verification (no gates):** Deferred enforcement works for low-cost
   fixes, but false completions that merge to main are expensive to remediate.
   Gate verification is justified by the cost formula (f * C_check << C_fix for merges).

## Consequences

- The Verifier is the single authority on output acceptability — no other role
  can override a FAIL verdict (the Meta-Orchestrator can request re-verification
  but cannot force acceptance)
- VALUES.md becomes a load-bearing document — changes to values require Verifier
  rule updates
- Adding a values compliance check introduces a new verification dimension beyond
  existing mechanical checks
- The Verifier role may be implemented as a combination of hooks (deterministic P1),
  LLM session (values check), and tooling (`/engram:bow`, `/agm:audit-completion`)

## Cross-References

- [ADR-020: VROOM Architecture Overview](ADR-020-vroom-architecture-overview.md)
- [ADR-024: Overseer Role](ADR-024-overseer-role.md) — triggers spot verification
- [Orchestrator Mission § False Completion Prevention](../orchestrator-mission.md)
- [Orchestrator Mission § Quality Gates](../orchestrator-mission.md)
- [DEAR Protocol § Enforce](../DEAR-PROTOCOL.md)
