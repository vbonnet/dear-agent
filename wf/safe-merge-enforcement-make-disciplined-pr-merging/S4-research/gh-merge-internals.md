# S4 Research — `gh pr merge` Internals, Server Enforcement, Extensions, Interception

> Produced by a research subagent, 2026-06-11, against gh 2.94.0 and cli/cli trunk. Sources cited inline.

## 1. Client-side logic of `gh pr merge`

Source: [merge.go](https://github.com/cli/cli/blob/trunk/pkg/cmd/pr/merge/merge.go), [http.go](https://github.com/cli/cli/blob/trunk/pkg/cmd/pr/merge/http.go).

The CLI fetches NO `statusCheckRollup`, NO `reviewDecision`, NO `reviewThreads`. Its entire gate is the server-computed `mergeStateStatus` enum, evaluated in `blockedReason(status, useAdmin)`:

| status | plain `gh pr merge` | `--admin` |
|---|---|---|
| `CLEAN` | merges immediately | merges |
| `UNSTABLE` (checks failing/pending but **not required**) | **merges immediately** | merges |
| `HAS_HOOKS` | merges immediately | merges |
| `BLOCKED` | refuses, exit 1, suggests `--auto`/`--admin` | **merges anyway** |
| `BEHIND` | refuses | merges anyway |
| `DIRTY` (conflict) | refuses | **still refuses** |
| `UNKNOWN` (still computing) | **proceeds, lets server decide** — no retry/backoff | same |

- **Plain `gh pr merge` does NOT wait for checks** — no polling exists in the merge path; no "checks failing, continue?" prompt exists at all.
- **`UNSTABLE` is a trap:** failing non-required checks never stop a plain merge. On free-private repos *nothing* is ever required, so everything is UNSTABLE-at-worst → plain merge always proceeds.
- **`--admin` does two purely client-side things:** suppress the BLOCKED/BEHIND refusal, and skip the merge queue. The GraphQL mutation sent is **identical** with or without it — bypass works only because the server lets that user through (enforce_admins=false or no protection).
- **`--auto`**: if PR already mergeable, merges immediately; else arms server-side auto-merge — which errors unless the repo setting "Allow auto-merge" is on (that's why it's "disabled" in our repos).

## 2. Server-side enforcement at the merge endpoint

Always enforced for everyone (no bypass): open, not draft, no conflicts (405), allowed merge method (405), and **`expectedHeadOid`/`sha` match → 409 on mismatch** — the server-side anchor for `--match-head-commit`, the one enforcement hook a wrapper gets for free.

With `enforce_admins=false` (dear-agent) the server accepts an admin merge over red required checks, missing reviews, unresolved conversations. With no protection (free private repos) there is no BLOCKED state at all — `reviewDecision`, checks, conversation resolution are pure advice. **In both of our configurations, discipline must live client-side.**

## 3. Extension ecosystem survey

`gh extension search` across merge/safe/verify/guard/policy terms + topic search: **no existing extension performs pre-merge verification (checks + threads + review decision). The niche is empty.** Closest: [fini-net/gh-amp](https://github.com/fini-net/gh-amp) *displays* CI/review state but does not gate; [agynio/gh-pr-review](https://github.com/agynio/gh-pr-review) resolves threads from the terminal (building block, not a gate); several extensions exist to *bulk-merge faster* (the opposite). A verifying `safe-merge` would be novel.

## 4. Shadowing/blocking `gh pr merge`

- **`gh alias set pr merge ...` is impossible** — [validations.go](https://github.com/cli/cli/blob/trunk/pkg/cmd/alias/shared/validations.go) rejects any alias that resolves to an existing command.
- **Extensions cannot override core commands** — [extension command.go](https://github.com/cli/cli/blob/trunk/pkg/cmd/extension/command.go): "An extension cannot override any of the core gh commands."

| Mechanism | Coverage | Bypass routes |
|---|---|---|
| gh extension (`gh safe-merge`) | Opt-in; adds safe path, blocks nothing | everything — pair with enforcement layer |
| Shell alias/function | Interactive human shells | `command gh`, `\gh`, abs path, scripts |
| PATH shim named `gh` | All PATH-resolved invocations | abs path `/opt/homebrew/bin/gh`; PATH order (homebrew is FIRST on this machine) |
| **PreToolUse hook (exit 2)** | All agent Bash invocations, robust to `cd`/quoting via fsguard parser | non-agent surfaces only |

To make the wrapper THE ONLY agent path: PreToolUse deny on `gh pr merge` + `gh api .../merge` + `gh api graphql` containing `mergePullRequest`, exit 2 pointing at the wrapper; wrapper itself vetted + allow-listed. The wrapper should call the real gh by absolute path (or via `gh api` forms the deny net distinguishes) so it isn't blocked by its own rules.

## 5. Exact pre-merge verification queries

One-shot GraphQL (single round trip, all signals), run via `gh api graphql`:

```graphql
query($owner:String!, $repo:String!, $pr:Int!) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$pr) {
      state isDraft mergeable mergeStateStatus reviewDecision
      reviewThreads(first:100) {
        totalCount
        nodes { isResolved isOutdated }
        pageInfo { hasNextPage endCursor }
      }
      commits(last:1) { nodes { commit {
        oid
        statusCheckRollup {
          state
          contexts(first:100) {
            totalCount
            nodes {
              __typename
              ... on CheckRun      { name status conclusion isRequired(pullRequestNumber:$pr) }
              ... on StatusContext { context state isRequired(pullRequestNumber:$pr) }
            }
          }
        }
      } } }
    }
  }
}
```

- Checks: `CheckRun.conclusion ∈ {SUCCESS, FAILURE, NEUTRAL, SKIPPED, CANCELLED, TIMED_OUT, ACTION_REQUIRED, STALE}`, `status ∈ {QUEUED, IN_PROGRESS, COMPLETED}`. On free-private repos nothing is `isRequired` → **gate on ALL checks**, not required ones.
- Unresolved threads: `reviewThreads.nodes[] | select(.isResolved | not)` — GraphQL-only data. Policy needed for `isOutdated` (GitHub's own resolution requirement counts them).
- `reviewDecision ∈ {APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, null}`.
- `mergeStateStatus`: treat `UNKNOWN` as **re-poll**, never pass.
- `gh pr checks N --json name,state,bucket` (`bucket ∈ pass/fail/pending/skipping/cancel`, exit 8 = pending, `--watch --fail-fast` for blocking waits).
- **Gotcha (verified live):** legacy combined-status endpoint `commits/{sha}/status` reports `pending` with `total_count: 0` on a fully green commit — never use it as the signal.
- **TOCTOU safety:** capture `headRefOid` during verification, merge with `--match-head-commit <sha>` / `expectedHeadOid` → any race push yields a server-side 409.

## Key takeaways

1. `gh pr merge` verifies one enum; `--admin` is a client-side "stop refusing" flag; `UNSTABLE` merges straight through.
2. On our repos the server enforces nothing relevant → the wrapper IS the policy layer.
3. No existing extension does this; alias/extension shadowing is closed in gh source → PreToolUse hook + vetted wrapper are the enforcement seams.
4. Verification = one GraphQL query + `--match-head-commit`.
