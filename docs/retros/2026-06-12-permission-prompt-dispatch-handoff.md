# DEAR Retro: Permission Prompts Block Dispatch→Code Handoffs

**Date:** 2026-06-12
**Bead:** ce-32t (P2) — "DEAR retro: Permission prompt blocking in Dispatch→Code tasks"
**Severity:** Medium (autonomous agents stall at permission prompts; human must unblock)
**Status:** Root causes documented; three action items produced.

## Define

**The trigger.** When an Orchestrator or Dispatch agent hands a task to a Code
agent, the Code agent routinely hits permission prompts mid-execution — usually
for `Bash` operations like `git push`, `gh api`, or file writes outside the
worktree — and cannot proceed without a human answering the prompt. The Code
agent either stalls waiting for input or the session dies. The handoff appears
complete from the Dispatch side but is actually blocked on the Code side.

**Why this matters.** A stalled Code agent does not emit a fail signal visible
to the orchestrator; it just sits at the prompt. The Dispatch→Code handoff is
a common automation pattern: Dispatch defines scope and spawns a Code worker.
If the Code worker can't run autonomously, the chain breaks silently.

**The question this retro answers:** *why do Code agents hit permission prompts
that Dispatch did not anticipate, and what structural fixes close the gap?*

## Execute (investigation)

### Root causes identified

**R1 — AskUserQuestion approval does not authorize Bash-level operations.**  
When an agent asks the user (via `AskUserQuestion`) whether to proceed with
an operation, a verbal "yes" answer does not grant the corresponding
`Bash(command:*)` or `Edit(file_path:*)` permission. Permission is governed
by the `settings.json` classifier rules, not by in-conversation approval.
The Code agent reads "yes" and tries the operation anyway — then hits the
classifier block. See `memory/permission-classifier-vs-askuserquestion.md`.

**R2 — Hook exit-2 is binary; there is no "warn and allow" path.**  
The fs/bash write-guard hooks (before ce-mp8) could only hard-block or
allow. When a Code agent writes to a path that is likely fine (e.g., a
`/tmp` staging area that was not in the `Writable` carveout list), the
hook exits 2 and the agent stalls with no escalation path other than
raising a `PERMISSION_ESCALATION` request — which a human must see and
act on. A "warn and continue" mode would have allowed autonomous recovery
in most of these cases. *Note: ce-mp8 (PR #361) ships `EnforceWarn` /
`EnforceAsk` / `EnforceDefer` to address this.*

**R3 — Scoped allow rules were over-removed.**  
The `Bash(gh api:*)` allow was removed from `settings.json` (dotfiles PR
#23) to tighten security. This is correct for the general case (broad
`gh api:*` allows too much). However, the removal left Code agents unable
to run `gh api --method GET` or `gh api graphql` for read-only queries
(e.g., `resolveReviewThread`). Agents in the middle of a PR-merge flow hit
a prompt at the graphql step with no workaround. Bead ce-9v2q tracks the
follow-up (scoped read-only entries).

**R4 — Dispatch does not propagate a "pre-approved command set" to Code.**  
A Dispatch agent that assembles a task knows which `gh`, `git`, and file
operations the Code worker will need. There is no mechanism to convey
"these operations are pre-approved for this task" to the Code agent's
session, so every Code agent starts with the same base permission set
regardless of what the orchestrator blessed.

### Data points (recent examples)

| Session | Operation blocked | Duration stalled |
|---------|------------------|-----------------|
| ce-5vog | `gh api graphql resolveReviewThread` | >30 min (human unblocked) |
| ce-37u  | `git push -u origin feat/*` (keychain prompt) | ~5 min (safe-push workaround) |
| ce-eky  | Bash write to `/private/tmp/...` (not in Writable carveout) | ~10 min |
| ce-1js  | `go install github.com/uudashr/gocognit@latest` (binary install) | ~3 min |

## Audit

**What enforcement is missing?**

1. The graduated enforcement (ce-mp8) ships `EnforceWarn`/`EnforceAsk`/
   `EnforceDefer` — a concrete path from "hard block" to "let the agent
   continue with guidance." Without this, every hook trigger requires human
   intervention.

2. There is no "Dispatch pre-approved commands" mechanism. Each session
   inherits only the global `settings.json` rules.

3. The JIT access principle (CLAUDE.md principle 7) says permission blocks
   should be escalated into model fixes — but Code agents cannot run a
   "mini DEAR retro and fix the model" while blocked on the prompt they
   need fixed. The fix must happen before the next handoff.

**Why did previous attempts not fix this?**

- The `Bash(gh api:*)` allow was over-broad and correctly removed, but
  no scoped replacement was added in the same change.
- The `EnforceWarn` mode did not exist (closed by ce-mp8).
- The JIT fix flow assumes a human is present to approve the escalation.
  In autonomous sessions, no human is watching.

## Retro (action items)

**R.1 — Add scoped `gh api` read-only allows [ce-9v2q, already open].**  
Add `Bash(gh api --method GET *)` and `Bash(gh api graphql *)` as scoped
entries in `settings.json`. This directly fixes R3 and the `resolveReviewThread`
stall case. Requires REVIEW.md gate (surface b). See ce-9v2q.

**R.2 — Set default enforcement to `EnforceWarn` for new non-src writes [ce-mp8, shipped].**  
The graduated enforcement shipped in PR #361. Operators can now set
`FSGUARD_ENFORCEMENT=warn` in their session to surface guidance without
hard-blocking. The next step (ce-mp8 follow-up) is to change the
*default* for the fs-write-guard to `warn` for paths outside `/src` but
also outside `~/worktrees`. These are common false-positive block sites
(tool caches, `~/go/bin`, `/private/tmp`). File a dedicated bead for the
default-change rather than doing it inline.

**R.3 — Document the "Code agent pre-flight" checklist in CLAUDE.md.**  
Before a Dispatch agent hands off to a Code worker, the dispatch prompt
should include the list of operations the Code agent will need and verify
they are in `settings.json`. This is a documentation fix, not a code fix.
Add a "Dispatch→Code handoff checklist" to `AGENTS.md` or the CLAUDE.md
"Anti-Stall" section that lists:
  - Which `Bash(*)` rules the Code agent will need
  - Which git operations are expected (`push`, `commit`, `worktree add`)
  - Whether `gh api graphql` is needed (for PR resolution)

**Out of scope for this retro (no inline fixes):**  
- Implementing a "pre-approved command set" propagation mechanism (large,
  architectural, no existing home)
- Fixing the `agm supervisor run` 401 (tracked in
  `memory/agm-supervisor-auth-blocker.md`)
