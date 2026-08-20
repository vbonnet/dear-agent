###############################################################################
# vbonnet/* personal repositories — standard fleet policy.
#
# Each active repo is governed by the ./modules/managed-repo module, which
# encapsulates the github_repository (security + merge hygiene), Dependabot
# security updates, and the branch-protection ruleset. This for_each over
# var.active_repos is the single instantiation point; changing fleet-wide
# policy means editing the module once, and the inventory is supplied through
# the private repos.auto.tfvars input.
#
# The module is provider-agnostic. It is instantiated here with the default
# (personal) github provider. When dear-labs org repos come online, add a
# second module block keyed on a dear-labs inventory with
# `providers = { github = github.dearlabs }` — see dear_labs.tf and the
# module README for the pattern.
###############################################################################

module "managed_repos" {
  source   = "./modules/managed-repo"
  for_each = var.active_repos

  name                                 = each.key
  visibility                           = each.value.visibility
  default_branch                       = try(each.value.default_branch, "main")
  enforce_canonical_ruleset_invariants = each.key == "dear-agent"
  # dear-agent's checked-in ruleset is the canonical desired policy. Other
  # repositories retain their private inventory-defined policy until they have
  # their own committed canonical declaration.
  ruleset = each.key == "dear-agent" ? local.dear_agent_ruleset : {
    name          = "branch-protection"
    target        = "branch"
    enforcement   = "active"
    bypass_actors = []
    conditions = {
      ref_name = {
        include = ["~DEFAULT_BRANCH"]
        exclude = []
      }
    }
    policy_validation = {
      unsupported_rule_types                  = []
      unsupported_condition_keys              = []
      unsupported_pull_request_parameter_keys = []
      unsupported_status_check_parameter_keys = []
      unsupported_policy_paths                = []
    }
    rules = {
      deletion                = true
      non_fast_forward        = true
      required_linear_history = true
      pull_request = {
        allowed_merge_methods             = ["squash"]
        required_approving_review_count   = 0
        dismiss_stale_reviews_on_push     = true
        require_code_owner_review         = false
        require_last_push_approval        = false
        required_review_thread_resolution = true
        required_reviewers                = []
      }
      required_status_checks = {
        enabled = length(coalesce(
          try(each.value.required_check_identities, null),
          [for context in try(each.value.required_checks, []) : {
            context        = context
            integration_id = null
          }],
        )) > 0
        # A green check is only evidence about the tip the PR will land on, so
        # fleet repositories inherit the same up-to-date requirement dear-agent
        # declares canonically (PR #1271). The cost is serialization: each merge
        # invalidates every other open PR's up-to-date status.
        strict_required_status_checks_policy = true
        do_not_enforce_on_create             = false
        required_checks = coalesce(
          try(each.value.required_check_identities, null),
          [for context in try(each.value.required_checks, []) : {
            context        = context
            integration_id = null
          }],
        )
      }
    }
  }
  enable_claude_review           = try(each.value.enable_claude_review, false)
  claude_review_rollout          = try(each.value.claude_review_rollout, false)
  claude_review_workflow_content = local.claude_review_workflow_content
  claude_code_oauth_token        = var.claude_code_oauth_token

  providers = {
    github = github
  }
}

# -----------------------------------------------------------------------------
# Merge-queue ruleset: intentionally NOT defined.
#
# GitHub merge queues require an ORGANIZATION account; they are unavailable on
# a personal account (even GitHub Pro). The previous merge_queue ruleset (and
# its local.merge_queue_repos input) was removed for this reason. If these
# repos ever move under an org, add a merge_queue rule to the module (gated
# behind a variable) and a merge_queue_repos inventory.
# -----------------------------------------------------------------------------
