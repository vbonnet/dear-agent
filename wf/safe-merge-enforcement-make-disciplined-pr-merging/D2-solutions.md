# D2 — Solution Space

Candidates evaluated against the W0 goal (*make skipping checks harder than waiting; disciplined path is the only path*), informed by S4 research.

## Adopted (layered — no single layer suffices)

| # | Solution | Layer | Why |
|---|----------|-------|-----|
| 1 | **Zero-bypass repository ruleset** on every public repo (required checks, require PR, conversation resolution, squash-only, empty `bypass_actors`) | Server | Free on public repos; makes `--admin` fail **server-side** — the only non-client-circumventable control we can get for $0. Circumvention = an audited Settings change, not a merge-time flag. |
| 2 | **`safe-merge` Go wrapper** (cmd/safe-merge + internal/safemerge): verifies the full merge predicate, waits for checks/reviewers, then merges atomically with `--match-head-commit` | Wrapper | Principle 9 (safe-push precedent). Research: gh verifies one enum client-side and the server enforces ~nothing on our repos → the wrapper IS the policy layer. Built-in *waiting* makes compliance easier than bypass. |
| 3 | **PreToolUse `checkGh` in `internal/fsguard`** + settings.json deny rules: block raw `gh pr merge`, REST merge (`gh api .../pulls/*/merge`), GraphQL `mergePullRequest`/`enablePullRequestAutoMerge`; exit 2 teaching message → `safe-merge` | Agent enforcement | fsguard's parser survives `cd`/quoting/`bash -c` (deny rules alone are prefix-porous). gh source closes alias/extension shadowing, so the hook is the seam. Must also narrow the `Bash(gh api:*)` allow rule (wholesale bypass today). |
| 4 | **Human-only break-glass** (`safe-merge` `break-glass` subcommand): requires interactive TTY + typed reason; appends to audit log, posts PR comment, files a bead | Bypass-of-last-resort | Resolves the "no escape hatches" vs "emergencies exist" tension: not a flag on the fast path; agents physically can't use it (no TTY); leaves a permanent record (Chromium `No-Try:` model). |
| 5 | **Detection tier**: recurring merge audit (cron) re-running the 48h audit query (merged-over-unresolved-threads, merged-before-checks, direct pushes); GitHub Action on public repos validating each merge post-hoc | Verify | The audit that found the 9 issues, made permanent. Catches what client layers miss; feeds DEAR retros. |
| 6 | *(Optional, user decision)* **GitHub Pro ($4/mo)** to extend ruleset enforcement to user-owned private repos (dotfiles, brain-v2, engram) | Server | The only way to get server-side enforcement on the private repos; cheap; not a design dependency. |

## Rejected

| Solution | Why rejected |
|---|---|
| `gh alias` / gh extension *overriding* `pr merge` | Impossible — both override paths explicitly closed in cli/cli source. (A `gh safe-merge` extension *alias-style install* of our binary is fine as packaging, not as a block.) |
| PATH shim binary named `gh` | `/opt/homebrew/bin` is first in PATH on this machine; absolute-path invocations bypass it; high maintenance (must faithfully proxy every subcommand). The PreToolUse hook covers the agent threat model better. |
| Shell alias/function wrapping `gh` | Human-habit nudge only; `command gh` bypasses; agents' Bash tool doesn't load rc. May add later as polish, not as a control. |
| GitHub merge queue | Unavailable: user-owned repos never get it (org-public or Enterprise-private only). |
| Kodiak / Mergify / bors-ng | Kodiak automates merging but doesn't *forbid* manual merges (and needs repo access for a third party); Mergify's protections still require binding branch protection (absent on free private); bors-ng is deprecated. |
| GitHub Action that auto-reverts/auto-closes improper merges | Revert automation on main is itself a dangerous irreversible actor (and Actions on private free repos burn limited minutes; Action can't "close" a merged PR). Downgraded to detect + alert + file-a-bead (solution 5). |
| Turning on `enforce_admins` on classic BP | Rulesets supersede it: same effect with layering, better audit, per-repo granularity; and the lockout-recovery concern is handled by ruleset editability (slow path) rather than a merge-time flag (fast path). |
| Turn-budget / honor-system instructions alone | The 48h audit is the proof this fails. Instructions remain (principle 2 teaching messages) but are not the control. |
