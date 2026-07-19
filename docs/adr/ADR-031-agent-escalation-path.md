<!-- Last audited at: 2026-06-15 -->

# ADR-031: Agent Escalation Path (No Bypass Flags)

**Status:** Accepted  
**Date:** 2026-06-15  
**Supersedes:** The `--emergency` hatch in `safe-pr` and the `--skip-bot-review` flag in `safe-merge`

---

## Context

Wrappers like `safe-pr` and `safe-merge` enforce the AGENTS.md guarded-delivery
policy by construction. Their original designs included bypass flags
(`--emergency`, `--skip-bot-review`) that allowed an agent to skip the gate
when "necessary." In practice, these bypasses:

1. **Erode the gate.** A gate with a one-keystroke bypass is not a gate — it is
   a suggestion. Agents under time pressure will reach for the bypass rather
   than resolve the underlying condition.

2. **Hide process gaps.** Every use of `--emergency` is a signal that either the
   tooling is broken or the policy is too rigid. Neither is visible when the
   bypass is silent.

3. **Leave no approved path.** An agent that bypasses a gate today leaves the
   next agent with no record of why the bypass was acceptable, so the next
   agent bypasses again.

The correct response when a gate cannot be cleared is to **escalate**, not to
bypass. Escalation makes the gap visible and routes it to a human who can
authorize the exceptional action, trigger a DEAR retro, and create an approved
path so the next agent does not hit the same wall.

---

## Decision

### 1. No bypass flags on our tools

`--emergency` is removed from `safe-pr`. `--skip-bot-review` / `--skip-bot-review-reason`
are removed from `safe-merge`. No replacement flags are provided. The
`internal/override` package's existing `--force`/`--skip-*` gates (which already
require `--reason` and route through the override guard) are out of scope — they
are not raw bypasses.

Every future tool built under the guarded-delivery policy MUST NOT include a bypass
flag. If the situation is genuinely exceptional, the escalation path (below) is
the right mechanism.

### 2. Hook enforcement for removed flags

`.claude/hooks/pretool-bypass-guard` exits 2 with positive guidance when it
detects:

- `safe-pr --emergency`
- `safe-merge --skip-bot-review`
- `git commit --no-verify`
- `git push --force` (bare; `--force-with-lease` remains approved)

The hook fires on every Bash call in this project. It fails open (exit 0 on
any parse error) so it cannot wedge a session for an unrelated reason.

### 3. Escalation path (future work — bead ce-m3ya)

When an agent hits a blocked action and no approved path exists, the correct
flow is:

```
agm escalate --action "<what the agent needs to do>" --reason "<why it cannot use the normal path>"
```

`agm escalate` (not yet implemented; tracked in bead ce-m3ya) will:

1. Create a bead in `~/beads/context-engine` tagged `escalation` with the
   action, reason, and originating session ID.
2. Route the bead up the VROOM supervisory chain: agent → orchestrator → overseer.
3. In parallel, fire a research agent to propose an approved path (a new
   `safe-*` wrapper, a hook exception, a policy revision).
4. Surface a human-readable notification: "Agent X needs to do Y. No approved
   path exists. Research suggests Z. Approve Y / Reject / Create approved path."

Every approval becomes a mini DEAR retro entry and, where appropriate, a new
approved path committed to the codebase — so the class of situation is resolved
for all future agents, not just the current one (the AGENTS.md permission-block rule).

### 4. Error message contract

When a gate fails and no bypass exists, the error message MUST:

- State what the agent was trying to do
- Explain why it was blocked
- Give the approved alternative (if one exists)
- Point to `agm escalate` as the fallback

Example (from `safe-pr`):

```
no wayfinder session given: pass --wayfinder <project-dir> or set
WAYFINDER_PROJECT_DIR to the directory containing WAYFINDER-STATUS.md.
Every PR must carry a wayfinder trace. If no approved path exists, escalate via:
  agm escalate --action "create PR" --reason "<why no session exists>"
```

---

## Consequences

**Positive:**

- Safety gates are now unconditional; no bypass path exists inside the tools.
- Every situation where an agent is genuinely blocked becomes a visible signal
  (a bead, a retro, a resolved approved path).
- The escalation path turns one-off human decisions into durable policy.

**Negative / trade-offs:**

- Until `agm escalate` is implemented (ce-m3ya), a blocked agent has no
  machine-readable path to surface the gap. In the interim, agents must file a
  bead manually (`bd --db ~/beads/context-engine/.beads create ...`) and
  report the block in their end-of-run summary.
- The bot-review gate in `safe-merge` was occasionally hit due to Gemini quota
  exhaustion. Without `--skip-bot-review`, the only options are `--watch` (poll
  until the bot posts) or escalation. This is intentional: quota exhaustion is a
  systemic gap, not an individual exception.

---

## Alternatives considered

**Keep `--emergency` but route it through `internal/override`.**  
Rejected. The override guard requires a `--reason` and audits the bypass, but
it still allows the bypass to happen. The audit trail is useful; the bypass
capability is not. Routing through override would preserve the gate-eroding
dynamic while adding auditing theater.

**Rate-limit bypass flags (e.g., max 3 uses per week).**  
Rejected. Rate-limiting treats the symptom without addressing the cause. The
goal is not to reduce bypasses; it is to eliminate the class by creating
approved paths.

**Keep `--skip-bot-review` for CI environments where the bot is disabled.**  
Rejected. CI environments should configure `safe-merge` via a different
mechanism (e.g., an env var that disables the bot-review gate globally when
the bot is known to be absent). A flag that any agent can pass is not the same
as a configuration that an operator sets.
