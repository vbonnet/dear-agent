# DEAR Retro: 9-PR Cascade Cleanup + CI-Health Monitor

**Date:** 2026-05-10
**Severity:** Medium (signal-loss recurrence; same root cause as 2026-05-09)
**Status:** Resolved — cascade cleared, CI-health monitor shipped

This is a follow-on to [`2026-05-09-ci-red-streak.md`](./2026-05-09-ci-red-streak.md).
That retro proposed five fixes; only one (the Go 1.26.3 + bash-exception PR
itself) shipped. This retro ships fix #1 — CI-health alerting — and audits the
cascade backlog that built up while the proposal sat unwired.

## Define

**The invariant:** PRs SHOULD merge on a tight cadence; a stale PR backlog is
a signal that something is wrong with the merge pipeline (red CI, missing
review, lost agent context).

**Today's snapshot, before this session:**

| PR  | Age (days) | State                              |
|-----|-----------:|-------------------------------------|
| #53 | 6          | DIRTY — 6716/6396 line lint refactor |
| #82 | 5          | UNKNOWN, red CI                     |
| #86 | 0          | UNKNOWN, red CI                     |
| #87 | 0          | UNKNOWN, red CI                     |
| #88 | 0          | MERGEABLE, red CI (the unblock PR)  |
| #89 | 0          | UNKNOWN, red CI                     |
| #90 | 0          | MERGEABLE, red CI                   |
| #91 | 0          | UNKNOWN, red CI                     |

Eight open PRs, all red, with the actual CI-fix PR (#88) sitting in the queue.
The 5/9 retro had identified the cause; nothing landed for ~24 hours.

## Enforce

**What broke (process, not code):**

The 5/9 retro proposed CI-health alerting as fix #1 but did not ship the
workflow. Per memory `dear-agent-retro-followthrough.md` ("retros need
wired-up patches, not just designs"), this is the same failure mode the
2026-05-01 worktree retro had: design without implementation = nothing
deterministic = silent regression.

The deeper issue is recursive: a retro about agent-judgment-dependent
processes was itself implemented in an agent-judgment-dependent way.

## Audit

**Cascade executed this session (in order):**

1. #88 — Go 1.26.3 + bash exception + 5/9 retro (the actual CI fix)
2. #90 — `captureStdout` drains until EOF (fixes `TestAnnounceAcceptanceCriteriaPrintsBanner`)
3. #87 — `cleanup-worktrees.sh` (now passes bash policy thanks to #88's exception)
4. #86 — audit outcomes framework + spec.coverage check
5. #89 — VerifierProvider seam (Phase 6.6)
6. #91 — recursive self-improvement benchmark scaffold
7. #82 — loop command + cost/model_variant + README rewrite
8. #53 — 6716-line lint cleanup (surprise: still merged cleanly after 6 days)

Pre-existing e2e test bitrot (`TestStatusLineE2E_*`, `TestAGM/version`) still
red — these were red before today's session and remain red. Tracked as
follow-up; the 5/9 retro's fix #5 (quarantine) covers this.

**Wired up this session:** `.github/workflows/ci-health-monitor.yml` — runs
every 6 hours, checks the last 5 CI conclusions on `main`, opens or updates
a `ci-red` issue if ≥3 of the last 5 are failures. Honors the 5/9 retro's
fix #1 as a deterministic signal.

## Retro

**The compounding failure pattern (fix-the-meta):**

Two retros in nine days have hit the same root cause: a deterministic-signal
gap dressed as a list of TODOs. This is the third instance (worktree
cleanup, CI red streak, cascade backlog). The pattern is:

> Agent identifies risk → writes retro → proposes alerting → does not wire
> alerting → next session inherits the same risk → writes another retro.

**Forcing function adopted here:** when the next retro proposes a
"monitoring" or "alerting" fix, the PR that contains the retro MUST also
contain the workflow / hook that implements it. Without that, the retro is
deferred-cost, not progress.

**Brain-v2 / vbonnet.ai status (out-of-cascade triage):**

- brain-v2: PR #10 (5-tier permissions) merged. PRs #9, #15, #16 are DIRTY
  (conflict with main); rebases needed.
- vbonnet.ai: CI green on main (last 5 deploys ✓). PRs #2, #4 conflicting;
  PR #1 mergeable but out of session scope.

## Action items (status)

| #  | Action                                            | Status   |
|----|---------------------------------------------------|----------|
| 1  | CI-health alerting workflow                       | ✅ shipped (this PR) |
| 2  | govulncheck in main `CI` workflow                 | TODO     |
| 3  | Auto-bump `go` directive in go.mod                | TODO     |
| 4  | Bash-policy: print exact INSERT snippet on miss   | ✅ shipped (ce-6as.48, PR #304) |
| 5  | Quarantine broken e2e tests with `t.Skip` + issue | TODO     |

Items 2-5 are inherited from the 5/9 retro and remain unwired. Per the
forcing function above, the next retro that touches CI process MUST land
at least one of them as code.
