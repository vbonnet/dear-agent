# module: managed-repo

Standard fleet policy for a single managed GitHub repository. Encapsulates the
three resources that together govern an actively managed repo:

| Resource | Purpose |
|---|---|
| `github_repository.this` | Security defaults + merge hygiene (squash-only, linear history, auto-merge, Dependabot vulnerability alerts, secret scanning on public repos). |
| `github_repository_dependabot_security_updates.this` | Dependabot security-update PRs. |
| `github_repository_ruleset.branch_protection` | Active branch-protection ruleset on the default branch (no deletion, no force-push, linear history, PR required, optional status-check gate). |

This is the single place to change fleet-wide repo policy. The repo **inventory**
lives in `../../locals.tf` (`local.active_repos`); this module owns the
**policy** applied to each entry.

## Provider-agnostic

The module declares `integrations/github` as a required provider but contains no
`provider` block, so the caller decides which account/org it acts on by passing
a provider explicitly:

```hcl
# Personal account (vbonnet/*)
module "managed_repos" {
  source   = "./modules/managed-repo"
  for_each = local.active_repos

  name            = each.key
  visibility      = each.value.visibility
  required_checks = try(each.value.required_checks, [])

  providers = {
    github = github
  }
}

# dear-labs org repos (once they come online), reusing the same policy:
module "dearlabs_repos" {
  source   = "./modules/managed-repo"
  for_each = local.dearlabs_repos

  name            = each.key
  visibility      = each.value.visibility
  required_checks = try(each.value.required_checks, [])

  providers = {
    github = github.dearlabs
  }
}
```

## Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `name` | `string` | — | Repository name (slug, without owner). |
| `visibility` | `string` | — | `"public"` or `"private"`. Secret scanning + push protection are enabled for public repos only (private requires GitHub Advanced Security). |
| `required_checks` | `list(string)` | `[]` | Exact check-run names required before merge. Empty = PR-required protection with no status-check gate. |
| `default_branch` | `string` | `"main"` | Target branch for Claude-review rollout PRs. |
| `enable_claude_review` | `bool` | `false` | Stages `.github/workflows/claude-code-review.yml` on a rollout branch, opens a normal PR to `default_branch`, and installs the `CLAUDE_CODE_OAUTH_TOKEN` secret. Advisory only — never added to `required_checks`. See `../../claude_review.tf` for the fleet rollout list. |
| `claude_review_workflow_content` | `string` | `null` | Workflow file content. Required when `enable_claude_review = true`; the caller (`../../claude_review.tf`) sources it from dear-agent's own hand-maintained copy. |
| `claude_review_rollout_branch` | `string` | `"automation/claude-code-review"` | Unprotected branch used to stage the workflow before its rollout PR. |
| `claude_code_oauth_token` | `string` (sensitive) | `null` | Claude Code OAuth token (from `claude setup-token`). Required when `enable_claude_review = true`. |

## Outputs

| Name | Description |
|---|---|
| `repository_name` | The repository's name. |
| `repository_node_id` | GraphQL node ID (for resources that key on `node_id`). |
| `repository_full_name` | `owner/name` slug. |
| `ruleset_id` | ID of the branch-protection ruleset. |
| `claude_review_enabled` | Whether the Claude Code review workflow + secret are installed on this repo. |

## Constraints

- **Personal Pro account.** Rulesets run in `active` enforcement only;
  `evaluate` (audit-only) mode is Enterprise-only.
- **No merge queue.** Merge queues require an organization account. Add a
  `merge_queue` rule (gated behind a variable) if these repos move under an org.
- **Archived repos are not managed here.** GitHub rejects mutations on archived
  repos; they are declared separately with `ignore_changes = all` (see
  `../../repos.tf`).
