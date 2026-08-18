output "repository_name" {
  description = "The managed repository's name."
  value       = github_repository.this.name
}

output "repository_node_id" {
  description = "GraphQL node ID of the repository (for resources that key on node_id)."
  value       = github_repository.this.node_id
}

output "repository_full_name" {
  description = "owner/name slug of the repository."
  value       = github_repository.this.full_name
}

output "ruleset_id" {
  description = "ID of the branch-protection ruleset."
  value       = github_repository_ruleset.branch_protection.id
}

output "claude_review_enabled" {
  description = "Whether the Claude Code PR-review workflow + secret are installed on this repo."
  value       = var.enable_claude_review
}
