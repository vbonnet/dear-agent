# Why: Harness Hygiene

## The principle
The harness is everything wrapped around the model: instructions, `AGENTS.md`,
skills, memory, tool schemas, permissions, hooks, gates, model choice. It
accretes **one reasonable correction at a time** — "every rule fixed a real
problem, and nobody can see them together." The residue is crud: anything that
makes the right procedure harder to find, the current rule harder to identify,
the output easier to reject, or the setup harder to maintain. Crud is not free
insurance; it is an active tax on completion. In one A/B, a 5,197-word method
scored *higher on analysis* than a 742-word brief but **failed the delivery
contract 2 of 3 times the brief passed** — stronger thinking, rejected output.

This squares with Anthropic's standing guidance: start with the simplest thing
that passes eval and add complexity only when it *demonstrably* improves
outcomes; find the smallest set of high-signal tokens, because a model "does not
read 200,000 tokens with the same care it reads 2,000." Earn every token and
every mechanism.

## The split-timer verdict (why this policy exists)
"Build everything, then pare later" is mis-specified, and that is what hurts us.
Two corrections make it right:

1. **It's per-capability, not two phases.** A "big pare-back later" invites a
   phase boundary that keeps sliding right — and demonstrably has (25:1
   filed-to-deployed; 3 of 9 flywheel edges wired). Earn the complexity
   continuously instead.
2. **Over-fit and over-harness have opposite urgency.**
   - *Over-fits are paged.* They cause outages NOW. Repair on sight.
   - *Over-harness consolidation is deferrable.* Collapsing the 3 spawn builders,
     the ~31 model-config sites, and the supervisor mesh is correct but large and
     *not on fire*; doing it mid-instability risks breaking the one thing limping
     along.

## Real failure cases (this repo)
- **The self-deadlocking RAM gate (ce-xj1b), re-embedded a THIRD time.** Spawns
  were gated on raw macOS `Pages free` — a number that is structurally near-zero
  forever (reclaimable pages parked off the free list). A gate meant to stop
  spawns during a real OOM read "exhausted" permanently: "we built a spawn gate
  to prevent overload, but now nothing can ever queue." ~2-day pipeline stall.
  The correct source is `memory_pressure -Q`. Now caught deterministically by
  `raw-mem-gate` in `cmd/structural-health` (ce-2vbg).
- **Nine LLM-on-a-timer monitors.** Each real question spawned its own scheduled
  LLM monitor; five silently died; one was created two days *after* a retro named
  the pattern broken. "LLM-on-a-timer is not a monitor; it is a standing quota
  drain shaped like one."
- **`trail.jsonl` at 18,188 lines, zero programmatic readers.** Write-only
  theater — a mechanism built, merged, and never wired to anything that reads it.
- **Heartbeat doctrine ported from the Go mesh** (tick <100ms) onto LLM
  supervisors (5–8min ticks) → near-100% false STALE, masking a 10-hour drought.
  "Invariants don't transfer across execution models."
- **`human_typing` guard as a denylist** of stacked point-fixes; a *different*
  trigger then blocked the Overseer overnight. The fix was to invert to an
  allowlist of known-safe states.

## The six verdicts (the safe pare operation)
Paring means *make visible and re-home*, not *delete on sight*. Assign each
control exactly one: **Keep** (earns its place) · **Give it one home**
(canonical owner, others call it) · **Load it later** (phase/task trigger, not
always-on) · **Turn it into a check** (prose → parser/counter/scan) ·
**Probation** (suspect; watch before removing) · **Retire safely** (migrate what
matters, then delete). "'Delete it because it's long' isn't a valid decision" —
guard equally against premature stripping.

## How to apply
- **Authoring a gate?** Add its liveness counter-check in the same change: it
  self-suppresses when throughput is zero AND the OS reports calm. A gate without
  one is a latent outage.
- **Touching an over-fit in a retro?** Pare it then, with a six-verdict
  disposition — do not file a "cleanup later" bead as a substitute (see
  [dear-retro](dear-retro.why.md)).
- **Reviewing a design?** Ask: which rules were written for a model that no
  longer exists; what should software enforce instead of a paragraph; is this one
  general mechanism or N per-incident ones.

See also: [broken-windows](broken-windows.why.md) (wire it or delete it),
[dear-retro](dear-retro.why.md) (pare continuously), and the strategy memo
`engram-research/memos/dear-agent-harness-audit-strategy-memo.md`.
