# Why `AGENTS.md` is a router

`AGENTS.md` is loaded by every supported harness, so its size and accuracy are
part of the repository interface. It contains only repository routing,
invariants, source-of-truth pointers, and verified entry commands.

Durable rules live in the canonical `docs/policies/*.ai.md` files. Command flags live
in the owning Cobra command or safe wrapper. Architecture facts live beside the
implementation. Temporal rationale, incident history, audits, and
retrospectives live in the configured `engram-research` worktree.

This separation prevents three recurring failures:

- copied policy bodies diverge from their canonical version;
- copied CLI and harness inventories survive after code changes;
- historical explanations consume context while looking like current rules.

Harness entry files therefore import `AGENTS.md` and do not copy it. Tests in
`internal/instructions` enforce that import topology, the router size budget,
its canonical links, and the absence of known retired command claims.
