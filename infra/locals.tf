# The checked-in dear-agent ruleset is the desired declaration for its GitHub
# provider resource. Keep the provider projection below narrow: this JSON owns
# the ruleset's identity, enforcement, zero-bypass invariant, strictness, and
# exact required-check identities; OpenTofu owns only the deployment.
locals {
  dear_agent_main_ruleset = jsondecode(file("${path.root}/../.github/rulesets/main.json"))

  dear_agent_required_status_checks_rule = one([
    for rule in local.dear_agent_main_ruleset.rules : rule
    if rule.type == "required_status_checks"
  ])

  dear_agent_pull_request_rule = one([
    for rule in local.dear_agent_main_ruleset.rules : rule
    if rule.type == "pull_request"
  ])

  dear_agent_supported_rule_types = toset([
    "deletion",
    "non_fast_forward",
    "required_linear_history",
    "pull_request",
    "required_status_checks",
  ])

  dear_agent_ruleset = {
    name          = local.dear_agent_main_ruleset.name
    target        = local.dear_agent_main_ruleset.target
    enforcement   = local.dear_agent_main_ruleset.enforcement
    bypass_actors = local.dear_agent_main_ruleset.bypass_actors
    conditions = {
      ref_name = {
        include = local.dear_agent_main_ruleset.conditions.ref_name.include
        exclude = local.dear_agent_main_ruleset.conditions.ref_name.exclude
      }
    }
    policy_validation = {
      unsupported_rule_types = tolist(setsubtract(
        toset([for rule in local.dear_agent_main_ruleset.rules : rule.type]),
        local.dear_agent_supported_rule_types,
      ))
      unsupported_condition_keys = tolist(setsubtract(
        toset(keys(local.dear_agent_main_ruleset.conditions)),
        toset(["ref_name"]),
      ))
      unsupported_pull_request_parameter_keys = tolist(setsubtract(
        toset(keys(local.dear_agent_pull_request_rule.parameters)),
        toset([
          "allowed_merge_methods",
          "required_approving_review_count",
          "dismiss_stale_reviews_on_push",
          "require_code_owner_review",
          "require_last_push_approval",
          "required_review_thread_resolution",
          "required_reviewers",
        ]),
      ))
      unsupported_status_check_parameter_keys = tolist(setsubtract(
        toset(keys(local.dear_agent_required_status_checks_rule.parameters)),
        toset([
          "strict_required_status_checks_policy",
          "do_not_enforce_on_create",
          "required_status_checks",
        ]),
      ))
      # Preserve a narrow, explicit subset instead of relying on Terraform's
      # structural conversion, which otherwise discards unknown nested keys.
      unsupported_policy_paths = concat(
        [
          for key in keys(local.dear_agent_main_ruleset) : "ruleset.${key}"
          if !contains(["name", "target", "enforcement", "bypass_actors", "conditions", "rules"], key)
        ],
        flatten([
          for rule in local.dear_agent_main_ruleset.rules : [
            for key in keys(rule) : "rules.${rule.type}.${key}"
            if !contains(["type", "parameters"], key)
          ]
        ]),
        flatten([
          for rule in local.dear_agent_main_ruleset.rules : [
            for key in try(keys(rule.parameters), []) : "rules.${rule.type}.parameters.${key}"
          ] if contains(["deletion", "non_fast_forward", "required_linear_history"], rule.type)
        ]),
        [
          for rule in local.dear_agent_main_ruleset.rules : "rules.${rule.type}.parameters"
          if contains(["deletion", "non_fast_forward", "required_linear_history"], rule.type) &&
          try(rule.parameters, null) != null && !can(keys(rule.parameters))
        ],
        [
          for key in keys(local.dear_agent_main_ruleset.conditions.ref_name) : "conditions.ref_name.${key}"
          if !contains(["include", "exclude"], key)
        ],
        flatten([
          for required_reviewer in local.dear_agent_pull_request_rule.parameters.required_reviewers : [
            for key in keys(required_reviewer) : "rules.pull_request.parameters.required_reviewers.${key}"
            if !contains(["file_patterns", "minimum_approvals", "reviewer"], key)
          ]
        ]),
        flatten([
          for required_reviewer in local.dear_agent_pull_request_rule.parameters.required_reviewers : [
            for key in keys(required_reviewer.reviewer) : "rules.pull_request.parameters.required_reviewers.reviewer.${key}"
            if !contains(["id", "type"], key)
          ]
        ]),
        flatten([
          for check in local.dear_agent_required_status_checks_rule.parameters.required_status_checks : [
            for key in keys(check) : "rules.required_status_checks.parameters.required_status_checks.${key}"
            if !contains(["context", "integration_id"], key)
          ]
        ]),
      )
    }
    rules = {
      # These three GitHub rules are presence-only. A canonical document that
      # omits or duplicates one fails at one(...) below rather than quietly
      # inheriting an OpenTofu default.
      deletion = one([for rule in local.dear_agent_main_ruleset.rules : true if rule.type == "deletion"])
      non_fast_forward = one([
        for rule in local.dear_agent_main_ruleset.rules : true
        if rule.type == "non_fast_forward"
      ])
      required_linear_history = one([
        for rule in local.dear_agent_main_ruleset.rules : true
        if rule.type == "required_linear_history"
      ])
      pull_request = {
        required_approving_review_count   = local.dear_agent_pull_request_rule.parameters.required_approving_review_count
        dismiss_stale_reviews_on_push     = local.dear_agent_pull_request_rule.parameters.dismiss_stale_reviews_on_push
        require_code_owner_review         = local.dear_agent_pull_request_rule.parameters.require_code_owner_review
        require_last_push_approval        = local.dear_agent_pull_request_rule.parameters.require_last_push_approval
        required_review_thread_resolution = local.dear_agent_pull_request_rule.parameters.required_review_thread_resolution
        required_reviewers                = local.dear_agent_pull_request_rule.parameters.required_reviewers
        allowed_merge_methods             = local.dear_agent_pull_request_rule.parameters.allowed_merge_methods
      }
      required_status_checks = {
        enabled                              = true
        strict_required_status_checks_policy = local.dear_agent_required_status_checks_rule.parameters.strict_required_status_checks_policy
        do_not_enforce_on_create             = local.dear_agent_required_status_checks_rule.parameters.do_not_enforce_on_create
        required_checks = [
          for check in local.dear_agent_required_status_checks_rule.parameters.required_status_checks : {
            context        = check.context
            integration_id = try(check.integration_id, null)
          }
        ]
      }
    }
  }
}

