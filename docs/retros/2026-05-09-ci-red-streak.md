# DEAR Retro: 14-Day CI Red Streak on main

**Date:** 2026-05-09
**Severity:** High (lost regression-detection signal; merge-by-default policy let breakage compound)
**Status:** Partially resolved — fixes landed for known root causes; monitoring/alerting still TODO

---

## Define

**The invariant that was violated:**

`main` SHOULD be green. CI is a regression-detection signal; when `main`
stays red, every subsequent PR inherits noise that masks new failures, and
the team loses its only automated answer to "did my change break
anything?"

**Scope of the breakage:**

```
Last green CI run on main: 2026-04-25T00:04Z
First red CI run on main:  2026-04-25 (later that day)
Today (2026-05-09):        still red — 14 days

CI runs on main, last 100: 88 failure / 6 success
                           50 consecutive failures most recently
```

For two full weeks every merge to `main` produced a red CI run, and the
breakage was not noticed until it became an explicit ask in this session.

---

## Enforce

**Root cause — four compounding failures:**

### 1. `gh pr merge --auto` merges immediately on this repo

There are no required status checks gating merges (memory:
`dear-agent-pr-flow.md`). `--auto` was treated as "merge when green," but
in this repo it is "merge now." The result: every PR with red CI was merged
into `main` anyway, and the broken state propagated.

### 2. New stdlib CVEs against the pinned Go version

`go.mod` pinned `go 1.26.2`. On 2026-04-25, govulncheck started reporting
8 new stdlib advisories (GO-2026-4918, 4971, 4976, 4977, 4980, 4981, 4982,
4986) all fixed in `1.26.3`. The govulncheck job uses
`go-version-file: 'go.mod'`, so bumping the local toolchain alone wouldn't
have helped — `go.mod` has to be the source of truth.

### 3. Bash size policy enforced on a moving target

The 20-line bash limit (PR #67-ish era) was a one-shot lockdown: every
existing >20-line script got an exception row in
`.github/language-policy/exceptions.db`. A new >20-line script
(`scripts/cleanup-worktrees.sh`, 286 lines, landed via PR #87) had no
exception row, so the policy job went red. Adding a script now requires
remembering to update the DB; the workflow does not auto-suggest this.

### 4. Pre-existing test bitrot ignored

`agm/test/e2e/status_line_test.go:422` checks `go.mod` for the substring
`ai-tools`. The repo was renamed to `dear-agent` months ago, so every test
in that file fails with `could not find agm repository root`. Similarly
`agm/test/e2e/testscript_test.go` requires network (`go install
github.com/vbonnet/dear-agent/agm/cmd/agm`) which doesn't work in CI's
go-mod-vendored environment. These have been red for weeks and were
treated as background noise.

**What "stayed red" means in practice:** with CI noise normalized,
new regressions (the Go CVEs, the bash policy violation) blended into the
existing failures, so the actual signal that "today's commit broke
something new" was lost.

---

## Audit

**Fixes landed in this PR:**

| Mechanism                               | What it does                                          |
|-----------------------------------------|-------------------------------------------------------|
| `go.mod` bumped 1.26.2 → 1.26.3         | Resolves 8 stdlib CVEs flagged by govulncheck         |
| Bash-size exception for cleanup-worktrees.sh | Unblocks the language-policy workflow            |
| `golang.org/x/{net,sys,term,text}` + fsnotify bumped | Catches up routine deps                  |
| Dogfooding rule in `.claude/CLAUDE.md`  | Forces use of AGM/VROOM so we hit our own tooling     |

**Verified locally:**
- `go build ./...` clean
- `go vet ./...` clean
- `govulncheck -scan package ./...` produces zero unallowed findings

**Still red (pre-existing, not fixed in this PR):**
- `agm/test/e2e/status_line_test.go` — stale `ai-tools` repo-name check
- `agm/test/e2e/testscript_test.go` — `go install` path needs network
- A handful of e2e tests requiring external infra (sqlite timeout, perf)

These need a follow-up PR. They are not blocking the same way the Go CVE +
bash policy were, because they were red *before* this PR too — but they
are why "is CI green?" cannot be a one-bit answer until they are fixed
or quarantined.

---

## Retro

**Why this happened (the deeper lesson):**

The dear-agent merge policy is unusually permissive: no required checks,
`--auto` merges immediately, and there is no human gate between "PR opened"
and "in main." That is a deliberate optimization for speed, but it relies on
*someone* noticing red CI promptly. Nobody was watching, so red persisted.

The Stop-hook lesson from the 2026-05-01 worktree retro applies almost
verbatim here: **a process that depends on agent judgment is a process that
will silently break.** That retro added a deterministic Stop hook so cleanup
no longer required the agent to remember. The CI-health analog is missing.

**Proposed fixes (prioritized):**

1. **CI-health alerting (highest leverage).** Add a scheduled GitHub Action
   that runs every 6 hours, checks the latest CI conclusion on `main`, and
   opens (or comments on) a `ci-red` GitHub issue if it has been failing for
   more than 24 hours. This is the deterministic-signal layer the worktree
   retro called out.

2. **Pre-merge govulncheck on `main`.** Add `govulncheck` to the required
   `CI` workflow (it currently lives only in `Security Scan` /
   `SBOM and Security Scan`, both PR/scheduled). Even without branch
   protection, having it in the same workflow surfaces stdlib CVE drift
   immediately, not on a weekly cadence.

3. **Auto-bump go.mod when toolchain advances.** Dependabot doesn't bump
   the `go X.Y.Z` directive in go.mod. Add a small scheduled workflow that
   reads the latest patch release from `go.dev/dl/?mode=json` and opens a
   PR if `go.mod` is behind. (Or rely on `GOTOOLCHAIN=auto` and decouple
   go.mod from CVE patches — riskier; needs deliberate decision.)

4. **Bash-policy DX: warn at PR-time on new violations.** The policy job
   reports a generic "add an exception" message. Make it list the exact SQL
   `INSERT` snippet, with a placeholder for `reason` and `sunset_date`. A
   contributor adding a script should not have to grep the workflow file
   to learn the exceptions schema.

5. **Quarantine the broken e2e tests.** Mark `TestStatusLineE2E_*` and
   `TestAGM/version` with `t.Skip("FIXME: …")` and an issue link. A flaky
   or stale test that everyone has learned to ignore is worse than no test:
   it eats signal budget.

**What to keep doing:** the DEAR retro framework itself. The 2026-05-01
worktree retro made a clean root-cause case and shipped tooling. This retro
is the same shape; both rely on the project trusting the framework enough
to actually wire up the proposed fixes (memory:
`dear-agent-retro-followthrough.md` — a retro that proposes a hook but
doesn't ship it is just a doc). The action items above must land as code,
not just commitments.
