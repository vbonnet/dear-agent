---
name: drive-pr-to-ready
description: Use when driving an open PR all the way to ready-for-merge, autonomously or on a GitHub event. Runs the deterministic pr-blockers classifier in a loop, applies each blocker's exact fix, and stops only at genuine human-decision points. Trigger on "drive this PR to ready", "get this PR merged", "unblock and merge PR N", a routine firing on a pull_request event, or any sweep that must take PRs from open to merged.
---

# Drive a PR to Ready

**STATUS: STUB.** The contract, the stop conditions, and the reuse map below are
final and binding. The orchestration between them is not yet implemented. See
`engram-research/spikes/design-event-driven-drive-pr-to-ready.md` (epic
`ce-zijb4`).

Take one PR from wherever it is to `pr-blockers` verdict READY, then merge it if
policy allows. Every step that GitHub's merge state already answers is run as a
binary. An LLM is invoked only for the judgment steps named in step 3.

## Preconditions (check all, in order, before touching the PR)

1. **Cost.** `agm quota --check`. Exit 4 means halted: **stop**.
   Departure from the default: the `provider_quota` gate fails *open* by design,
   so an unreadable or stale reading normally allows the spawn. **This skill
   inverts that.** An unreadable quota is a stop, because an auto-driver with no
   cost ceiling and an event storm is the failure this bounds.
2. **Idempotence.** Compute `(repo, pr_number, head_sha, blocker_set)`. If a
   prior attempt at this exact key **succeeded** (reached READY or merged),
   **stop**: two GitHub events for the same already-completed state must not
   re-run. Track attempt state as `in-progress` / `failed` / `succeeded`
   rather than a binary processed flag — a `failed` or `in-progress` prior
   attempt at the same key still falls through to the attempt budget below,
   so a transient error or timeout gets to retry instead of being wrongly
   treated as already handled.
3. **Attempt budget.** At most 3 attempts per `head_sha`. Exhausted means
   escalate to a human, never retry.
4. **Sensitivity.** Classify the changed paths through
   `internal/mergeloop.Classify`. A `blocked-policy` verdict means the PR stays a
   draft and a human is notified. **Stop.**

## Workflow

### 1. Diagnose. Never guess.

```sh
pr-blockers <number> --json
```

Exit 0 READY or MERGED, 1 BLOCKED or CLOSED, 2 error. This list is exhaustive
over GitHub's merge state. Do not investigate a merge failure any other way;
that is the `pr-merge-blockers` skill's rule and it applies here unchanged.

### 2. Deterministic blockers. No LLM.

| Blocker | Action |
|---|---|
| `BEHIND` | `gh pr update-branch <n>`, then go to step 1 |
| `PENDING_REQUIRED_CHECK` | `gh pr checks <n> --watch` with a timeout, then go to step 1 |
| `DRAFT`, non-carve-out | `gh pr ready <n>`, then go to step 1 |

A PR blocked only by these must never reach an LLM.

### 3. Judgment blockers. Agent works here.

| Blocker | Action |
|---|---|
| `CONFLICTS` | `safe-rebase` onto base, resolve, `safe-push`. Never force-push. |
| `FAILING_REQUIRED_CHECK` | Read the logs, fix the code, push. A known flake (configured under `flaky_checks` in `.safe-merge.yml`, enforced by `internal/safegit/flakevalve.go`) gets **exactly one** rerun. |
| `UNRESOLVED_THREADS` | Address in code, verify the fix landed, then resolve. See the hard rule below. |
| `CHANGES_REQUESTED` | Address the review, push, re-request review. |
| `REVIEW_REQUIRED` | Obtain an approving review. Escalate if none is available. |

**Thread resolution, hard rule.** Use the `github-thread-resolver` skill, which
verifies each fix actually landed before resolving. Then
`resolve-review-threads resolve-all <owner> <repo> <n>` to reach ZERO unresolved,
outdated included.

- **Never resolve by author identity.** Fetch the comment body.
- **Never resolve a thread you did not verify fixed in code.** A reply saying
  "fixed" is not evidence.
- **A P1 or severity-marked thread you have not fixed is a stop, not a resolve.**

Bead `ce-lr7j` (P0, open): `mergeloop` auto-resolved 5 Codex threads on PR #989,
four of them P1 correctness findings on a live production reconciler, releasing
auto-merge one second later, because it matched on author login and never
fetched the body. That is the defect this rule exists to prevent.

