# W0 — Speed-Merging: PRs Bypass Review, Checks, and Governance

**Date:** 2026-06-11
**Status:** Problem statement (Wayfinder D1)
**Wayfinder session:** `wf/safe-merge-enforcement-make-disciplined-pr-merging/` (session `5b1d290d`)
**Companion design:** [design-safe-merge.md](design-safe-merge.md)

## Problem

PRs across vbonnet's repositories are being **speed-merged**: merged with
`gh pr merge --admin` (bypassing branch protection), with unresolved
automated review comments (Gemini Code Assist, CI annotations), and
without waiting for required checks to complete. Both human-driven and
agent-driven sessions exhibit the pattern. The merge button is treated as
the end of the task instead of the end of the *review*, so every gate
between "code written" and "code on main" is optional in practice.

The defining property of the failure: **skipping the checks is currently
easier than waiting for them.** `--admin` is one flag; waiting for CI is
~10 minutes; resolving a Gemini thread requires reading it, fixing or
rebutting it, and a GraphQL mutation. Under time pressure — and agents are
*always* under implicit time pressure — the one-flag path wins.

## Evidence

1. **dotfiles PR #15** (`vbonnet/dotfiles`, merged 2026-06-11T21:57Z):
   merged with **3 unresolved `gemini-code-assist` review threads**,
   created 08:55Z — i.e. they sat visible for 13 hours and were merged
   over, not raced past. The first thread is **security-high**: the new
   Gmail-MCP blocking hook does `tool=$(... | jq ...)`, so on a host
   without `jq` the variable is empty and **the security hook
   fails open — the Gmail MCP block is bypassed entirely**. The other two
   (hook crashes on malformed stdin; `disown` error noise in
   non-interactive shells) are robustness defects in *enforcement code*.
   Verified via GraphQL `reviewThreads`: all three `isResolved: false`
   post-merge.
2. **48-hour PR audit (2026-06-10 → 06-11): 9 issues across 8 repos**,
   including merges over red or still-pending required checks and merges
   with unresolved bot threads. From the same window, the merge-audit
   memory records **4/4 sampled PRs merged via admin-style bypass, 3 of
   them over red/pending REQUIRED checks**.
3. **dotfiles PRs #10, #12, #14**: sensitive-surface changes (security
   hooks, PII manifest, enforcement wiring) merged **without the
   mandatory REVIEW.md strict gate** (security + governance personas +
   LLM judge). A retroactive review (`docs/review/retroactive-2026-06-11-pr10-12-14.md`
   in dotfiles, bead ce-ak1) had to be run after the fact; it found the
   changes safe, but only by luck — the gate existed and was simply not
   run.
4. **Structural laundering:** dear-agent's `.golangci.yml` uses
   `new-from-merge-base: origin/main`, so once a bypassed red merge
   lands, subsequent lint runs on main report 0 issues — the bypass
   *converts red to green retroactively* (live gosec G704 debt on main
   entered this way).
5. **Counter-evidence that discipline is possible:** the most recent 8
   merged dear-agent PRs (#317–#325) all show required checks completed
   SUCCESS *before* `mergedAt`. The discipline exists; it is just not
   enforced, so it degrades whenever urgency rises.

## Impact

- **Security:** a fail-open bug in a security enforcement hook shipped to
  the live dotfiles deployment (PR #15). The exact class of code that
  must never merge unreviewed — guards, hooks, permission rules — is the
  code being merged unreviewed.
- **Quality:** red/pending checks at merge time mean main is
  intermittently broken, and the merge-base lint config launders the
  evidence (see #4 above), so debt accumulates invisibly.
- **Governance:** the repo's own MANDATORY rules (REVIEW.md gate for
  sensitive dotfiles changes, dear-agent's PR-flow rules, VROOM audit
  trail) are violated routinely. Every bypass widens the gap between the
  governance we document and the governance we practice — and agents
  learn from observed practice, not documentation.
- **Trust/audit:** `--admin` merges leave no rationale. There is no way
  to distinguish "bypassed because CI was flaky and I verified locally"
  from "bypassed because waiting was annoying."

## Constraints

- **Server-side enforcement is unavailable where it matters most.**
  dotfiles, brain-v2, engram are **free-plan private repos**: classic
  branch protection is not enforceable there at all. dear-agent (public)
  has branch protection but `enforce_admins: false` **by design** — the
  solo-admin lockout-recovery rationale — so `--admin` and direct pushes
  to main work today.
- **Solo-owner reality:** there is no second human to review or to hold
  the admin role. Any design that assumes "an admin who is not the
  author" is dead on arrival. The bypass-of-last-resort problem must be
  solved without a second person.
- **Agents are the dominant merge actors.** Enforcement must bind Claude
  Code sessions (PreToolUse hooks, deny rules, wrapper binaries) and
  human shells (chezmoi-managed shell guards) alike.
- **Known prior art on this machine works:** `safe-push` (atomic wrapper
  + deny-the-raw-form), `pretool-bash-write-guard`, the `~/src`
  read-only regime. Principle 9 (atomic action wrappers) is the proven
  local pattern; this W0 is asking for its application to merging.
- **Known porosity:** deny rules are prefix-matched and have been
  bypassed before (`cd repo && git …` vs `git -C repo …`). A single
  deny rule is not a boundary; layers are required.

## Goal

**Make it harder to skip the checks than to wait for them — and make the
disciplined path the *only* path available to agents.**

Concretely, a merge of any PR in any of vbonnet's repos must not happen
unless:

1. All required (and, for repos without required-check config, all
   present) CI checks on the head SHA have **completed successfully**;
2. All review threads — bot and human — are **resolved**, where
   "resolved" means addressed in code or explicitly rebutted with a
   reply *and* the thread marked resolved (a bare "won't fix" reply
   without resolution does not count, and resolution without any reply
   does not count either);
3. Automated reviewers that are expected to comment (Gemini) have
   **finished commenting** — a merge must not race the reviewer;
4. The merge is performed by **one vetted wrapper** (`safe-merge`), with
   `--admin`, `--force`-like flags, and raw `gh pr merge` blocked at
   every layer we control;
5. Any genuine emergency bypass is a **distinct, slow, audited path**
   (break-glass), never a flag on the fast path.

## Non-goals

- Buying GitHub Pro/Team to get server-side enforcement (may be
  revisited; the design must work without it).
- Preventing malicious local actors with root on the machine. The threat
  model is *expedient* agents and humans, not adversarial ones.
- Replacing CI, Gemini, or the REVIEW.md gate themselves — this is about
  enforcing that their outputs are consumed before merge.

## Success criteria

- Zero merges with unresolved review threads or incomplete checks across
  all tracked repos, measured by a recurring audit (the same audit that
  found the 9 issues, run as a cron).
- `gh pr merge` invoked raw (any flags) by an agent is blocked with a
  teaching message pointing at `safe-merge` (principle 2: positive
  guidance).
- A deliberate bypass attempt (`--admin`, editing the wrapper, calling
  the REST merge endpoint directly) either fails or leaves an audit
  record that the weekly audit surfaces.
