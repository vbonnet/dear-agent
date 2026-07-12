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
    default_branch  = string
    required_checks = list(string)
  }))
}

variable "archived_repos" {
  description = "Archived managed repositories (frozen; all changes ignored). From repos.auto.tfvars."
  type = map(object({
    visibility = string
  }))
}
