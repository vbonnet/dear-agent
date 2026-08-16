# ADR-034: Squash-only merge contract + auto-merge arming

Status: Accepted (2026-06-24)

- **Epic:** ce-kf6j
- **Closes:** ce-kf6j.7
- **Relates:** ce-kf6j.1, ce-kf6j.5, ce-r81r, ce-kf6j.3

Three changes landed together to form a new merge pipeline:

1. **Strict required-status-checks dropped** (ce-kf6j.1) — branches are no longer
   required to be up-to-date with `main` before required CI checks run.
2. **Squash-only enforced via IaC** (ce-kf6j.5) — rebase and merge-commit strategies
   are blocked by the `modules/managed-repo` branch-protection ruleset; only squash
   merges land on `main`.
3. **Auto-merge armed on routine `safe-pr create`** (ce-r81r) — `cmd/safe-pr`
   runs `gh pr merge --auto --squash <url>` after opening a non-draft PR, so it
   merges once required checks and reviews pass. Drafts are an explicit human
   handoff and remain unarmed.

**The routine pipeline:** push branch → open a non-draft PR via `safe-pr` → CI
green → auto-merge fires. Human-required work stays draft until explicitly
advanced.

**Supervisor contract (what changed for burndown workers):**

- **Do not rebase a branch just because it is behind `main`.** A squash merge puts the
  flattened commit on top of current `main` regardless of how many commits `main` has
  accumulated — drift is absorbed at merge time, not before. Rebasing "behind" branches
  wastes CI cycles and is actively wrong.
- **Only run `safe-rebase` when there are actual merge conflicts.** The conflict check
  is sufficient; staleness is not.

**Merge-velocity health** (automated: `cmd/merge-velocity` emits the
`merge.velocity.*` OTel instruments via `internal/telemetry`; these thresholds
interpret those signals):

| Signal | Healthy | Warning |
|--------|---------|---------|
| Created-vs-merged delta/day | ≤ 5 | > 10 (net accumulation) |
| Median time-to-merge | ≤ 24 h | > 48 h |
| Open PR count trend | flat/falling | growing week-over-week |

Manual fallback check: `gh pr list --json number,createdAt | jq length`. The firehose recurred
in ce-qpg9 when +72 PRs/day went unnoticed for days; these thresholds are set to catch
the trend before it compounds.

When the delta exceeds the warning threshold, the correct response is to pause PR
creation (not speed up workers) and drain the existing queue first.
