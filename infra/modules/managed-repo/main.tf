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

  # The ruleset's allowed merge methods are the canonical policy. Keep the
  # repository capability flags in lockstep so the provider cannot advertise a
  # method that the protected default branch rejects (or vice versa).
  allow_squash_merge     = contains(var.ruleset.rules.pull_request.allowed_merge_methods, "squash")
  allow_rebase_merge     = contains(var.ruleset.rules.pull_request.allowed_merge_methods, "rebase")
  allow_merge_commit     = contains(var.ruleset.rules.pull_request.allowed_merge_methods, "merge")
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
  name        = var.ruleset.name
  target      = var.ruleset.target
  enforcement = var.ruleset.enforcement

  conditions {
    ref_name {
      include = var.ruleset.conditions.ref_name.include
      exclude = var.ruleset.conditions.ref_name.exclude
    }
  }

  dynamic "bypass_actors" {
    for_each = var.ruleset.bypass_actors
    content {
      actor_id    = bypass_actors.value.actor_id
      actor_type  = bypass_actors.value.actor_type
      bypass_mode = bypass_actors.value.bypass_mode
    }
  }

  rules {
    deletion                = var.ruleset.rules.deletion
    non_fast_forward        = var.ruleset.rules.non_fast_forward
    required_linear_history = var.ruleset.rules.required_linear_history

    pull_request {
      allowed_merge_methods             = var.ruleset.rules.pull_request.allowed_merge_methods
      required_approving_review_count   = var.ruleset.rules.pull_request.required_approving_review_count
      dismiss_stale_reviews_on_push     = var.ruleset.rules.pull_request.dismiss_stale_reviews_on_push
      require_code_owner_review         = var.ruleset.rules.pull_request.require_code_owner_review
      require_last_push_approval        = var.ruleset.rules.pull_request.require_last_push_approval
      required_review_thread_resolution = var.ruleset.rules.pull_request.required_review_thread_resolution

      dynamic "required_reviewers" {
        for_each = var.ruleset.rules.pull_request.required_reviewers
        content {
          file_patterns     = required_reviewers.value.file_patterns
          minimum_approvals = required_reviewers.value.minimum_approvals

          reviewer {
            id   = required_reviewers.value.reviewer.id
            type = required_reviewers.value.reviewer.type
          }
        }
      }
    }

    # Only emit required_status_checks when the repo has at least one check
    # configured. An empty contexts list would make the rule a no-op and
    # generate unnecessary plan noise.
    dynamic "required_status_checks" {
      for_each = var.ruleset.rules.required_status_checks.enabled ? [1] : []
      content {
        # Whether a branch must be up to date with its base before merge is
        # part of the caller's declared ruleset rather than a separate module
        # knob, so the checked-in canonical JSON stays the single authority.
        # It matters because a green check is only evidence about the tip the
        # PR will actually land on (PR #1271); every caller here declares true.
        strict_required_status_checks_policy = var.ruleset.rules.required_status_checks.strict_required_status_checks_policy
        do_not_enforce_on_create             = var.ruleset.rules.required_status_checks.do_not_enforce_on_create

        dynamic "required_check" {
          for_each = var.ruleset.rules.required_status_checks.required_checks
          content {
            context        = required_check.value.context
            integration_id = try(required_check.value.integration_id, null)
          }
        }
      }
    }
  }

  lifecycle {
    precondition {
      condition     = trimspace(var.ruleset.name) != ""
      error_message = "ruleset.name must be non-empty."
    }

    precondition {
      condition     = var.ruleset.target == "branch"
      error_message = "managed-repo currently supports branch-target rulesets only."
    }

    precondition {
      condition     = var.ruleset.enforcement == "active"
      error_message = "ruleset.enforcement must be active for managed repositories."
    }

    precondition {
      condition     = length(var.ruleset.bypass_actors) == 0
      error_message = "managed repository rulesets must declare zero bypass actors."
    }

    # This resource *is* the default-branch protection boundary, so the target
    # is part of the contract, not a parameter. A length check alone accepts a
    # canonical JSON edited to include = ["refs/heads/release"], or one that
    # includes ~DEFAULT_BRANCH and then excludes it (or excludes the branch by
    # its literal ref), which applies cleanly while moving the zero-bypass
    # policy off the branch every merge actually lands on. Requiring the
    # marker and forbidding every exclusion is what makes the saved plan's
    # success mean the default branch is still protected.
    precondition {
      condition     = contains(var.ruleset.conditions.ref_name.include, "~DEFAULT_BRANCH")
      error_message = "ruleset.conditions.ref_name.include must contain ~DEFAULT_BRANCH; this module protects the default branch."
    }

    precondition {
      condition     = length(var.ruleset.conditions.ref_name.exclude) == 0
      error_message = "ruleset.conditions.ref_name.exclude must be empty; an exclusion can lift protection from the default branch."
    }

    precondition {
      condition = alltrue([
        length(var.ruleset.policy_validation.unsupported_rule_types) == 0,
        length(var.ruleset.policy_validation.unsupported_condition_keys) == 0,
        length(var.ruleset.policy_validation.unsupported_pull_request_parameter_keys) == 0,
        length(var.ruleset.policy_validation.unsupported_status_check_parameter_keys) == 0,
        length(var.ruleset.policy_validation.unsupported_policy_paths) == 0,
      ])
      error_message = "ruleset contains fields outside the supported zero-bypass branch-protection subset."
    }

    precondition {
      condition = alltrue([
        var.ruleset.rules.deletion,
        var.ruleset.rules.non_fast_forward,
        var.ruleset.rules.required_linear_history,
      ])
      error_message = "managed repository rulesets must retain deletion, non_fast_forward, and required_linear_history rules."
    }

    precondition {
      condition = alltrue([
        length(var.ruleset.rules.pull_request.allowed_merge_methods) == 1,
        toset(var.ruleset.rules.pull_request.allowed_merge_methods) == toset(["squash"]),
      ])
      error_message = "managed repository rulesets must allow squash merges only."
    }

    precondition {
      condition = !var.enforce_canonical_ruleset_invariants || alltrue([
        var.ruleset.rules.required_status_checks.enabled,
        var.ruleset.rules.required_status_checks.strict_required_status_checks_policy,
      ])
      error_message = "the canonical dear-agent ruleset must enable strict required status checks."
    }

    precondition {
      condition = !var.enforce_canonical_ruleset_invariants || alltrue([
        for check in var.ruleset.rules.required_status_checks.required_checks :
        try(check.integration_id, null) == 15368
      ])
      error_message = "every canonical dear-agent required check must use GitHub Actions integration_id 15368."
    }

    precondition {
      condition = alltrue([
        for check in var.ruleset.rules.required_status_checks.required_checks :
        trimspace(check.context) != "" &&
        (try(check.integration_id, null) == null ||
        (check.integration_id > 0 && floor(check.integration_id) == check.integration_id))
      ])
      error_message = "every required check must declare a non-empty context."
    }

    precondition {
      condition     = !var.ruleset.rules.required_status_checks.enabled || length(var.ruleset.rules.required_status_checks.required_checks) > 0
      error_message = "an enabled required_status_checks rule must declare at least one check."
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
