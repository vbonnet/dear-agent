# module: managed-repo

Standard fleet policy for a single managed GitHub repository. Encapsulates the
three resources that together govern an actively managed repo:

| Resource | Purpose |
|---|---|
| `github_repository.this` | Security defaults + merge hygiene (squash-only, linear history, auto-merge, Dependabot vulnerability alerts, secret scanning on public repos). |
| `github_repository_dependabot_security_updates.this` | Dependabot security-update PRs. |
| `github_repository_ruleset.branch_protection` | Active branch-protection ruleset on the default branch (no deletion, no force-push, linear history, PR required, optional status-check gate). |

This is the single place to apply fleet-wide repo policy. The repo **inventory**
lives in private inputs; this module owns the provider projection applied to
each entry. For dear-agent, the committed `../../.github/rulesets/main.json`
is the policy authority and OpenTofu is only its deployment path.

## Provider-agnostic

The module declares `integrations/github` as a required provider but contains no
`provider` block, so the caller decides which account/org it acts on by passing
a provider explicitly:

```hcl
# Personal account (vbonnet/*)
module "managed_repos" {
  source   = "./modules/managed-repo"
  for_each = var.active_repos

  name            = each.key
  visibility      = each.value.visibility
  ruleset = {
    name          = "branch-protection"
    target        = "branch"
    enforcement   = "active"
    bypass_actors = []
    conditions = { ref_name = { include = ["~DEFAULT_BRANCH"], exclude = [] } }
    policy_validation = {
      unsupported_rule_types                  = []
      unsupported_condition_keys              = []
      unsupported_pull_request_parameter_keys = []
      unsupported_status_check_parameter_keys = []
    }
    rules = {
      deletion = true
      non_fast_forward = true
      required_linear_history = true
      pull_request = {
        allowed_merge_methods = ["squash"]
        required_approving_review_count = 0
        dismiss_stale_reviews_on_push = true
        require_code_owner_review = false
        require_last_push_approval = false
        required_review_thread_resolution = true
        required_reviewers = []
      }
      required_status_checks = {
        enabled = length(each.value.required_checks) > 0
        strict_required_status_checks_policy = true
        do_not_enforce_on_create = false
        required_checks = [for context in each.value.required_checks : {
          context = context
          integration_id = null
        }]
      }
    }
  }

  providers = {
    github = github
  }
}

```

An organization caller supplies the same supported `ruleset` subset through its
own inventory and passes `github.dearlabs`; it must not use the retired
`required_checks` module input.

## Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `name` | `string` | — | Repository name (slug, without owner). |
| `visibility` | `string` | — | `"public"` or `"private"`. Secret scanning + push protection are enabled for public repos only (private requires GitHub Advanced Security). |
| `ruleset` | object | — | Ruleset identity and enforcement. Each required check has `context` plus optional `integration_id`; each path-scoped required reviewer preserves `file_patterns`, `minimum_approvals`, and nested reviewer `id`/`type`. The module rejects non-active or non-zero-bypass input. Empty checks = PR-required protection with no status-check gate. |
| `enforce_canonical_ruleset_invariants` | `bool` | `false` | Enables dear-agent's strict required-check and GitHub Actions integration-ID invariants without changing legacy inventory-owned fleet policy. |
| `default_branch` | `string` | `"main"` | Target branch for Claude-review rollout PRs. |
| `enable_claude_review` | `bool` | `false` | Installs the `CLAUDE_CODE_OAUTH_TOKEN` secret. Advisory only — never added to `required_checks`. See `../../claude_review.tf` for the fleet rollout list. |
| `claude_review_rollout` | `bool` | `false` | Transiently stages `.github/workflows/claude-code-review.yml` and opens its rollout PR. Set false after merge so the deleted rollout branch is not recreated. |
| `claude_review_workflow_content` | `string` | `null` | Workflow file content. Required when `claude_review_rollout = true`; the caller (`../../claude_review.tf`) sources it from dear-agent's own hand-maintained copy. |
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
