###############################################################################
# Branch protection for vbonnet/* personal repositories.
#
# Uses github_branch_protection (the personal-account path). Rulesets require
# GitHub Pro or an org account; branch protection works on all tiers.
#
# Requires: GitHub Pro for private-repo enforcement ($4/mo plan).
#
# Required status checks are configured per repo in locals.tf (required_checks
# key). Repos with an empty list get protection without check enforcement —
# useful for repos with no CI yet.
###############################################################################

resource "github_branch_protection" "active" {
  for_each = local.active_repos

  repository_id = github_repository.active[each.key].node_id
  pattern       = each.value.default_branch

  required_linear_history         = true
  allows_force_pushes             = false
  allows_deletions                = false
  require_conversation_resolution = true

  # false = solo maintainer workflow where the owner is also the admin.
  # Flip to true to enforce rules for admins too (blocks self-merge without PR).
  enforce_admins = false

  required_pull_request_reviews {
    dismiss_stale_reviews           = true
    # 0 = PR is required but a second approver is not; raise for shared repos.
    required_approving_review_count = 0
  }

  # Only emit required_status_checks when the repo has at least one check
  # configured. An empty contexts list would make the rule a no-op and
  # generate unnecessary plan noise.
  dynamic "required_status_checks" {
    for_each = length(try(each.value.required_checks, [])) > 0 ? [1] : []
    content {
      # strict = false: branch does not need to be up-to-date before merge.
      # Flip to true to enforce "merge main before merging" — useful for repos
      # with long-lived feature branches and shared mutable state.
      strict   = false
      contexts = each.value.required_checks
    }
  }
}
