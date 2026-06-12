# D4 — Requirements

User-stated requirements (verbatim intent), decomposed and made testable. R = requirement, layer tags refer to D3 tiers (T1 server, T2 wrapper, T3 agent-enforcement, T4 audit).

## R1 — `--admin` bypass is PREVENTED, not discouraged

- R1.1 On every public repo, a `gh pr merge --admin` against an unmet ruleset MUST fail server-side (zero-bypass ruleset). [T1]
- R1.2 An agent invoking `gh pr merge` with `--admin` (any spelling/position) MUST be blocked pre-execution with a teaching message. [T3]
- R1.3 `safe-merge` MUST NOT expose any flag that weakens the predicate (`--admin`, `--force`, `--skip*`, `--no-verify` rejected by construction, as safe-push rejects force-push). [T2]
- R1.4 REST/GraphQL merge routes (`gh api .../pulls/{n}/merge`, `mergePullRequest`, `enablePullRequestAutoMerge`) MUST be blocked for agents; the `Bash(gh api:*)` allow rule MUST be narrowed. [T3]

## R2 — Merges wait for automated reviewers and CI

- R2.1 `safe-merge` MUST NOT merge while any check run on the head SHA is queued/in-progress, and MUST treat `mergeStateStatus: UNKNOWN` as wait-and-re-poll. [T2]
- R2.2 `safe-merge` MUST gate on ALL checks present on the head SHA (not only required ones — free-private repos have none required; `UNSTABLE` must not pass). Failure conclusions (FAILURE, TIMED_OUT, CANCELLED, ACTION_REQUIRED) block; policy table decides NEUTRAL/SKIPPED/STALE. [T2]
- R2.3 `safe-merge` MUST wait for expected bot reviewers (per-repo config; Gemini Code Assist where installed) to have produced a review newer than the head SHA, with an explicit timeout that reports "reviewer never arrived" as a distinct outcome rather than silently passing. [T2]
- R2.4 Known-flaky checks MAY be re-run once via a sanctioned path inside the wrapper; "CI is flaky" MUST never justify bypass. [T2]

## R3 — All review comments addressed before merge

- R3.1 `safe-merge` MUST refuse to merge while any review thread has `isResolved: false` (including outdated threads). [T2]
- R3.2 A thread counts as addressed ONLY if it has at least one reply (fix description or explicit rebuttal, e.g. "not relevant because…") AND is marked resolved. Resolution-without-reply and reply-without-resolution both block, reported separately. [T2]
- R3.3 The wrapper MUST print the unresolved threads (path, author, preview) so the operator can act, and point at `resolve-review-threads` / the github-thread-resolver skill. [T2, principle 2]
- R3.4 On repos where rulesets are enforced, required-conversation-resolution MUST also be on server-side. [T1]

## R4 — No escape hatches

- R4.1 The wrapper has no bypass flags (R1.3); its predicate is not configurable below the floor defined here (per-repo config may only ADD requirements). [T2]
- R4.2 Falling back to raw commands MUST be blocked for agents: `gh pr merge` (all spellings), REST/GraphQL merge mutations, and merges of PR branches into main via raw `git` push paths already covered by safe-push/branch rules. [T3]
- R4.3 Break-glass exists but MUST be: a separate subcommand, interactive-TTY-only (agents cannot drive it), require a typed reason, write an append-only audit record, post the reason as a PR comment, and file a tracker issue. It is slower than compliance by construction. [T2/T4]
- R4.4 Disabling/weakening the enforcement itself (settings deny rules, hooks, ruleset config) MUST route through the dotfiles REVIEW.md strict gate / PR review — never a live edit. [T3, existing governance]

## R5 — The wrapper is THE ONLY way to merge

- R5.1 All agent merge attempts route to `safe-merge` via exit-2 guidance (R1.2, R4.2); the wrapper binary/invocation is allow-listed so the compliant path needs no per-call approval. [T3]
- R5.2 The wrapper is installed on PATH (`~/go/bin`) by `make install-safe-merge`; never into chezmoi-managed hook dirs. [T2]
- R5.3 Human web-UI merges on public repos hit the same server-side ruleset (R1.1). On free private repos the web UI cannot be blocked — accepted residual risk, mitigated by T4 audit. [T1/T4]

## R6 — Regression detection (derived from W0 success criteria)

- R6.1 A recurring audit MUST scan all tracked repos' merged PRs for: unresolved threads at merge, merge-before-checks-complete, direct pushes to main; output a report and file beads for violations. [T4]
- R6.2 Public repos SHOULD additionally run a post-merge GitHub Action performing the same validation per merge (detect + alert, never auto-revert). [T4]
- R6.3 Every detected violation triggers a DEAR retro entry per principle 3. [T4]

## Traceability

| User requirement | Primary | Backstop |
|---|---|---|
| Disable --admin entirely | R1.1 (server) | R1.2/R1.4 (agent), R6 (audit) |
| Wait for automated comments | R2.1–R2.3 (wrapper) | R3.4 (server), R6 |
| Address all comments | R3.1–R3.2 (wrapper) | R3.4 (server), R6 |
| No escape hatches | R4.1–R4.3 | R4.4 (governance) |
| Wrapper is the only way | R5.1 (hook) | R1.1 (server), R6 (audit) |
