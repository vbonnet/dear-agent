###############################################################################
# vbonnet/* personal repositories — standard fleet policy.
#
# Each active repo is governed by the ./modules/managed-repo module, which
# encapsulates the github_repository (security + merge hygiene), Dependabot
# security updates, and the branch-protection ruleset. This for_each over
# var.active_repos is the single instantiation point; changing fleet-wide
# policy means editing the module once, and the inventory lives in locals.tf.
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

  name                           = each.key
  visibility                     = each.value.visibility
  default_branch                 = try(each.value.default_branch, "main")
  required_checks                = try(each.value.required_checks, [])
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
