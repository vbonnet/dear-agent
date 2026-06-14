###############################################################################
# Repository rulesets for vbonnet/* personal repositories.
#
# Rulesets are the successor to branch protection rules. They support merge
# queues, layered enforcement, and audit-mode ("evaluate"). This file adds
# rulesets alongside the existing github_branch_protection resources; once
# validated in evaluate mode, flip enforcement to "active" and remove
# branch_protection.tf.
#
# Availability: repository-level rulesets require GitHub Pro for private repos.
# Merge queue requires rulesets (not available via branch protection rules).
#
# Provider: integrations/github ~> 6.0 (merge_queue support added in v6.8).
###############################################################################

# -----------------------------------------------------------------------------
# Branch protection ruleset — applied to ALL active repos.
#
# Mirrors the existing github_branch_protection config but in ruleset form.
# Starts in "evaluate" mode so violations are logged but merges are not
# blocked. Once validated, flip enforcement to "active" and remove the
# legacy github_branch_protection resources in branch_protection.tf.
# -----------------------------------------------------------------------------

resource "github_repository_ruleset" "branch_protection" {
  for_each = local.active_repos

  repository  = each.key
  name        = "branch-protection"
  target      = "branch"
  enforcement = "evaluate"

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
      required_approving_review_count   = 0
      dismiss_stale_reviews_on_push     = true
      require_code_owner_review         = false
      require_last_push_approval        = false
      required_review_thread_resolution = true
    }

    dynamic "required_status_checks" {
      for_each = length(try(each.value.required_checks, [])) > 0 ? [1] : []
      content {
        strict_required_status_checks_policy = false

        dynamic "required_check" {
          for_each = each.value.required_checks
          content {
            context = required_check.value
          }
        }
      }
    }
  }
}

# -----------------------------------------------------------------------------
# Merge queue ruleset — initially dear-agent only.
#
# Enables the GitHub merge queue with squash-and-merge. PRs enter a queue,
# are tested together in groups, and merge only if all checks pass.
#
# Expand to other repos by adding entries to local.merge_queue_repos.
# -----------------------------------------------------------------------------

resource "github_repository_ruleset" "merge_queue" {
  for_each = local.merge_queue_repos

  repository  = each.key
  name        = "merge-queue"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
    merge_queue {
      merge_method                     = "SQUASH"
      min_entries_to_merge             = each.value.min_group_size
      max_entries_to_merge             = each.value.max_group_size
      max_entries_to_build             = each.value.max_group_size
      grouping_strategy                = "ALLGREEN"
      check_response_timeout_minutes   = each.value.check_timeout_minutes
      min_entries_to_merge_wait_minutes = 0
    }
  }
}
