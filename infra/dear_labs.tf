###############################################################################
# dear-labs organization — baseline branch ruleset.
#
# Applied via the github.dearlabs provider alias (owner = "dear-labs").
# The same GITHUB_TOKEN must have org:admin scope (or repo:admin + ruleset
# write) on the dear-labs org.
#
# Enforcement is "active". "evaluate" (audit-only) mode is GitHub
# Enterprise-only and 422s on non-Enterprise accounts, so we enforce directly.
# The org has no repos yet, so an active ruleset blocks nothing until repos
# are added — at which point it immediately protects their default branch.
#
# NOTE: applying THIS resource requires a token with admin:org (ruleset write)
# on dear-labs. The default `gh auth token` here carries only read:org, so a
# full apply will 403 on this resource. Treat admin:org as a production-plan
# prerequisite; do not use a targeted apply to evade an incomplete full-root
# review.
#
# Required status checks are deliberately omitted at the org level: check
# context names are repo-specific. Add a github_repository_ruleset per repo
# (or extend this file with conditions.repository_name.include) once repos
# exist and their CI job names are known.
###############################################################################

# MIGRATION GATE: NO github_repository resources may be added to this file
# until the dear-labs → vbonnet org migration decision is finalized.
# Trigger: first external collaborator need OR first org-level feature need.
# The decision record is retained in the private research archive.
#
# All new repos should be created under github.com/vbonnet until migration
# is decided. dear-labs is in standby mode.

resource "github_organization_ruleset" "baseline" {
  provider    = github.dearlabs
  name        = "baseline"
  target      = "branch"
  enforcement = "active"

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
