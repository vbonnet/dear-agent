# DEAR Retro: A Red-Lint Commit Reached `main` via Direct Push

**Date:** 2026-05-29
**Severity:** Medium (one lint-red commit on `main` blocked an unrelated PR;
no bad code shipped, but the merge gate was bypassed)
**Status:** Resolved for the symptom (PR #166 fixed the lint, PR #165
unblocked). Root-cause vector — admin direct-push bypassing required checks —
is the same trade-off the [2026-05-10 retro](./2026-05-10-ci-red-and-unguarded-merges.md)
documented and left open.

## Define

**The trigger.** PR #165 (`docs/atomic-action-wrapper`, a docs-only change)
sat with red `Build & Test (ubuntu-latest)` and `Build & Test (macos-latest)`.
The failures had nothing to do with #165 — they were inherited from `main`:
`cmd/chezmoi-deploy/main.go` had three lint findings that fail the `Lint` step
inside the `Build & Test` job (gocyclo: `run()` complexity 24 > 15; gosec
G602 slice-bounds; gosec G702 command-injection taint).

**The question this retro answers:** *was CI not a required check? did the
lint config drift?*

## Enforce (root cause)

Neither of the two hypotheses in the ask is the cause. The facts:

1. **Lint IS gated, and IS required.** `golangci-lint` runs as the `Lint`
   step inside the `ci` job, whose check name is `Build & Test (<os>)` —
   which is in the branch-protection required-status-checks list. A PR with
   red lint cannot *merge*.

2. **No config drift was needed.** The offending commit's own CI was red at
   push time. We don't need a moved linter version to explain it (though
   `ci.yml:74` does pin `version: latest` for golangci-lint — an unpinned
   floating version is a *latent* drift vector, not today's cause).

3. **The commit reached `main` by direct push, not a PR.**

   ```
   gh api repos/vbonnet/dear-agent/commits/48efbefdb0/pulls   → []  (no PR)
   gh api .../commits/48efbefdb0/check-runs:
     Build & Test (ubuntu-latest) → failure
     Build & Test (macos-latest)  → failure
   ```

   The commit `48efbefdb0 cmd/chezmoi-deploy: atomic chezmoi apply + commit
   + push` was pushed straight to `main`. Its CI ran *after* the push, came
   back red, and simply sat there — because **required status checks only
   gate PR merges; they do not gate direct pushes by an admin when
   `enforce_admins` is off.**

This is the exact trade-off the 2026-05-10 retro called out and accepted, in
its own words:

> `enforce_admins` is left **off** intentionally — this is a solo-dev repo,
> and a self-locked admin can't recover from a busted required check. The
> trade-off is documented here so it's not silently re-enabled later without
> thought.

The trade-off held its end of the bargain: `enforce_admins: off` is precisely
what let an admin push bypass the lint gate. It was exercised not maliciously
but by the ordinary act of committing the `chezmoi-deploy` tool directly to
`main` instead of through a PR. (Compounding signal: the repo's own
`CLAUDE.md` already carries a "route to a PR, don't direct-push protected
`main`" rule — this commit predates or ignored it.)

## Audit

- **PR #166** — fixed all three findings (extracted `parseArgs` to drop
  `run()` complexity; read `argv[i+1]` before advancing `i` so the bounds
  guard dominates the access and clears G602 with no suppression; the
  refactor also broke the taint chain so G702 stopped firing — a `#nosec`
  added then removed once it proved to be a no-op). Squash-merged green.
- **PR #165** — server-side rebased onto the fixed `main`
  (`gh pr update-branch --rebase`, since `required_linear_history: true`
  forbids a merge-in and local force-push to a branch is denied here),
  CI re-run, then merged.

## The recurring lesson

The 2026-05-10 retro's closing line was: *"a retro that proposes a process
fix without an enforcement mechanism is a retro that will recur."* The
enforcement it shipped (required checks) does work — for PRs. The gap it
*documented and accepted* (admin direct-push) is what recurred. Three weeks
later, the accepted trade-off cost one red `main` and one blocked downstream
PR. That is cheap, and the documented rationale (solo-dev admin lockout
recovery) is still valid — so this is a data point, not a mandate to flip
`enforce_admins` on.

## Follow-ups (not done here — would be scope creep on the lint fix)

1. **Push protection, not just merge protection.** Two low-friction options
   that preserve admin-lockout recovery:
   - A `pre-push` hook (the repo already has `make install-preflight-hook`,
     PR #162) that refuses to push to `main` directly — local, advisory,
     and trivially overridable in a real emergency.
   - A lightweight CI-health alert specifically on `push` events to `main`
     that go red (the 2026-05-10 monitor reports red main; confirm it fires
     for direct pushes, not only PR merges).
2. **Pin golangci-lint.** `ci.yml:74` `version: latest` means the rule set
   can shift under the repo with no commit. Pin to a known version and bump
   deliberately (this is also noted in the 2026-05-27 preflight retro, R.4).
3. **Make lint a first-class required check.** It's currently invisible —
   buried as a step inside `Build & Test`. A red lint and a red test look
   identical from the outside. A separate `Lint` job/check would make
   "lint is gating" legible and independently required.

---

**Provenance:** required-checks list and `enforce_admins: false` confirmed
via `gh api repos/vbonnet/dear-agent/branches/main/protection` on 2026-05-29;
direct-push + red-CI confirmed via the `commits/48efbefdb0/{pulls,check-runs}`
APIs above.
