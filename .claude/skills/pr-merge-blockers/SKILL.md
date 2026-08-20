---
name: pr-merge-blockers
description: Use BEFORE working any stuck, blocked, or non-merging PR, or whenever merge state is unclear. Runs the deterministic pr-blockers classifier so the exact blocker and exact fix come from GitHub's merge state instead of guesswork. Trigger on "why won't this merge", "PR is blocked", "checks are green but it won't merge", or any impulse to investigate a merge failure by reading code.
---

# PR Merge Blockers

Never diagnose a stuck PR by reading code, re-running CI on a hunch, or
theorizing. Every real merge blocker is deterministically knowable from two
queries, and the `pr-blockers` tool runs both and prints the exact fix.

Evidence this rule exists: in the week of 2026-08-10 alone, ten sessions
misdiagnosed merge blockers (stale-base theories, "BLOCKED = awaiting review"
guesses, "threads are stale noise" assumptions), costing roughly 150 wasted
tool calls, while about 90 percent of actual blockers were unresolved review
threads or an out-of-date branch. See the DEAR retro
`engram-research/retrospectives/2026-08-18-pr-merge-blocker-guessing.md`.

## Workflow

1. Run the classifier first, before any other investigation:

   ```sh
   pr-blockers <number>            # or: pr-blockers <number> --json
   ```

   Install once if missing: `make install-pr-blockers`.

2. Apply the printed fix for each blocker, in the printed order:

   | Blocker | Exact fix |
   |---|---|
   | `DRAFT` | `gh pr ready <n>` (humans flip security/product/money PRs) |
   | `CONFLICTS` | `safe-rebase` onto base, resolve, `safe-push` |
   | `FAILING_REQUIRED_CHECK` | fix the named check; known flakes get one rerun |
   | `PENDING_REQUIRED_CHECK` | `gh pr checks <n> --watch` |
   | `UNRESOLVED_THREADS` | address in code, then per thread: `resolve-review-threads reply-resolve <threadId> "Fixed - <what changed>"`; sweep with `resolve-all <owner> <repo> <n>` (it refuses unanswered threads) |
   | `CHANGES_REQUESTED` | address the review, push, re-request |
   | `REVIEW_REQUIRED` | obtain an approving review |
   | `BEHIND` | `gh pr update-branch <n>` |

3. Re-run `pr-blockers <number>` until the verdict is `READY`.

4. Merge only through the gate: `safe-merge --pr <number>`. In repos without
   safe-merge, the equivalent is `gh pr merge <n> --auto --squash`.

## The two traps this skill kills

- **Outdated threads still block, and outdated is not obsolete.** The GitHub
  UI collapses them behind "Show outdated" and casual queries omit them, but
  required conversation resolution counts every unresolved thread. The inverse
  guess is just as wrong: do not assume unresolved threads are stale noise;
  measure with the query below. Unresolved must reach zero, outdated or not.
  `isOutdated` only means the diff hunk moved. On #1242 all three outdated
  threads were unaddressed P1 findings, so outdated never licenses a resolve.
- **BEHIND churn is normal, not a defect.** Branch protection requires
  branches up to date with base, so every merge flips sibling PRs to BEHIND.
  The fix is one `gh pr update-branch`, not an investigation. Expect a bot
  re-review after any push; resolve the new threads and go again.

## Resolving threads: evidence, not volume

Resolving a thread asserts its point was handled. Two opposite failures have
both shipped here, and both are invisible afterwards because a resolved thread
looks the same either way:

- **Bulk-resolve without addressing.** Measured 2026-08-20 across the 24 open
  PRs: 30 of 134 resolved threads had no reply at all, and on #1242 and #1263
  every resolved thread was closed without one.
- **Reply and never resolve.** Four unresolved threads carried replies, leaving
  their PRs blocked on conversation resolution for nothing.

So the tool now enforces the rule instead of asking you to remember it. A
thread counts as ANSWERED when someone other than its opening author had the
last word:

```sh
# the normal path: state the reason and close the thread in one step
resolve-review-threads reply-resolve <threadId> "Fixed - <what changed>"

# sweep: resolves ANSWERED threads, refuses the rest by name, exits non-zero
resolve-review-threads resolve-all <owner> <repo> <n> [author]
```

`resolve-all --force` resolves unanswered threads. Use it only to dismiss a
finding deliberately, and say why in a PR comment. Never reach for it to make
a refusal go away.

## Verification

- `pr-blockers <number>` exits 0 with verdict `READY` (then merge), or you
  are actively executing one of its printed fixes. If you are investigating
  a merge failure any other way, you are off-skill: stop and run it.
- A `BLOCKED` verdict with `UNKNOWN_BLOCK` is the only case where deeper
  diagnosis is sanctioned, and it starts from branch-protection comparison,
  never from code.

## References

- `cmd/pr-blockers/SPEC.md`: EARS contract for the classifier.
- `internal/safegit/blockers.go`: classification logic (single owner).
- Raw thread query, when you need thread IDs for targeted resolution
  (`isOutdated` included deliberately; paginate on `endCursor`):

  ```graphql
  query($owner:String!, $repo:String!, $pr:Int!, $after:String) {
    repository(owner:$owner, name:$repo) {
      pullRequest(number:$pr) {
        reviewThreads(first:100, after:$after) {
          pageInfo { hasNextPage endCursor }
          nodes { id isResolved isOutdated path comments(first:1) { nodes { author { login } body } } }
        }
      }
    }
  }
  ```
