# Branch Protection for `main` (zero-bypass ruleset)

`main` is protected by a **repository ruleset**, defined as code in
[`.github/rulesets/main.json`](../.github/rulesets/main.json) and applied via
`gh api`. The ruleset is **zero-bypass** (`bypass_actors: []`) — it binds
*everyone*, including the repo owner. This is the source of truth; the GitHub UI
mirrors it but should not be edited by hand.

Per `docs/design-safe-merge.md` §4.1 / §5 (P1), this replaces the legacy
**classic** branch-protection rules, which allowed admin bypass
(`enforce_admins: false`).

## What the ruleset enforces

| Rule | Value |
|------|-------|
| Target | default branch (`~DEFAULT_BRANCH`) |
| Enforcement | `active` |
| Bypass actors | none (`[]`) — binds the owner too |
| Require a pull request before merging | yes |
| Require conversation resolution | yes (`required_review_thread_resolution`) |
| Dismiss stale reviews on push | yes |
| Allowed merge methods | **squash only** |
| Require linear history | yes (`required_linear_history` + `non_fast_forward`) |
| Block branch deletion | yes (`deletion`) |
| Require status checks | yes, strict (branch must be up to date) |
| Required checks | the contexts below |

### Required status checks

Each is pinned to `integration_id: 15368` (the GitHub Actions app) so a
same-named check from a *different* app can never satisfy the gate — the failure
mode behind the [phantom Trivy check](https://github.com/vbonnet/engram-research/blob/main/retrospectives/2026-06-08-phantom-trivy-required-check.md).

| Context | Produced by |
|---------|-------------|
| `Build & Test (ubuntu-latest)` | `ci.yml` |
| `Build & Test (macos-latest)` | `ci.yml` |
| `Analyze Go Code (go)` | `codeql.yml` |
| `govulncheck` | `ci.yml` |
| `Bash Script Size Check (20-line limit)` | `language-policy.yml` |
| `Vulnerability Scan` | `sbom-scan.yml` |

`5-Dimension AI Review` (`review.yml`) is **not** currently in this list. It was
added by #991, then paused 2026-07-27 — `ANTHROPIC_API_KEY` was never funded
(no quota outside the Max plan, which the workflow can't bill against), so the
gate ran fail-closed on every PR. It was removed here rather than left in
`main.json` unapplied, so a routine `gh api ... --method PUT` sync of this file
can't silently start blocking merges on a check nobody is funding.

To re-enable: set the `ANTHROPIC_API_KEY` repo secret, and re-add the context
to `main.json` + re-apply (below) if you also want it required again. Nothing
else changes — the workflow gates itself on a `Detect review key` preflight
step that reads the secret via `env:` and writes `present=true|false` to
`$GITHUB_OUTPUT`, which the review step then tests as
`steps.key.outputs.present == 'true'`. That indirection is load-bearing:
GitHub does **not** expose the `secrets` context inside a step `if:`, so
`if: secrets.ANTHROPIC_API_KEY != ''` does not work. See the PAUSED comment at
the top of `review.yml`.

**Before adding/removing a required check, confirm a job emits a check run
with that exact context name** (matrix suffixes included) on PRs to `main` —
otherwise the gate becomes unsatisfiable.

## Applying the ruleset

> [!WARNING]
> Applying this binds the repo owner immediately and is intended to go in
> through a reviewed PR, not an ad-hoc push. Run these only after the change is
> approved.

**Create** (first time):

```sh
gh api repos/vbonnet/dear-agent/rulesets \
  --method POST --input .github/rulesets/main.json
```

**Update** the existing ruleset in place (find its id with
`gh api repos/vbonnet/dear-agent/rulesets`):

```sh
gh api repos/vbonnet/dear-agent/rulesets/<RULESET_ID> \
  --method PUT --input .github/rulesets/main.json
```

**Verify**:

```sh
gh api repos/vbonnet/dear-agent/rulesets/<RULESET_ID> \
  | jq '{enforcement, bypass_actors, rules: [.rules[].type]}'
```

## Retiring classic branch protection

Once the ruleset is active and verified, remove the legacy classic protection
so there is a single source of truth:

```sh
gh api -X DELETE repos/vbonnet/dear-agent/branches/main/protection
```

> [!IMPORTANT]
> The daily **Branch Protection Audit** (`.github/workflows/branch-protection-audit.yml`)
> historically read only the classic-protection endpoint. It has been updated to
> also accept a qualifying active ruleset, so retiring classic protection will
> **not** trigger false-positive `branch-protection` issues. Retire classic
> protection only on a version of `main` that already contains that audit update.
