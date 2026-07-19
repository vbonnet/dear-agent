###############################################################################
# Claude Code PR review — fleet rollout.
#
# dear-agent is the REFERENCE repo: it hand-maintains
# .github/workflows/claude-code-review.yml directly in git (see that file for
# the actual trigger/permissions/action config). This file reads that same
# content and republishes it, byte-for-byte, to every OTHER repo that opts in
# via `enable_claude_review = true` in its infra/repos.auto.tfvars entry — one
# source of truth, no drift between "the repo everyone copy-pastes from" and
# what's actually deployed elsewhere.
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
# mechanical schema). Default the fleet split like this:
#
#   Non-PII repos (safe default: enable_claude_review = true)
#     dear-agent        — excluded from the fan-out (see above); already live
#                          via its own committed workflow file.
#     ai-tools
#     codebase-analyzer
#     gdoc-sync
#     vbonnet.ai
#
#   Deliberate private-repo opt-in (enable_claude_review = true; owner
#   sign-off recorded 2026-07-19 — code from this repo ships to Anthropic's
#   API on every PR, same as the public repos above, but the repo itself is
#   private):
#     engram-research
#
#   PII repos (opt-in only, still OFF — enabling ships code to Anthropic's
#   API for review; that's a data-handling decision for a human, not a
#   default this IaC should make):
#     engram-kb
#     brain-v2
#     ai-conversation-logs
#   Leave `enable_claude_review` unset (defaults to false) for these until a
#   human explicitly decides to opt one in.
###############################################################################

locals {
  claude_review_workflow_content = file("${path.module}/../.github/workflows/claude-code-review.yml")
}