# The repo inventory (active_repos / archived_repos) previously lived here as
# locals. Because this repo is PUBLIC, enumerating private repo names and their
# per-repo security posture here leaked them. The inventory now lives in the
# variables `var.active_repos` / `var.archived_repos` (see variables.tf),
# populated from the gitignored infra/repos.auto.tfvars (schema in
# repos.auto.tfvars.example). This file is intentionally left as a marker.
#
# Merge-queue inputs: merge queues require an ORGANIZATION account and are
# unavailable on a personal account (see rulesets.tf). If these repos ever move
# under an org, reintroduce a merge_queue_repos input.
#
# ARCHIVED-repo handling (why archived repos stay declared rather than removed):
# GitHub rejects ruleset/settings mutations on archived repos, and REMOVING a
# previously-managed repo would make `tofu apply` propose DESTROYING its
# github_repository resource — which deletes the repo on GitHub. So archived
# repositories are kept in var.archived_repos, declared minimally with all
# changes ignored. Their identities stay in the private inventory.
#
# PENDING ONBOARDING (from the reconciliation guard added in this PR, ce-1onr):
# Any live-but-unmanaged repositories reported by reconciliation must be added
# to the gitignored private tfvars out of band. Do not copy their identities or
# classifications into committed source. Repositories without CI use
# required_checks = [].
