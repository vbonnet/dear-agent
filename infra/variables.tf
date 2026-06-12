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
