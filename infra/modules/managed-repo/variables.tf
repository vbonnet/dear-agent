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
