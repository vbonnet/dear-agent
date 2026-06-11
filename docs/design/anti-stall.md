# Anti-Stall — Continuous Execution

> The single authoritative specification for **how an agent decides
> whether to keep working or to stop and ask a human**. Referenced from
> [`.claude/CLAUDE.md`](../../.claude/CLAUDE.md) so every agent inherits
> it. This is the *behavioural* contract; its infrastructure companions
> are listed under [See also](#see-also).

## TL;DR

**Keep going. The default is to continue.** An agent working a backlog,
a plan, or a multi-step task does not stop to ask "should I keep going?"
— it keeps going and lets the human interrupt if priorities changed.
Stopping to ask permission to continue is the failure mode this spec
exists to prevent.

Stalling is not the same as the legitimate stops in
[Agent Delegation Enforcement](../../.claude/CLAUDE.md#agent-delegation-enforcement-mandatory).
A stall is an agent halting on a question it could have answered itself,
or asking permission to do work it was already asked to do. The five
directives below say when to push through; the
[boundary section](#the-boundary--when-stopping-is-correct) says when
stopping is genuinely correct.

## Why this is a spec and not a one-off prompt line

This guidance has been given to agents in this repo **repeatedly** —
"never pause to ask 'should I keep going?'", "execute the backlog
continuously", "never ask whether to pick up backlog items, just do
them". A directive that has to be repeated is a directive that is not
landing. Per-prompt instructions protect only the prompt that contains
them: a sub-agent does not inherit the line, and the next task forgets
it. The fix is structural — the same lesson
[ADR-018](../adr/ADR-018-graceful-exit-framework-default.md) recorded
for the no-overfit guardrail and [AGENTS.why.md](../../AGENTS.why.md)
recorded for output routing. Publish the contract once, in a stable
place every agent reads, and reference it from `CLAUDE.md`.

## The five directives

### 1. Continue through backlogs without asking

When there is more work in the plan, the backlog, or the task list, do
the next item. Do **not** stop to confirm that you should. "I've
finished step 3, should I proceed to step 4?" is a stall — proceed to
step 4. "There are five more items in the backlog, want me to keep
going?" is a stall — keep going.

The human is watching and will interrupt if priorities changed. Asking
permission to continue trades their attention for nothing: the answer is
almost always "yes, obviously", and the round-trip cost is a stalled
agent and a context switch for the human.

### 2. "Nothing found" is always a valid outcome

A search, audit, or review that turns up nothing reports *nothing* — it
does not inflate a weak match to avoid an empty result, and it does not
stall asking whether an empty result is acceptable. It is. This is the
[graceful-exit guardrail](graceful-exit.md) ([ADR-018](../adr/ADR-018-graceful-exit-framework-default.md));
anti-stall incorporates it because "I found nothing, should I keep
looking / is that OK?" is a stall with the same root cause as the
others. Report the empty result and move to the next item.

### 3. Present decisions, not questions

When you reach a fork, prefer **deciding and stating the decision** over
**asking which way to go**. "I'm using approach A because B is blocked
on X; say so if you'd rather I do B" keeps moving and still gives the
human a clean interrupt point. "Should I use A or B?" stops dead waiting
for an answer you were equipped to choose.

Reserve genuine questions for decisions that are the human's to make and
that you cannot resolve from the request, the code, or sensible defaults
— the same bar the `AskUserQuestion` tool sets. Pick the obvious option,
state it, and proceed.

### 4. Minimize blocking on human input

Every block on a human is a stall for as long as the human is away. Drive
the blocking surface toward zero:

- Resolve from context, code, and defaults before asking.
- Batch genuinely necessary questions instead of trickling them one at a
  time across turns.
- Make irreversible or outward-facing actions the *only* routine reason
  to pause for confirmation (see the boundary below).
- When you must report rather than ask, report and keep working on the
  parts that are not blocked.

### 5. If genuinely blocked, file it and move on — do not idle

A real blocker on item N does not stop the whole run. Capture it and pick
up item N+1:

1. Create a Beads task recording the blocker and what unblocks it
   (canonical store: the `context-engine` Beads DB,
   `BEADS_DIR=~/beads/context-engine/.beads`; see
   [principle 8](../../.claude/CLAUDE.md#core-engineering-principles-mandatory)).
2. Note the block in your summary so the human sees it.
3. Move to the next independent item in the backlog.

An agent that idles on a single blocker while independent work waits is
stalling the whole backlog on one item. The blocker becomes tracked work
(visible, prioritisable, hand-off-able); the run continues.

## The boundary — when stopping *is* correct

Anti-stall is **not** "never stop". It is "do not stall on questions you
could answer or work you were already asked to do". These stops are
correct and override the directives above:

| Stop trigger | Rule | Source |
|---|---|---|
| Supervisor / user sends `stop`, `wrap up`, `status?`, or a redirect | Acknowledge ≤2 turns, comply ≤5; `stop` → commit and return | [Agent Delegation Enforcement §2](../../.claude/CLAUDE.md#agent-delegation-enforcement-mandatory) |
| Same approach failed twice with the same error | Stop retrying; switch approach or report with two concrete options | [§3 two-retry maximum](../../.claude/CLAUDE.md#agent-delegation-enforcement-mandatory) |
| Permission / access block | 0 retries — report immediately; escalate into a model fix | [§3](../../.claude/CLAUDE.md#agent-delegation-enforcement-mandatory); [principle 7 (JIT access)](../../.claude/CLAUDE.md#core-engineering-principles-mandatory) |
| Irreversible or outward-facing action (delete, force-push, publish, send) | Confirm first unless durably authorized | repo guardrails |
| A decision genuinely the human's to make, unresolvable from context | Ask — but present options and a recommendation, not an open question | directive 3 |

The throughline: **stop for commands, for repeated failure, for
irreversibility, and for decisions only a human can make — never for
permission to continue work you are already doing.**

## See also

- [Agent Delegation Enforcement](../../.claude/CLAUDE.md#agent-delegation-enforcement-mandatory)
  — the stop-side complement (commit discipline, supervisor commands,
  two-retry maximum, cleanup). Anti-stall and Delegation Enforcement are
  two halves of one contract: *keep going by default; stop for these
  specific triggers*.
- [graceful-exit.md](graceful-exit.md) / [ADR-018](../adr/ADR-018-graceful-exit-framework-default.md)
  — the no-overfit guardrail that directive 2 incorporates.
- `agm/docs/specs/SPEC-stall-detection.md` — the AGM *infrastructure*
  that detects and recovers stalled worker processes (permission
  timeout, no-commit, error loop). That spec is the enforcement
  counterpart to this behavioural one: it catches the stalls this spec
  tells agents not to create.
- [AGENTS.why.md](../../AGENTS.why.md) — the publish-once,
  reference-everywhere precedent.
