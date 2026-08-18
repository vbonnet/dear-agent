# Why: DEAR Retro Everything

## The principle
A fix without a retro fixes one instance; the failure class stays open and
recurs on another agent, another model, another day. The retro converts a
one-off patch into a durable prevention (a test, a gate, a policy). Writing it
to the shared knowledge base — not one agent's memory — is what makes the
prevention portable across harnesses and machines.

## Real failure cases (this repo)
- **The gpt-5.6 codex brick.** AGM defaulted codex workers to `gpt-5.6`, which a
  ChatGPT-account codex rejects with a hard 400. Every codex worker died on
  arrival; the `vroom-orchestrator` supervisor sat dead for ~11 hours before
  anyone noticed. A one-line default with no "is the default model actually
  callable?" check. The fix is trivial; the *prevention* (a startup validation)
  only exists if a retro demands it.
- **Fail-open write guard.** The `fsguard` bash tokenizer fails open on anything
  it can't parse — a security control that silently degrades. It shipped because
  no retro asked "what happens on unparseable input?"
- **Repeated worktree-reaper deletions** of active worktrees recurred multiple
  times because early incidents were patched, not retro'd into a guard.

## How to apply
- Trigger: any systemic defect, any surprising seam, any "that shouldn't have
  been possible." Not every trivial bug — the *class*-worthy ones.
- Location: `~/src/engram-research` (temporal/knowledge store), via a worktree —
  `~/src` is read-only golden.
- Output: Define / Execute / Audit / Retro, then one bead per prevention.

## Pare over-fits in the retro, not "later"
A retro that touches an over-fit must pare it *there*, because the deferred
cleanup demonstrably never arrives: a spawn-throttle keyed on a macOS memory
metric that reads structurally near-zero forever (so the gate never
re-opened — full incident in
[harness-hygiene](harness-hygiene.why.md#real-failure-cases-this-repo)) was
re-embedded a THIRD time while its cleanup bead sat open, and
`daily-ops-audit` shipped two days *after* a retro named its pattern broken.
Folding the six-verdict paring rubric into the retro itself is the fix for
"the deferred 'then' never comes." Prefer the "Turn it into a check" verdict —
a deterministic gate (like the `raw-mem-gate` scan) outlives any prose
reminder.

See also: [broken-windows](broken-windows.why.md),
[harness-hygiene](harness-hygiene.why.md).
