# S4 Research — GitHub Rulesets & Server-Side Enforcement on Free Plans

> Produced by a research subagent, 2026-06-11. Sources cited inline.

## 1. Rulesets vs classic branch protection — plan matrix

Canonical doc reusables ([repo-rules](https://raw.githubusercontent.com/github/docs/main/data/reusables/gated-features/repo-rules.md), [protected-branches](https://raw.githubusercontent.com/github/docs/main/data/reusables/gated-features/protected-branches.md)):

- **Free-plan PUBLIC repos:** branch/tag rulesets fully available and **enforced** — required status checks, require-PR, required conversation resolution, linear history, block force pushes, required merge method, signed commits.
- **Free-plan PRIVATE repos:** rulesets can be *created and viewed* but are **NOT enforced** — same paid gate as classic branch protection (Pro for user-owned, Team for org-owned). UI banner confirms: "Your rulesets won't be enforced on this private repository until you upgrade" ([#184363](https://github.com/orgs/community/discussions/184363)).
- Structural advantages over classic BP: multiple rulesets layer; read-access users can view rules; Evaluate mode exists but is Enterprise-only.
- **Cheapest paid path:** GitHub **Pro ($4/mo individual)** unlocks enforcement on **user-owned private** repos.

## 2. Bypass behavior — the headline finding

- **Rulesets support ZERO bypass actors.** `bypass_actors` is optional/empty-able ([REST: repository rules](https://docs.github.com/en/rest/repos/rules?apiVersion=2022-11-28)); repo admins get **no implicit exemption**. With an empty bypass list, admins/owners are blocked from merging past required checks and unresolved conversations like everyone else (confirmed behavior: [#113172](https://github.com/orgs/community/discussions/113172)). `gh pr merge --admin` **fails** against such a ruleset.
  - *Flagged caveat:* no single doc sentence literally says "empty list ⇒ admins cannot bypass"; established by the opt-in API design + observed behavior.
- **Classic BP is the inverse:** admins exempt by default; `enforce_admins` opt-in. dear-agent today has `enforce_admins: false` (deliberate lockout-recovery choice), which is exactly why `--admin` works.
- Bypass modes when actors ARE listed: `always`, `pull_request`, and `exempt` (Sept 2025 — **silently** skips enforcement, no audit prominence; avoid).
- **Honest limit for a solo owner:** the admin can still go to Settings and disable/delete the ruleset. What zero-bypass buys: merge-time one-flag bypass fails; circumvention requires a deliberate, *auditable settings change* (ruleset edits appear in the repo audit log / ruleset history). True rule-tamper-proofing needs org-level rulesets (Team+) with a non-owner repo admin — moot solo.

## 3. Required conversation resolution

Yes — sub-setting of "Require a pull request before merging"; REST `required_review_thread_resolution` ([Available rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)). Free public; Pro/Team private.

## 4. Merge queue

Org-owned public repos (any plan) or Enterprise Cloud private repos only. **User-owned repos: never** ([gated-features/merge-queue](https://raw.githubusercontent.com/github/docs/main/data/reusables/gated-features/merge-queue.md)). Not available in org-level rulesets.

## 5. Push rulesets

Team plan only, private/internal repos only. Not relevant on Free.

## 6. Org-level rulesets

Team+ only (extended from Enterprise on 2025-06-16). Free orgs: repo-level rulesets, enforced on public only.

## 7. 2025–2026 changes

- 2025-03-24: PR **merge method rule** GA (can force squash-only).
- 2025-04-09: push-rule delegated bypass GA (Team+).
- 2025-09-10: `exempt` bypass type (silent — avoid).
- 2026-02-17: required reviewer rule GA (file-pattern-scoped teams).
- 2026-05-07: **individual users as bypass actors** on repo-level rulesets.
- **No change relaxes the free-private enforcement gate** ([#174400](https://github.com/orgs/community/discussions/174400) still open).

## Bottom line for vbonnet's repos

| Repo class | Today (Free) | Action |
|---|---|---|
| Public, user-owned (dear-agent, ...) | Rulesets fully enforced | Branch ruleset on `main`: required checks + require PR + conversation resolution + **empty bypass list** → `--admin` dies. $0. |
| Private, user-owned (dotfiles, brain-v2, engram) | Rules viewable, NOT enforced | GitHub Pro ($4/mo) enables enforcement; until then client-side only |
| Merge queue | Unavailable (user-owned) | Client-side wrapper must do the re-validate-against-latest-base job |
