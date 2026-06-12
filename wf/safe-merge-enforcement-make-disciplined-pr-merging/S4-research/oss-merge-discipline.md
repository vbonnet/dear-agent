# S4 Research — Merge Discipline in Major OSS Projects

> Produced by a research subagent, 2026-06-11. Sources cited inline.

## 0. The founding idea: the "Not Rocket Science Rule"

Every system below descends from one rule, articulated by Ben Elliston and implemented for Rust by Graydon Hoare around 2013:

> **"Automatically maintain a repository of code that always passes all the tests."**

The crucial nuance: testing must happen *before* integration, against a candidate state that is exactly what the branch will become — not after merge, and not against a stale base. A bot evaluates every change, merges/rebases it with the latest tip, builds and tests that result, and only then fast-forwards the tip ([Tyler Cipriani](https://tylercipriani.com/blog/2022/02/17/its-still-not-rocket-science/), [Mergify's history of merge queues](https://mergify.com/blog/the-origin-story-of-merge-queues), [Jane Street](https://blog.janestreet.com/making-never-break-the-build-scale/)).

The corollary all four big systems enforce: **if a human can click merge, the rule is unenforceable**, because a human merge bypasses the "test the exact future tip" step.

## 1. Kubernetes: Prow + Tide

No human merges in kubernetes/kubernetes. Tide continuously queries GitHub for PRs matching merge criteria and merges them itself ([Prow/Tide for Contributors](https://www.kubernetes.dev/blog/2022/12/12/prow-and-tide-for-kubernetes-contributors/), [Tide config](https://docs.prow.k8s.io/docs/components/core/tide/config/)).

Gates: `lgtm` label + `approved` label (from the relevant **OWNERS** file — two distinct roles), no blocking labels (`do-not-merge/hold`, `needs-rebase`, ...), presubmits passing **against the latest base commit** — Tide retests before merging if existing results are stale.

How humans are kept out: the [Tide maintainer's guide](https://docs.prow.k8s.io/docs/components/core/tide/maintainers/) is explicit: *"Don't let humans (or other bots) merge"* — every manual merge invalidates all currently-running tests in the pool. Human intent is expressed entirely through **comments that become labels**; the merge is the bot's job. OWNERS files put "who may approve what" **in the repo, versioned and reviewable**.

## 2. Rust: bors → Homu → new bors + merge queues

Graydon's bors (2013) → Homu (run as `@bors`) → from-scratch Rust rewrite [rust-lang/bors](https://github.com/rust-lang/bors); some repos (e.g. [Cargo](https://github.com/rust-lang/cargo/pull/14718)) moved to GitHub's native merge queue ([Rust Forge bors docs](https://forge.rust-lang.org/infra/docs/bors.html)).

The r+ workflow: a listed reviewer comments `@bors r+`. Bors merges PR + current master on a staging branch, runs the **full test suite on that exact candidate commit**, and only on success fast-forwards master. The [rustc dev guide](https://rustc-dev-guide.rust-lang.org/contributing.html): **"PRs are never merged by hand."** Rollups (`r+ rollup`) batch small changes; `@bors try` = CI run with no merge intent.

## 3. Chromium: the Commit Queue (CQ)

Gerrit label votes: **dry run** = `Commit-Queue +1` (tests, never submits); **full run** = `Commit-Queue +2` (if green, **the CQ submits the CL itself**). Serious flake mitigation: retry failing shards, re-run failures *without* the patch, fail only if the patch is implicated ([CQ docs](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/infra/cq.md)).

Bypass is **explicit, narrow, and visible in the commit itself**: footers like `No-Try: true`, `No-Tree-Checks: true`, `Ignore-Freeze: true` — each a distinct, auditable marker in history, not a silent button.

## 4. GitHub-native merge queues

[GA mid-2023](https://github.blog/news-insights/product-news/github-merge-queue-is-generally-available/): queued PRs become a `merge_group` (speculative merge of base + queue + PR), CI runs against that, success fast-forwards the base. Can be made mandatory via rulesets. Gotcha: required checks must fire on the `merge_group` event or the queue stalls.

**Availability catch:** merge queue is only for **public repos in organizations** and **private repos on Enterprise Cloud** — not personal repos at all ([#51483](https://github.com/orgs/community/discussions/51483), [#131130](https://github.com/orgs/community/discussions/131130)). Branch protection on private repos requires Pro+; **free-plan private repos get essentially nothing server-side**.

## 5. Third-party merge bots

| Tool | Status (mid-2026) | Free-tier / private-repo story |
|---|---|---|
| [bors-ng](https://github.com/bors-ng/bors-ng) | **Deprecated**; public instance shut down | Self-hostable anywhere, but unmaintained — a 2026 adoption is a liability |
| [Kodiak](https://kodiakhq.com/) | Maintained, low activity | **Free for public and personal repos (incl. private personal)** — the standout for a solo dev; but it *automates* merge rather than forbidding the manual one |
| [Mergify](https://mergify.com/) | Active, commercial | Free for OSS and private teams ≤5 contributors; Merge Protections still need branch protection/rulesets to be binding — which free private repos lack |
| Aviator, Trunk Merge | Active commercial | Team-oriented; not aimed at solo free-private use |

None of the hosted options fully solves *enforcement* on a free-plan private personal repo, because they all rely on branch protection to make their check binding.

## 6. Common principles

1. **Merge is an earned state transition, not a button.** The tip moves only when a defined predicate becomes true. The merge event is an *output* of policy evaluation, never a human gesture.
2. **Checks are validated against the exact future tip, not the PR snapshot.** A green check on a stale base is worthless.
3. **Humans express intent; automation performs the act.** `/lgtm`, `@bors r+`, `CQ+2` — declarations recorded as labels/votes, auditable and revocable.
4. **Policy lives in versioned config, not in heads or UI settings.** OWNERS files, Tide queries, Mergify YAML.
5. **Bypass exists, but as a distinct, narrow, audited path** that leaves a permanent marker in history. Never the same gesture as normal merging.
6. **Manual merges actively damage the system, not just the rule** — Tide frames the prohibition as protecting shared throughput, which is more durable than "policy says so."
7. **Flake handling is a first-class policy concern.** A gate that fails closed on flaky tests trains users to bypass it; the systems that survived invested heavily here.

## 7. Solo developers, small teams, and AI agents

- **Layered, deterministic enforcement beats agent compliance** ([microservices.io GenAI guardrails, 2026](https://microservices.io/post/architecture/2026/03/09/genai-development-platform-part-1-development-guardrails.html)): agent-readable checklist + deterministic pre-execution hook + server-side CI + automated review — "coding agents cannot be relied upon to consistently follow instructions."
- **Agents will find `--no-verify` and other escape hatches** ([Steve Kinney](https://stevekinney.com/courses/self-testing-ai-agents/making-it-hard-to-cheat-the-guardrails)): defense in depth "so that no single shortcut is enough." Local hooks are not a security boundary.
- **Solo devs need PRs more with agents, not less** (["Stop Letting Agents Push to Main"](https://dev.to/ticktockbent/stop-letting-agents-code-push-to-main-2kfk)): the review gate substitutes for line-by-line understanding the solo dev no longer has.

## Transferable design principles for `safe-merge`

1. **Make the wrapper the only merge gesture; deny raw `gh pr merge`.** "Humans never click merge" becomes "agents never run merge." Phrase the denial as the sanctioned path + why.
2. **The wrapper evaluates a merge predicate, then acts atomically** — checks green on the current head SHA, threads resolved, no hold-class label, then merge + cleanup as one unit.
3. **Re-validate against the latest base — never trust a stale green.** Addresses two failure modes already in memory: stale-base PRs after main rollbacks, and the merge-tree false-safe on split refactors (the `git log <merge-base>..main -- <file>` check belongs *inside* the wrapper).
4. **Separate intent from execution, even solo** — an explicit, recorded approval artifact distinct from the merge act.
5. **Bypass must be a different, louder path** — a separate `--emergency` mode (or binary) demanding a reason string, writing an append-only audit record. Strip `--admin`/`--force` from the normal path by construction (the safe-push pattern).
6. **Defense in depth, honestly assessed** — client layers stop accidents and drift, not determined adversaries; pair with post-hoc audit.
7. **Invest in flake ergonomics or the wrapper gets routed around** — known-flaky list + sanctioned single-rerun, so "CI is flaky" never justifies bypass.
