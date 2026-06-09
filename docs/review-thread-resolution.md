# Resolving GitHub PR review threads (Gemini/bot comments)

Findings for building an agent SKILL that resolves PR review threads without a
human clicking **Resolve conversation**. Verified against the live GitHub
GraphQL schema and a real PR on 2026-06-09.

## Why this matters here

`dear-agent` main has `required_conversation_resolution=true` (see
[[dear-agent-pr-flow]]). A PR cannot merge while any review thread is
unresolved. **Replying to a comment does not resolve its thread** — resolution
is a separate mutation. Gemini (`gemini-code-assist`) opens one thread per
finding, so an agent that wants to land a PR must resolve those threads itself.

## The one hard fact: resolution is GraphQL-only

There is **no REST endpoint** to read or change thread resolution state.

- REST `GET /repos/{owner}/{repo}/pulls/{n}/comments` returns review *comments*,
  but the comment object has **no `resolved` field** and there is no
  `.../resolve` route. REST is useless for this task.
- Resolution lives entirely on the GraphQL `PullRequestReviewThread` object
  (`isResolved`, `viewerCanResolve`, `viewerCanUnresolve`, `resolvedBy`) and the
  `resolveReviewThread` / `unresolveReviewThread` mutations.

## 1. The mutation — `resolveReviewThread`

Schema-verified input type `ResolveReviewThreadInput`:

| field             | type      | notes                                   |
|-------------------|-----------|-----------------------------------------|
| `threadId`        | `ID!`     | **the thread ID** — see naming gotcha   |
| `clientMutationId`| `String`  | optional                                |

> **Gotcha:** the input field is `threadId`, **not** `pullRequestReviewThreadId`.
> (A common misremembering — the *object* is a `PullRequestReviewThread`, but the
> mutation arg is just `threadId`.)

```bash
gh api graphql -f threadId="PRRT_kwDO..." -f query='
  mutation($threadId:ID!) {
    resolveReviewThread(input:{threadId:$threadId}) {
      thread { id isResolved }
    }
  }'
```

`unresolveReviewThread` has the identical shape (use it to re-open a thread).

## 2. Getting thread IDs — list review threads

Thread IDs are **not** the numeric review-comment IDs from REST. They are opaque
node IDs shaped `PRRT_kwDO...`, obtained from
`repository.pullRequest.reviewThreads`:

```bash
gh api graphql -f owner=vbonnet -f repo=dear-agent -F pr=162 -f query='
  query($owner:String!, $repo:String!, $pr:Int!) {
    repository(owner:$owner, name:$repo) {
      pullRequest(number:$pr) {
        reviewThreads(first:100) {
          totalCount
          nodes {
            id
            isResolved
            isOutdated
            path
            comments(first:1) { nodes { author { login } body } }
          }
        }
      }
    }
  }'
```

Notes from the live run:
- `-F pr=162` (capital F) sends an **integer**; `pr` is `Int!`. Lowercase `-f`
  would send a string and fail the type check.
- `reviewThreads(first:100)` is a single page. PRs with >100 threads need
  cursor pagination (`pageInfo { hasNextPage endCursor }` + `after:`); rare for
  bot review, but the SKILL should handle it if it targets large PRs.
- `viewerCanResolve` is `false` for threads already resolved — filter on
  `isResolved == false` before attempting to resolve.

## 3. Resolve all unresolved threads on a PR

List → filter `isResolved == false` (optionally by `author ==
"gemini-code-assist"`) → call `resolveReviewThread` per `id`. Implemented in the
wrapper below.

## Wrapper script

`scripts/resolve-review-threads.sh` (in this repo) wraps all of the above:

```
resolve-review-threads.sh list        <owner> <repo> <pr>           # unresolved threads (JSON lines)
resolve-review-threads.sh list-all    <owner> <repo> <pr>           # every thread
resolve-review-threads.sh resolve     <threadId>                    # one thread
resolve-review-threads.sh resolve-all <owner> <repo> <pr> [author]  # all unresolved, optional author filter
resolve-review-threads.sh unresolve   <threadId>                    # re-open
```

Example — resolve every open Gemini thread on PR 231:

```bash
scripts/resolve-review-threads.sh resolve-all vbonnet dear-agent 231 gemini-code-assist
```

## Verification status

- **Read path** (`list`, `list-all`) — live-tested against PR #162: returns its
  4 `gemini-code-assist` threads correctly.
- **Mutation** (`resolve`/`unresolve`) — verified by schema introspection
  (input `threadId: ID!`, return `thread { id isResolved }`). A live round-trip
  was intentionally **not** run: it mutates shared, collaborator-facing PR state,
  which the auto-mode classifier blocks for a research task (correctly). Run a
  real resolve only on a PR you own when you actually intend to land it.

## SKILL design notes

- **Scope to bots by default.** Auto-resolving a *human* reviewer's thread is
  rude and can hide unaddressed feedback. Default the author filter to
  `gemini-code-assist` (and other known bots); require an explicit flag to touch
  human threads.
- **Resolve only after addressing.** The honest workflow is: reply to/fix the
  finding, *then* resolve. A SKILL that blindly resolves to force a merge defeats
  `required_conversation_resolution`. Pair this with a reply step.
- **No keychain/push concern.** All calls are `gh api graphql` (HTTPS via gh's
  token), not `git push`, so the keychain-hang guardrails ([[macos-env-gaps]],
  [[git-push-keychain-hang-safepush]]) don't apply here.
- Related merge-flow constraints live in [[dear-agent-pr-flow]].
