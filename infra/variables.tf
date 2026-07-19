variable "personal_owner" {
  description = "GitHub personal account that owns vbonnet/* repositories."
  type        = string
  default     = "vbonnet"
}

variable "org_name" {
  description = "GitHub organization name for dear-labs/* repositories."
  type        = string
  default     = "dear-labs"
}

# NOTE: There is intentionally NO github_token variable.
# The integrations/github provider reads GITHUB_TOKEN from the environment.
# Keeping the token out of Terraform variables guarantees it can never land
# in *.tfvars files or state.

# The repo INVENTORY was moved out of locals.tf into these variables so the
# private repo names + per-repo security posture do NOT ship in this PUBLIC
# repo. Real values live in the gitignored infra/repos.auto.tfvars (see
# repos.auto.tfvars.example for the schema). No defaults => a plan/apply without
# the tfvars fails loudly rather than managing an empty fleet.
variable "active_repos" {
  description = "Non-archived managed repositories, keyed by repo name. Populated from the gitignored repos.auto.tfvars."
  type = map(object({
    visibility      = string
    default_branch  = optional(string, "main")
    required_checks = optional(list(string), [])
    # Installs .github/workflows/claude-code-review.yml (content sourced from
    # dear-agent's own hand-maintained copy, the reference implementation —
    # see claude_review.tf) plus the CLAUDE_CODE_OAUTH_TOKEN secret. Advisory
    # review only; never added to required_checks. dear-agent itself is
    # deliberately excluded from this flag (see claude_review.tf) since it
    # hand-maintains its own workflow files as the source of truth.
    enable_claude_review = optional(bool, false)
  }))
}

variable "archived_repos" {
  description = "Archived managed repositories (frozen; all changes ignored). From repos.auto.tfvars."
  type = map(object({
    visibility = optional(string, "private")
  }))
}

# Claude Code OAuth token (from `claude setup-token`, a Pro/Max subscription
# credential — NOT an Anthropic Console API key). Shared across every repo
# that has enable_claude_review = true; GitHub secrets are per-repo (these are
# personal-account repos, so no org-level secret is available to share it
# fleet-wide — see the "No merge_queue" note in modules/managed-repo for the
# same personal-account constraint). Supply via TF_VAR_claude_code_oauth_token
# in the environment — never in a committed *.tfvars file. Sensitive: still
# lands in the OpenTofu state file (github_actions_secret has no encrypted-only
# write mode), so the R2 state bucket must stay private (see backend.tf).
variable "claude_code_oauth_token" {
  description = "Claude Code OAuth token applied to every repo with enable_claude_review = true. Null-safe: repos without the flag never reference it."
  type        = string
  default     = null
  sensitive   = true
}
