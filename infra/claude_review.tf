###############################################################################
# Claude Code PR review — fleet rollout.
#
# dear-agent is the REFERENCE repo: it hand-maintains
# .github/workflows/claude-code-review.yml directly in git (see that file for
# the actual trigger/permissions/action config). This file reads that same
# content and republishes it, byte-for-byte, to every OTHER repo that opts in
# via `enable_claude_review = true` plus a temporary
# `claude_review_rollout = true` in its infra/repos.auto.tfvars entry — one
# source of truth, no drift between "the repo everyone copy-pastes from" and
# what's actually deployed elsewhere. Reset the rollout flag after its PR
# merges so Terraform does not recreate GitHub's deleted head branch.
#
# dear-agent is deliberately NOT in this fan-out (see modules/managed-repo's
# claude_code_review_workflow resource, applied per repo via for_each over
# var.active_repos): writing this file back into dear-agent via the GitHub
# Contents API, while dear-agent also owns it via normal git commits, would
# fight itself — every `tofu apply` would "fix" whatever the last hand-edit
# changed. Hand-edit dear-agent's copy; everyone else gets it from here.
#
# Advisory only. This workflow is never added to required_checks (see
# managed-repo/main.tf) — a Claude review comment is a second opinion, not a
# merge gate.
#
# ROLLOUT (this is guidance for repos.auto.tfvars, which is gitignored and not
# committed to this public repo — see repos.auto.tfvars.example for the
# mechanical schema). Keep repository identities and classifications in that
# private inventory. The fleet policy is:
#
#   * dear-agent is excluded from fan-out and owns the reference workflow.
#   * Public and otherwise non-sensitive repositories may opt in by default.
#   * Every private repository requires recorded owner approval because review
#     sends pull-request code to Anthropic's API.
#   * Repositories containing PII remain off until a human explicitly approves
#     that data-handling decision.
###############################################################################

locals {
  claude_review_workflow_content = file("${path.module}/../.github/workflows/claude-code-review.yml")
}
