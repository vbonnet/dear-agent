# Why: Broken Windows

## The principle
"Your hack / unfinished cleanup is the next agent's precedent." In a codebase
worked by many autonomous agents across harnesses, deprecated code is not inert —
it is an active invitation. An agent that lands on the old path cannot tell it is
dead; it extends it. Now two implementations exist and both accrete changes.

## Real failure cases (this repo)
- **VROOM forked into five.** A programmatic Go mesh and an LLM-`/loop` mesh were
  left coexisting. By 2026-07-01 there were **five** `cmd/vroom-*` entrypoints
  (`vroom-dispatch`, `vroom-dispatch-direct`, `vroom-governor`, `vroom-mesh`,
  `vroom-prompt-gen`). Nobody chose one and deleted the rest, so each new agent
  extended whichever it found. All five entrypoints still exist; consolidation
  is tracked as ce-93lw.5.
- **Phantom TypeScript engine.** `wayfinder/SKILL.md` still described a
  `core/cortex/*.ts` engine that does not exist — the Go rewrite never removed the
  old design docs, so agents were guided toward a codebase that was gone.
- **Dead `--workflow` flag.** `agm session new --workflow` is parsed, validated,
  logged — and never consumed. A non-functional surface presented as real; still
  present, removal tracked in ce-93lw.12.
- **Wayfinder V1 alongside V2.** Code defaulted `AllPhases()` to V1 while declaring
  V2 canonical — the old model was never deleted, so the gate was ambiguous.

## How to apply
1. Replacing something? Find every caller/doc/flag/entrypoint of the old thing.
2. Migrate what's worth keeping.
3. Delete the old implementation **in the same PR**. No "cleanup follow-up" bead
   as a substitute for deletion — follow-ups get abandoned (they were, repeatedly).
4. Grep afterward to prove only one version remains.

See also: [definition-of-done](definition-of-done.why.md) (unmerged/partial work
is itself a broken window).
