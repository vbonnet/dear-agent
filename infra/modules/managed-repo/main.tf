###############################################################################
# managed-repo — standard fleet policy for a single GitHub repository.
#
# Encapsulates the three resources that together define how every actively
# managed repo is governed:
#   - github_repository                              security + merge hygiene
#   - github_repository_dependabot_security_updates  Dependabot auto-PRs
#   - github_repository_ruleset "branch-protection"  default-branch rules
#
# Provider-agnostic by design: the caller supplies the github provider (the
# default personal account, or an org alias such as github.dearlabs) via the
# module `providers` argument. The same policy therefore applies unchanged to
# vbonnet/* personal repos and to dear-labs org repos once they come online.
#
# Availability / personal-Pro constraints baked in here:
#   - Repository rulesets work on GitHub Pro ($4/mo) for private repos.
#   - enforcement = "active" is the only mode available; "evaluate" (audit-only)
#     is GitHub Enterprise-only and 422s on a personal account.
#   - No merge_queue rule: merge queues require an ORGANIZATION account. Add one
#     (gated behind a variable) if these repos ever move under an org.
###############################################################################

resource "github_repository" "this" {
  name       = var.name
  visibility = var.visibility

  # Merge hygiene: squash-only; no rebase or merge commits (linear history);
  # auto-merge and branch-delete-after-merge enabled.
  allow_squash_merge     = true
  allow_rebase_merge     = false
  allow_merge_commit     = false
  allow_auto_merge       = true
  delete_branch_on_merge = true

  has_wiki     = false
  has_projects = false
  has_issues   = true

  # Deprecated attribute but the only supported path to vulnerability alerts;
  # keep until the provider ships a dedicated resource.
  vulnerability_alerts = true

  archived = false

  # Secret scanning and push protection are free for public repos.
  # Private repos require GitHub Advanced Security — do NOT enable there.
  dynamic "security_and_analysis" {
    for_each = var.visibility == "public" ? [1] : []
    content {
      secret_scanning {
        status = "enabled"
      }
      secret_scanning_push_protection {
        status = "enabled"
      }
    }
  }

  # Project metadata (description, topics, homepage) is owned by each repo,
  # not by this IaC. Ignore it so the plan stays focused on security/merge
  # settings and never wipes repo metadata.
  lifecycle {
    ignore_changes = [
      description,
      homepage_url,
      topics,
      pages,
    ]
  }
}

resource "github_repository_dependabot_security_updates" "this" {
  repository = github_repository.this.name
  enabled    = true
}

# -----------------------------------------------------------------------------
# Branch-protection ruleset (the sole branch-protection mechanism; the legacy
# github_branch_protection resources it replaced were removed in ce-yg6b).
# -----------------------------------------------------------------------------
resource "github_repository_ruleset" "branch_protection" {
  repository  = github_repository.this.name
  name        = "branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
    deletion                = true
    non_fast_forward        = true
    required_linear_history = true

    pull_request {
      allowed_merge_methods             = ["squash"]
      required_approving_review_count   = 0
      dismiss_stale_reviews_on_push     = true
      require_code_owner_review         = false
      require_last_push_approval        = false
      required_review_thread_resolution = true
    }

    # Only emit required_status_checks when the repo has at least one check
    # configured. An empty contexts list would make the rule a no-op and
    # generate unnecessary plan noise.
    dynamic "required_status_checks" {
      for_each = length(var.required_checks) > 0 ? [1] : []
      content {
        # Require the branch to be up to date with the base before merge, so a
        # green check is evidence about the tip the PR will actually land on.
        strict_required_status_checks_policy = var.strict_required_status_checks

        dynamic "required_check" {
          for_each = var.required_checks
          content {
            context = required_check.value
          }
        }
      }
    }
  }
}

# -----------------------------------------------------------------------------
# Claude Code PR review (opt-in via var.enable_claude_review) — advisory only,
# deliberately absent from required_status_checks above. Pushes the same
# workflow content dear-agent hand-maintains at
# .github/workflows/claude-code-review.yml (see ../../claude_review.tf, which
# reads that file as the single source of truth) plus the OAuth secret it
# needs. Not applied to dear-agent itself — it owns that file directly in git.
#
# The workflow never writes directly to the protected default branch. OpenTofu
# updates a dedicated rollout branch and opens (or updates) a normal PR. A
# maintainer must review and merge that PR under the repository's ruleset.
# -----------------------------------------------------------------------------
resource "github_branch" "claude_code_review_rollout" {
  count = var.enable_claude_review && var.claude_review_rollout ? 1 : 0

  repository    = github_repository.this.name
  branch        = var.claude_review_rollout_branch
  source_branch = var.default_branch
}

resource "github_repository_file" "claude_code_review_workflow" {
  count = var.enable_claude_review && var.claude_review_rollout ? 1 : 0

  repository          = github_repository.this.name
  branch              = github_branch.claude_code_review_rollout[0].branch
  file                = ".github/workflows/claude-code-review.yml"
  content             = var.claude_review_workflow_content
  commit_message      = "chore(claude-review): sync claude-code-review.yml"
  commit_author       = "OpenTofu"
  commit_email        = "opentofu@users.noreply.github.com"
  overwrite_on_create = true

  lifecycle {
    precondition {
      condition     = try(trimspace(var.claude_review_workflow_content) != "", false)
      error_message = "claude_review_workflow_content must be non-empty when enable_claude_review is true."
    }
  }
}

resource "github_repository_pull_request" "claude_code_review_rollout" {
  count = var.enable_claude_review && var.claude_review_rollout ? 1 : 0

  base_repository       = github_repository.this.name
  base_ref              = var.default_branch
  head_ref              = github_branch.claude_code_review_rollout[0].branch
  title                 = "chore(claude-review): roll out Claude Code review workflow"
  body                  = "Managed by OpenTofu. This rollout PR intentionally requires the repository's normal review and merge policy; do not bypass branch protection."
  maintainer_can_modify = true

  depends_on = [github_repository_file.claude_code_review_workflow]
}

resource "github_actions_secret" "claude_code_oauth_token" {
  count = var.enable_claude_review ? 1 : 0

  repository      = github_repository.this.name
  secret_name     = "CLAUDE_CODE_OAUTH_TOKEN"
  plaintext_value = var.claude_code_oauth_token

  lifecycle {
    precondition {
      condition     = try(trimspace(var.claude_code_oauth_token) != "", false)
      error_message = "claude_code_oauth_token must be non-empty when enable_claude_review is true."
    }
  }
}
