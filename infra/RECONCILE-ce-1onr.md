# Repo-settings drift reconciliation — ce-1onr (2026-06-19)

## Follow-up apply — `allow_rebase_merge` (2026-06-20)

After PR #555 (`fix/iac-repos-tf-no-rebase`, merged 2026-06-20) flipped the
desired state to `allow_rebase_merge = false` (squash-only, no rebase), this
branch was rebased onto `origin/main` to pick up that change and the remaining
drift was reconciled.

Targeted apply — **0 added, 12 changed, 0 destroyed**:

```
tofu apply -target='github_repository.active'
```

- `allow_rebase_merge` → `false` on the 12 repos that still had it enabled
  (`dear-agent` was already `false`). No other attribute changed.

Verified post-apply: all 13 active repos report `allow_rebase_merge=false` via
`gh api`; a follow-up targeted `tofu plan` on `github_repository.active` +
`github_repository_dependabot_security_updates.active` reports **No changes**.

State note: the R2 remote backend (`backend.tf`, ce-6f1b) is now in tree but R2
credentials / `backend.hcl` were not available in this sandbox, so this apply
reused the local tfstate from the 2026-06-19 run (refreshed from the live GitHub
API at plan time). GitHub remains the durable source of truth; the apply is
idempotent. A future run with R2 creds should `tofu init -backend-config=backend.hcl`
and confirm no drift.

---


Reconciled `github_repository` + Dependabot settings drift across all 13 active
`vbonnet/*` repos to the desired IaC state. Applied with OpenTofu against the
live GitHub account.

## What was applied

Targeted apply — **0 added, 26 changed, 0 destroyed**:

```
tofu apply -target='github_repository.active' \
           -target='github_repository_dependabot_security_updates.active'
```

- `github_repository.active` × 13 — in-place updates
- `github_repository_dependabot_security_updates.active` × 13 — enable security updates

### Setting changes (all 13 active repos)

| Setting | Change | Repos |
|---|---|---|
| `allow_merge_commit` | → `false` (linear history) | 13 |
| `vulnerability_alerts` | → `true` | 13 |
| `allow_auto_merge` | → `true` | 12 (dear-agent already on) |
| `delete_branch_on_merge` | → `true` | 12 (engram-research already on) |
| `has_projects` | → `false` | 12 (dear-agent already off) |
| `has_wiki` | → `false` | 1 (ai-sdlc) |
| `has_issues` | → `true` | 1 (vbonnet) |
| secret scanning + push protection | → `enabled` | 1 (vbonnet, public) |
| Dependabot security updates | → `enabled` | 13 |

Verified post-apply via `gh api`: settings match desired state; vulnerability
alerts return `204` (enabled); `vbonnet` shows secret scanning + push protection
enabled.

`has_downloads → null` and `ignore_vulnerability_alerts_during_read` appeared in
the diff — both benign provider-internal no-ops, not GitHub-visible changes.

## What was deliberately NOT applied (out of scope / blocked)

The full plan proposed `26 to add`. Everything in the "add" set was excluded:

1. **13 `github_repository_ruleset.branch_protection` (creates)** — all 13 repos
   already have an active `branch-protection` ruleset on GitHub. `import.sh` was
   **not importing rulesets**, so a blind apply would have created 13 DUPLICATE
   rulesets (GitHub permits duplicate names) — doubling enforcement, not
   reconciling it. Fixed `import.sh` (see below); after re-import these show as
   no-change.
2. **12 `github_branch_protection.active` (creates)** — a second protection layer
   alongside the existing rulesets. Not part of repo-settings drift; belongs to
   the branch-protection / auto-merge follow-on (ce-r81r). Deferred.
3. **1 `github_organization_ruleset.baseline` (dear-labs)** — requires a token
   with `admin:org`. The current `gh` token has only `read:org`, so this would
   403. Deferred until an `admin:org` token is available (as dear_labs.tf notes).

## Code fix included on this branch

`import.sh` now imports the existing `branch-protection` repository ruleset per
repo (looked up by numeric id), preventing the duplicate-ruleset hazard above.

## Caveats / follow-ups

- **State was local-only and ephemeral** (this ran in a sandbox worktree; no
  remote backend — ce-6f1b is still open). The GitHub side is the durable source
  of truth and the apply is idempotent, so the lost tfstate is immaterial: a
  future `import.sh` + `plan` against a real backend reconstructs state and shows
  no drift. **ce-6f1b (remote backend) should still land before further applies.**
- The `vulnerability_alerts` deprecation warning is expected (provider will move
  to a dedicated resource); no action needed now.
