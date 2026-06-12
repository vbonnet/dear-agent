# Design — safe-merge: Disciplined PR Merging as the Only Path

**Date:** 2026-06-11 · **Status:** DRAFT — awaiting review (do not implement until reviewed)
**Problem statement:** [w0-speed-merging.md](w0-speed-merging.md)
**Wayfinder project:** `wf/safe-merge-enforcement-make-disciplined-pr-merging/` (D1 charter, D2 solutions, D3 approach, D4 requirements, S4 research artifacts)

---

## 1. Problem (summary — full statement in W0)

PRs are speed-merged: `--admin` bypasses, unresolved bot review threads,
merges before checks finish. Concrete trigger: dotfiles PR #15 merged
2026-06-11 over a **security-high, fail-open** Gemini finding (missing
`jq` silently disables the Gmail-MCP block hook) plus two more unresolved
threads, all 13 hours old at merge. A 48-hour audit found 9 issues across
8 repos. Free-plan private repos have no server-side protection at all;
dear-agent has protection but `enforce_admins: false`. Skipping checks is
currently one flag; waiting is ~10 minutes. **Goal: invert that.**

## 2. Requirements

Full decomposition with traceability: [D4-requirements.md](../wf/safe-merge-enforcement-make-disciplined-pr-merging/D4-requirements.md). The five user requirements:

1. **Disable `--admin` bypass entirely** — prevent, not discourage.
2. **Enforce waiting** for CI and automated reviewers (Gemini) to finish.
3. **All review comments addressed before merge** — fixed or explicitly
   rebutted ("not relevant because…"), replied AND resolved.
4. **No escape hatches** — no `--force`/`--skip`, no raw-command fallback.
5. **The wrapper is THE ONLY way to merge** — raw `gh pr merge` blocked.

## 3. Research findings (full reports in `wf/.../S4-research/`)

**[gh internals](../wf/safe-merge-enforcement-make-disciplined-pr-merging/S4-research/gh-merge-internals.md):**
`gh pr merge` checks exactly one server-computed enum
(`mergeStateStatus`) client-side — no checks, no reviews, no threads. It
merges immediately through `UNSTABLE` (failing *non-required* checks —
and on free-private repos nothing is ever required). `--admin` is purely
a client-side "stop refusing" flag; the GraphQL mutation is identical.
The server enforces only: open, not draft, no conflict, allowed method,
and `expectedHeadOid` match (409 on mismatch — our TOCTOU anchor). gh's
alias and extension systems both refuse to shadow built-ins, so gh
cannot be made safe from within. **No existing gh extension does
pre-merge verification — the niche is empty.**

**[GitHub rulesets](../wf/safe-merge-enforcement-make-disciplined-pr-merging/S4-research/github-rulesets-free-plan.md):**
Repository rulesets are **fully enforced on free-plan public repos** and
support an **empty bypass list — repo admins get no implicit exemption**,
unlike classic branch protection's admin-exempt-by-default. A zero-bypass
ruleset makes `gh pr merge --admin` fail server-side, for $0. On
free-plan *private* repos rulesets are viewable but NOT enforced (Pro
$4/mo unlocks user-owned private; merge queue is never available to
user-owned repos). Circumventing a ruleset requires editing it in
Settings — a deliberate, audited act, not a merge-time flag, which is
exactly the break-glass shape we want; it also answers the original
reason `enforce_admins` was left off (lockout recovery).

**[OSS merge discipline](../wf/safe-merge-enforcement-make-disciplined-pr-merging/S4-research/oss-merge-discipline.md):**
Kubernetes (Tide), Rust (bors), Chromium (CQ) all converge on the same
principles: *merge is an earned state transition, not a button*; checks
are validated against the exact future tip; humans express intent,
automation performs the act; bypass is a distinct, louder, audited path
(Chromium's `No-Try:` footers); and **flake ergonomics are existential**
— a gate that fails closed on flaky tests trains users to bypass it.
Hosted options don't fit (merge queue unavailable, Kodiak doesn't forbid
manual merges, bors-ng deprecated, Mergify needs binding protection).

