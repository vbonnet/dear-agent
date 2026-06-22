###############################################################################
# Repository rulesets for vbonnet/* personal repositories.
#
# Rulesets are the successor to branch protection rules and are the SINGLE
# active branch-protection mechanism for vbonnet/* repos. The legacy
# github_branch_protection resources (branch_protection.tf) were validated as
# fully covered by these rulesets and removed in ce-yg6b — see that bead and
# the PR for the field-by-field coverage proof.
#
# Availability / personal-Pro constraints:
#   - Repository-level rulesets work on GitHub Pro ($4/mo) for private repos.
#   - enforcement = "active" is the only mode available here. "evaluate"
#     (audit-only) mode is GitHub Enterprise-only — it 422s on a personal
#     account, so this file uses "active" throughout.
#   - Merge-queue rulesets require an ORGANIZATION account and are NOT
#     available on a personal account, so no merge_queue ruleset is defined
#     here. (Merge queues were tried and removed for this reason.)
#
# Provider: integrations/github ~> 6.0.
###############################################################################

# -----------------------------------------------------------------------------
# Branch protection ruleset — applied to ALL active repos.
#
# In "active" enforcement (the only mode available on a personal Pro account).
# This is the sole branch-protection mechanism; the legacy
# github_branch_protection resources it replaced were removed in ce-yg6b.
# -----------------------------------------------------------------------------

resource "github_repository_ruleset" "branch_protection" {
  for_each = local.active_repos

  repository  = each.key
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
# Merge-queue ruleset: intentionally NOT defined.
#
# GitHub merge queues require an ORGANIZATION account; they are unavailable on
# a personal account (even GitHub Pro). The previous merge_queue ruleset (and
# its local.merge_queue_repos input) was removed for this reason. If these
# repos ever move under an org, reintroduce a github_repository_ruleset with a
# merge_queue rule and a merge_queue_repos local.
# -----------------------------------------------------------------------------
