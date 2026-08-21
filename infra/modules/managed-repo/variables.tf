variable "name" {
  description = "Repository name (the GitHub repo slug, without owner)."
  type        = string
}

variable "visibility" {
  description = <<-EOT
    Repository visibility: "public" or "private". Secret scanning and push
    protection are enabled for public repos only — private repos require GitHub
    Advanced Security, so enabling it there would fail on apply.
  EOT
  type        = string

  validation {
    condition     = contains(["public", "private"], var.visibility)
    error_message = "visibility must be \"public\" or \"private\"."
  }
}

variable "ruleset" {
  description = <<-EOT
    Explicitly supported zero-bypass branch-protection subset. Every required
    check is identified by its context and optional GitHub App integration ID,
    so reconciliation does not collapse distinct check identities to strings.
    The caller must declare an active, zero-bypass ruleset.
  EOT
  type = object({
    name        = string
    target      = string
    enforcement = string
    bypass_actors = list(object({
      actor_id    = number
      actor_type  = string
      bypass_mode = string
    }))
    conditions = object({
      ref_name = object({
        include = list(string)
        exclude = list(string)
      })
    })
    policy_validation = object({
      unsupported_rule_types                  = list(string)
      unsupported_condition_keys              = list(string)
      unsupported_pull_request_parameter_keys = list(string)
      unsupported_status_check_parameter_keys = list(string)
      unsupported_policy_paths                = optional(list(string), [])
    })
    rules = object({
      deletion                = bool
      non_fast_forward        = bool
      required_linear_history = bool
      pull_request = object({
        allowed_merge_methods             = list(string)
        required_approving_review_count   = number
        dismiss_stale_reviews_on_push     = bool
        require_code_owner_review         = bool
        require_last_push_approval        = bool
        required_review_thread_resolution = bool
        required_reviewers = list(object({
          file_patterns     = list(string)
          minimum_approvals = number
          reviewer = object({
            id   = number
            type = string
          })
        }))
      })
      required_status_checks = object({
        enabled                              = bool
        strict_required_status_checks_policy = bool
        do_not_enforce_on_create             = bool
        required_checks = list(object({
          context        = string
          integration_id = optional(number)
        }))
      })
    })
  })
}

variable "enforce_canonical_ruleset_invariants" {
  description = "Enforce dear-agent's non-negotiable canonical strict-check and GitHub Actions identity invariants. Leave false for legacy inventory-owned fleet policy."
  type        = bool
  default     = false
}

variable "default_branch" {
  description = "Default branch name, used as the target branch for rollout pull requests (e.g. the Claude review workflow)."
  type        = string
  default     = "main"
}

variable "enable_claude_review" {
  description = "Install the Claude Code OAuth secret on this repo. Set claude_review_rollout temporarily to stage the workflow through a PR. Advisory-only review; never wired into required_checks."
  type        = bool
  default     = false
}

variable "claude_review_rollout" {
  description = "Transiently stage the Claude review workflow on a rollout branch and open its PR. Set false after that PR merges so GitHub's deleted head branch is not recreated."
  type        = bool
  default     = false
}

variable "claude_review_workflow_content" {
  description = "Raw content for .github/workflows/claude-code-review.yml. Required when enable_claude_review = true; ignored otherwise."
  type        = string
  default     = null
}

variable "claude_review_rollout_branch" {
  description = "Unprotected branch OpenTofu uses to stage Claude review workflow updates before opening a PR to default_branch."
  type        = string
  default     = "automation/claude-code-review"
}

variable "claude_code_oauth_token" {
  description = "Claude Code OAuth token written to the CLAUDE_CODE_OAUTH_TOKEN repo secret. Required when enable_claude_review = true; ignored otherwise."
  type        = string
  default     = null
  sensitive   = true
}
