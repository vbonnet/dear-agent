###############################################################################
# dear-labs organization — baseline branch ruleset.
#
# Applied via the github.dearlabs provider alias (owner = "dear-labs").
# The same GITHUB_TOKEN must have org:admin scope (or repo:admin + ruleset
# write) on the dear-labs org.
#
# Enforcement is set to "evaluate" so the ruleset is active in audit mode:
# violations are logged but merges are not blocked. Flip to "active" once
# the org has repos and the team is comfortable with the rules.
#
# Required status checks are deliberately omitted at the org level: check
# context names are repo-specific. Add a github_repository_ruleset per repo
# (or extend this file with conditions.repository_name.include) once repos
# exist and their CI job names are known.
###############################################################################

resource "github_organization_ruleset" "baseline" {
  provider    = github.dearlabs
  name        = "baseline"
  target      = "branch"
  enforcement = "evaluate"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
    repository_name {
      include = ["~ALL"]
      exclude = []
    }
  }

  # bypass_actors is intentionally empty: no one (including org admins) can
  # bypass this ruleset. To allow admin bypass, add:
  #
  #   bypass_actors {
  #     actor_id    = 1
  #     actor_type  = "OrganizationAdmin"
  #     bypass_mode = "always"
  #   }

  rules {
    deletion                = true
    non_fast_forward        = true
    required_linear_history = true

    pull_request {
      required_approving_review_count   = 1
      dismiss_stale_reviews_on_push     = true
      require_code_owner_review         = false
      require_last_push_approval        = false
      required_review_thread_resolution = true
    }
  }
}
