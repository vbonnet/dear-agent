# Branch Protection for `main` (zero-bypass ruleset)

`main` is protected by a **repository ruleset**, defined as code in
[`.github/rulesets/main.json`](../.github/rulesets/main.json) and applied via
`gh api`. The ruleset is **zero-bypass** (`bypass_actors: []`) — it binds
*everyone*, including the repo owner. This is the source of truth; the GitHub UI
mirrors it but should not be edited by hand. The ruleset is the **only**
protection mechanism on `main` — no classic branch-protection rules are
configured.

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
| `Identity, index, and lifecycle parity` | `adr-integrity.yml` |
| `Header block format` | `doc-header-lint.yml` |

`5-Dimension AI Review` (`review.yml`) is deliberately **not** in this list, so
a check nobody is funding cannot block merges. The workflow gates itself on a
`Detect review key` preflight step that reads the `ANTHROPIC_API_KEY` secret
via `env:` and writes `present=true|false` to `$GITHUB_OUTPUT`, which later
steps test as `steps.key.outputs.present == 'true'`. That indirection is
load-bearing: GitHub does **not** expose the `secrets` context inside a step
`if:`, so `if: secrets.ANTHROPIC_API_KEY != ''` does not work. The fail-closed
contract, including the neutral paths taken while no reviewer credential is
configured, is documented in the comment at the top of `review.yml`. To make
the check required: set the `ANTHROPIC_API_KEY` repo secret, then add the
context to `main.json` and re-apply (below).

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

## Branch Protection Audit

The daily **Branch Protection Audit**
(`.github/workflows/branch-protection-audit.yml`) verifies each fleet repo is
protected by classic protection **or** a qualifying active ruleset
(default-branch target, no bypass actors, required status checks + pull
request), so ruleset-only repos like this one do not raise false positives.