**[Local stack](../wf/safe-merge-enforcement-make-disciplined-pr-merging/S4-research/local-enforcement-stack.md):**
The pattern is already proven here: `safe-push` + `chezmoi-deploy` are
Principle-9 atomic wrappers with raw forms denied. `internal/fsguard`
(PreToolUse bash guard) parses compound commands robustly — but **has
zero `gh` awareness today**, and the **`Bash(gh api:*)` allow rule
permits REST/GraphQL merges outright**. The only thing currently
standing between an agent and `gh pr merge` is the auto-mode permission
classifier — a heuristic, not a rule. `cmd/resolve-review-threads`
already implements the GraphQL thread primitives. Two deployment traps:
Go binaries must not be installed into chezmoi-managed hook dirs
(clobbered on next `chezmoi apply`), and `/opt/homebrew/bin` is first on
PATH (a `gh` shim wouldn't win).

## 4. Architecture

Four tiers, instantiating the existing instruct → enforce → verify
philosophy. Every user requirement is covered by at least two
independent tiers (D4 traceability table).

```
            ┌────────────────────────────────────────────────────────┐
  instruct  │ CLAUDE.md / AGENTS.md rules; every block emits a       │
            │ teaching message: "right way is X, because Y"          │
            ├────────────────────────────────────────────────────────┤
            │ T1  SERVER   zero-bypass rulesets on public repos      │
            │              (checks + PR + conversation resolution +  │
            │              squash-only; bypass_actors: [])           │
  enforce   │ T2  WRAPPER  safe-merge: predicate → wait → atomic     │
            │              merge with expectedHeadOid                │
            │ T3  AGENT    fsguard checkGh (exit 2) + deny rules +   │
            │              allow-listed wrapper                      │
            ├────────────────────────────────────────────────────────┤
  verify    │ T4  AUDIT    merge-audit cron + post-merge Action on   │
            │              public repos + DEAR retro on violations   │
            └────────────────────────────────────────────────────────┘
```

### 4.1 T1 — Server-side rulesets (public repos)

One branch ruleset on the default branch of every public repo
(dear-agent first):

- Require a pull request before merging; require all conversations
  resolved; required status checks (the repo's CI job names); require
  linear history; block force pushes; required merge method: squash.
- **`bypass_actors: []`** — nobody, including the owner, can merge past
  it. `gh pr merge --admin` then fails server-side (R1.1).
- Managed as JSON in-repo (`.github/rulesets/main.json`) and applied via
  `gh api` so the config is versioned and drift-detectable by the T4
  audit (the phantom-Trivy retro showed required-check config drifts).
- Classic branch protection is retired on repos where the ruleset is
  active (one source of truth).
- Private repos (dotfiles, brain-v2, engram): rulesets are created but
  unenforced on Free. **Decision for user:** GitHub Pro ($4/mo) would
  make them binding. The design does not depend on it — T2/T3 carry
  those repos.

### 4.2 T2 — The `safe-merge` wrapper

`cmd/safe-merge` (thin main) + `internal/safemerge` (testable policy),
mirroring `safe-push`/`internal/safegit`. Go, per principle 4.

**Interface:**

```
safe-merge [pr-number|branch|url] [--repo owner/name]
           [--timeout 45m] [--dry-run]
safe-merge break-glass <pr> --repo ...        # human-only, see 4.4
```

No `--admin`, `--force`, `--skip*`, `--no-verify` — unknown flags are
rejected with the teaching message, and these specific flags get a
targeted explanation (R1.3/R4.1). Merge method is squash, always
(matches repo policy; not configurable below the floor).

**The merge predicate** (all must hold; evaluated from one GraphQL query
— the exact query is in the gh-internals research doc):

| Gate | Pass condition | On failure |
|---|---|---|
| PR state | OPEN, not draft | refuse, explain |
| Conflicts | `mergeable != CONFLICTING`, `mergeStateStatus != DIRTY` | refuse: rebase first |
| Freshness | `mergeStateStatus != BEHIND`; base hasn't moved in a way that invalidates checks → if BEHIND, instruct update + re-verify (Not-Rocket-Science rule: never trust a stale green) | refuse with update instructions |
| Checks | **every** check run / status context on the head SHA is COMPLETED with conclusion SUCCESS (or policy-allowed NEUTRAL/SKIPPED); not just `isRequired` ones — `UNSTABLE` must not pass (R2.2) | wait (see below) or refuse on hard failure |
| Bot reviewers | each expected reviewer (per-repo config) has submitted a review/comment **newer than the head SHA's push**; absence after timeout is a distinct "reviewer never arrived" outcome, surfaced, never silently passed (R2.3) | wait, then refuse with status |
| Threads | `reviewThreads[*].isResolved == true` for all (incl. outdated), **and** every resolved thread has ≥1 reply after the root comment (R3.2 — resolution-without-reply blocks; reply-without-resolution blocks; reported separately) | refuse; print each unresolved thread (path, author, preview); point at `resolve-review-threads` |
| UNKNOWN | `mergeStateStatus == UNKNOWN` → re-poll with backoff, never pass (gh itself gets this wrong) | wait |

**Waiting is built in (R2.1):** when checks are pending or an expected
reviewer hasn't arrived, safe-merge does not exit 1 and leave the
operator tempted — it **watches** (poll with backoff, `--timeout`
default 45m), printing progress. The compliant path costs zero extra
keystrokes compared to the old `--admin` path; it just takes wall-clock
time, which is the point.

**Flake ergonomics (R2.4):** a per-repo known-flaky list (the existing
CI-flakes memory, codified in config). A failed check on that list may
be re-run **once** via `gh run rerun --failed`; a second failure is
real. This is the release valve that keeps "CI is flaky" from becoming
the bypass justification — every surviving OSS system learned this.

**Atomic merge (TOCTOU-safe):** the predicate snapshot captures
`headRefOid`; the merge executes `gh pr merge --squash
--match-head-commit <oid> --delete-branch`. A racing push → server 409 →
safe-merge re-verifies from scratch. After merge: confirm merged state,
then worktree/branch cleanup per the existing §5 discipline (reusing the
Stop-reaper logic where available).

**Audit log:** every invocation appends one JSONL record (predicate
snapshot, outcome, durations) to `~/.local/state/safe-merge/audit.jsonl`
— the T4 audit and DEAR retros read this.

**Config:** `.safe-merge.yml` at repo root (expected reviewers, flaky
list, extra requirements). Per-repo config can only **add** requirements;
the floor in this doc is not configurable away (R4.1). Repos without
config get the floor.

### 4.3 T3 — Agent enforcement (the "only way" guarantee)

1. **`internal/fsguard` gains `checkGh`** (modeled on `checkGit`),
   wired into the existing `pretool-bash-write-guard` PreToolUse hook.
   Blocks, with exit 2 and a teaching message pointing at `safe-merge`:
   - `gh pr merge` — any flags, any arg order, including via `cd`,
     `bash -c`, `eval` (the fsguard parser already handles these);
   - `gh api` calls whose path matches `repos/*/pulls/*/merge` or whose
     GraphQL body contains `mergePullRequest` / `enablePullRequestAutoMerge`;
   - `gh pr ready` + merge chains are left alone — only the merge action
     is gated.
   Block message (principle 2): *"You're trying to merge a PR directly.
   Use `safe-merge <pr>` instead — it waits for CI and Gemini, verifies
   every review thread is resolved, and merges atomically. Raw merges
   are blocked because they have shipped unreviewed security defects
   (see docs/w0-speed-merging.md). If safe-merge itself is broken, file
   a bead and report — do not work around. PERMISSION_ESCALATION: ..."*
2. **settings.json (chezmoi source, REVIEW.md strict gate):**
   - deny: `Bash(gh pr merge*)` spellings (backstop for the fail-open
     hook);
   - **narrow `Bash(gh api:*)`** to the read/comment endpoints actually
     used (this closes the wholesale REST/GraphQL bypass and also
     retires a pre-existing REVIEW.md auto-FAIL `:*` debt);
   - allow: `Bash(safe-merge *)` — the wrapper is safe by construction,
     so it is allow-listed (principle 9) and the compliant path needs no
     human approval.
3. **Deployment:** Go binaries → `~/go/bin` via `make
   install-safe-merge` (never into chezmoi-managed hook dirs — the
   2026-05-28 retro failure mode). Hook registration and deny rules →
   chezmoi source branch → REVIEW.md gate → `chezmoi-deploy`.
4. **Honest limits:** the PreToolUse hook fails open by design (deny
   rules are the backstop); the human terminal is not bound by T3 at all
   (T1 binds it on public repos; T4 catches the rest). We do not claim
   tamper-proofness against a determined human owner — the threat model
   is expedience, not malice (W0 non-goal).

### 4.4 Break-glass (R4.3)

Emergencies exist (CI provider outage with a critical fix needed). Per
Chromium/Tide practice, bypass must be a *different, slower, louder*
path — never a flag on the fast one:

- `safe-merge break-glass <pr>`: **requires an interactive TTY**
  (refuses if stdin/stdout are not terminals) — agents' Bash tool
  cannot drive it, making it structurally human-only. No flag can
  disable the TTY check.
- Prompts for a typed reason (min length), then: posts the reason as a
  PR comment, appends a break-glass record to the audit log, files a
  P1 bead (`bd add`) for the post-hoc review, and only then merges.
- On public repos T1 still applies: the ruleset will refuse even
  break-glass — there the true break-glass is editing the ruleset in
  Settings, which GitHub itself audits. safe-merge prints exactly that
  instruction instead of attempting the merge.
- Every break-glass use triggers a DEAR retro entry (T4 checks the
  audit log).

### 4.5 T4 — Detection and regression prevention

1. **`merge-audit`** (new subcommand or `cmd/merge-audit`): re-runs the
   48h audit permanently — for each tracked repo, scan merged PRs for:
   unresolved threads at merge time, checks incomplete/red at
   `mergedAt`, direct pushes to the default branch (commit with no
   associated PR), break-glass records. Output: report to
   `~/src/engram-research` (per output routing) + a bead per violation.
   Run weekly via the existing scheduled-agent infrastructure.
2. **Post-merge Action on public repos:** on push to main, validate the
   merged PR satisfied the predicate; on violation, open an issue and
   fail a visible check on main. Detect + alert only — **no
   auto-revert** (an auto-reverter is itself a dangerous unattended
   actor, and reverts on main are exactly the irreversible action this
   project exists to gate).
3. **Ruleset drift detection:** merge-audit compares live ruleset JSON
   against `.github/rulesets/main.json` (the phantom-Trivy failure
   class).
4. **DEAR loop:** every violation or break-glass use produces a retro
   entry per principle 3; the retro's action items become beads.

## 5. Implementation plan

Phases are independently shippable, ordered by leverage-per-effort.
Each phase = one scoped plan, one agent, one PR (principle 1). Beads
filed under label `safe-merge`: P1 `ce-5i6o`, P2 `ce-nwlf`, P3 `ce-ebt4`
(blocked by P2), P4 `ce-3k3o` (blocked by P2), P5 `ce-j2m5` (blocked by
P2), P6 `ce-5vog`.

| Phase | Deliverable | Notes |
|---|---|---|
| **P1 — Server rulesets (quick win, $0)** | Zero-bypass ruleset on dear-agent main (+ other public repos), config as `.github/rulesets/main.json`, applied via `gh api`; retire classic BP there | Kills `--admin` server-side where most merges happen, before any code ships. Needs care: roll out with the user present, since it binds the user too (lockout-recovery: ruleset edit via Settings). |
| **P2 — `safe-merge` MVP** | `cmd/safe-merge` + `internal/safemerge`: predicate (checks-all-green, threads-resolved-with-reply, freshness, TOCTOU merge), watch-mode waiting, audit log, `--dry-run`; `make build/install-safe-merge`; unit tests against recorded GraphQL fixtures | Reuses `resolve-review-threads` GraphQL code (promote to `internal/`), `safegit` patterns. DoD includes: committed, pushed, preflight green. |
| **P3 — Agent enforcement** | `internal/fsguard` `checkGh` + tests; dotfiles branch: deny rules, `gh api` narrowing, hook registration, `Bash(safe-merge *)` allow | Two PRs: dear-agent (fsguard) and dotfiles (settings — REVIEW.md strict gate applies). Only after P2 ships, so the teaching message points at something real. |
| **P4 — Reviewer-wait + flake valve** | Expected-reviewer config + wait logic (Gemini detection), known-flaky list + single-rerun | Separated from P2 because reviewer-arrival heuristics need tuning against real Gemini latency. |
| **P5 — Break-glass** | TTY-only subcommand, reason capture, PR comment + bead + audit record | Deliberately last among wrapper features: until it exists, the break-glass is asking the human, which is the correct default. |
| **P6 — Detection tier** | `merge-audit` (weekly scheduled), post-merge Action on public repos, ruleset drift check | Makes the W0 success criteria measurable; closes the loop. |

Out of scope for all phases (re-stated): auto-revert; paid-plan
dependency; blocking the human web UI on free private repos.

## 6. Risks and open questions

| Risk / question | Position |
|---|---|
| Zero-bypass ruleset locks the owner out during an incident | Accepted and intended: the recovery path is an audited Settings edit (slow), not a merge flag (fast). Document the procedure in-repo. |
| fsguard hook fails open (by design) | Deny rules + T1 + T4 layers backstop it; T4 audit alarms on any merge that bypassed the wrapper (no audit-log record ↔ merged PR). |
| Gemini reviewer-arrival detection is heuristic | Surfaced as a distinct gate outcome, never a silent pass; timeout configurable per repo; P4 tunes it with real data. |
| `Bash(gh api:*)` narrowing breaks legitimate workflows | Inventory actual `gh api` usage from transcripts before writing the new rules (the fewer-permission-prompts skill's scan approach); REVIEW.md gate reviews the diff. |
| Wrapper itself gets blocked by the new deny rules | Wrapper calls gh by absolute path with argv (no shell), and the hook exempts the safe-merge process; integration test covers it. |
| Private repos remain web-UI-mergeable | Accepted residual (free plan); T4 catches it; **user decision pending: GitHub Pro ($4/mo) for dotfiles/brain-v2/engram?** |
| Solo-dev `reviewDecision` is null (no human reviewer exists) | The predicate intentionally gates on *threads + checks*, not on `reviewDecision`, except where a ruleset requires approval. |

## 7. Prevention of regression

- The T4 audit is the permanent version of the audit that caught this
  (W0 success criterion: zero violations, measured weekly).
- Enforcement changes route through REVIEW.md / PR review — the gate
  that PR #15 skipped now protects the gate itself.
- This design doc + W0 live in-repo; the wayfinder project records why
  each decision was made (anti-relitigation).
- Per principle 3: any future bypass found by the audit triggers a DEAR
  retro, not an in-line patch.
