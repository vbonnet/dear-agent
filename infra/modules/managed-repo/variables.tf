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

variable "required_checks" {
  description = <<-EOT
    Exact GitHub Actions check-run names that must pass before merge. An empty
    list yields PR-required branch protection with no status-check gate (the
    required_status_checks rule is omitted entirely to avoid a no-op rule).
    Derive names from:
      gh api /repos/<owner>/<repo>/commits/<branch>/check-runs \
        --jq '[.check_runs[].name] | unique[]'
  EOT
  type        = list(string)
  default     = []
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
