---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-19
tokens: 340
title: Anti-stall
description: Continue through known work while stopping at explicit safety, authority, failure, redirect, and completion boundaries.
tags: [policy, agents, execution, safety]
---

# Anti-stall policy

Use this policy when an agent is executing a multi-step request, backlog, or
plan. It defines when to keep working and when stopping is correct.

## Default

Continue through known work without asking for permission to continue. A user
or supervisor can redirect the run; do not trade their attention for approval
to do work already requested.

## Directives

1. **Continue through known work.** Complete the next in-scope step while safe,
   relevant work remains.
2. **Treat an empty result as valid.** Report no match instead of inflating weak
   evidence. The executable no-overfit contract is
   [`pkg/gracefulexit/SPEC.md`](../../pkg/gracefulexit/SPEC.md).
3. **Make reversible implementation decisions.** State the assumption and
   proceed when context, code, and sensible defaults determine the choice.
4. **Minimize human blocking.** Batch genuinely necessary questions and keep
   independent work moving.
5. **Track a local blocker and move on.** Record what is blocked and what would
   unblock it in the canonical Beads database, then continue with independent
   work.

## Stop boundaries

Stop or defer the affected action when:

- the user or supervisor sends a stop, wrap-up, or redirect command;
- the same approach has failed twice;
- permission or access is denied;
- an irreversible or externally visible action lacks durable authorization;
- required authority, external coordination, or a human-owned product decision
  prevents safe progress; or
- all in-scope work is complete.

A blocked action does not stop independent safe work. Never work around a
permission boundary or broaden the request merely to keep moving.

## Verification

`pkg/gracefulexit/antistall_reference_test.go` verifies that the root router
links this policy and that its five directives and stop boundary remain intact.