**Review-round cap, hard rule.** Track a review-round counter per PR,
independent of the attempt budget above and the no-progress detector below —
both miss the case a real automated-reviewer bot produces: each round's fix
lands cleanly (real progress, a changed blocker set, a fresh head_sha), so
neither the attempt budget (same-key retries) nor the no-progress detector
(identical blocker set) ever fires, yet the bot never runs out of new P2
findings to raise. **Stop after 5 review rounds on the same PR**, or after 2
consecutive rounds that raise only P2-or-lower findings with no new attack
surface, whichever comes first — merge if genuinely READY and non-carve-out,
otherwise escalate to a human with the remaining findings named. Never
grind past the cap alone.

Bead `ce-0wdjc` (P2, open): PR #1355 ran 16 review rounds and accumulated 106
review threads in one session before being manually stopped on a fresh P1 —
this rule is the fix, not yet enforced anywhere before this stub.

### 4. Re-diagnose and loop

Re-run step 1 after every action. **Act only on a fresh diagnosis**, never on one
from earlier in the run: a human may be editing the same PR concurrently.

**No-progress detector:** compare the blocker set at the specific-detail level
(failing check names, conflicting paths, thread IDs) not just the high-level
blocker codes — a new commit that changes which check fails or which files
conflict is progress even if the blocker code (e.g. `FAILING_REQUIRED_CHECK`)
stays the same. Stop only when the detailed set is unchanged after an
attempt; that is a loop, not progress.

### 5. Merge

Only on verdict READY, only for non-carve-out PRs, only through the gate:

```sh
safe-merge --pr <number>
```

`safe-merge` re-checks the head SHA at merge time, which is what makes it safe
against a concurrent human push.

## Stop conditions (hand to a human)

1. **Carve-out** per `docs/policies/autonomous-merge.ai.md`: security, product
   behavior, money/billing, agent governance (`docs/policies/`), or an agent
   control surface (auth, quota, notification, merge policy). For these the
   draft-to-ready flip **and** the merge are human-only. Leave it a draft.
   Known gap: a policy category with no matching glob in
   `DefaultSensitiveGlobs` is not enforced and will classify green. When unsure,
   treat it as a carve-out.
2. **Data-loss shaped:** migrations, deletions, anything needing a force push.
3. `UNKNOWN_BLOCK` from `pr-blockers`.
4. An unfixed P1 review thread.
5. Attempt budget exhausted, or the no-progress detector fired.
6. Review-round cap reached (5 rounds, or 2 consecutive P2-or-lower-only
   rounds).
7. `agm quota --check` halted or unreadable.
8. An opt-out label (`no-autodrive`) is present.

## NEVER

- `safe-merge --skip-review-check` or `safe-merge break-glass`. Break-glass is a
  TTY-only, audited human emergency path.
- Merge with unresolved threads.
- Force-push (`requiresLinearHistory=true`; use `safe-rebase` + `safe-push`).
- Flip a carve-out draft to ready.
- Diagnose a merge failure by reading code.

## Reuse (build nothing that exists)

`cmd/pr-blockers`, `internal/safegit.Diagnose`, `internal/mergeloop.Classify`,
`cmd/safe-merge`, `cmd/safe-pr`, `cmd/safe-rebase`, `cmd/safe-push`,
`cmd/resolve-review-threads`, `cmd/babysit-prs` (the sibling-BEHIND fan-out body;
do not fork it), `agm quota`, `internal/safegit/flakevalve.go`.

Skills: `pr-merge-blockers` (diagnosis discipline), `github-thread-resolver`
(verified thread resolution).

## Verification

- `pr-blockers <number>` exits 0 READY and the PR merged, **or** the run stopped
  at a named stop condition above with a human notified. Any other ending is a
  defect.

## References

- Design spike: `engram-research/spikes/design-event-driven-drive-pr-to-ready.md`
  (epic `ce-zijb4`).
- Sensitivity policy: `docs/policies/autonomous-merge.ai.md`.
- Flake allowlist: `.safe-merge.yml` (`flaky_checks`), enforced by
  `internal/safegit/flakevalve.go`.
- Incident this rule prevents: bead `ce-lr7j`.
- Review-round cap incident and retro: bead `ce-n6g3y` (parent), `ce-0wdjc`
  (this rule).
