# Automated PR code review — Claude + Codex setup

- **Status:** authoritative
- **Last audited:** 2026-07-23

Two independent, advisory (non-blocking) review bots comment on PRs across the
fleet: Claude (via `anthropics/claude-code-action`) and OpenAI Codex cloud
review. Neither is wired into `required_status_checks` — a bot review is a
second opinion, not a merge gate. This doc is the one-time manual checklist;
everything else is codified (workflow files + OpenTofu).

## What's IaC-able vs. click-ops

| Piece | Mechanism | Reproducible? |
|---|---|---|
| Claude review workflow file (`.github/workflows/claude-code-review.yml`) | Hand-committed in dear-agent (reference repo); staged on a dedicated rollout branch via `github_repository_file`, then opened as a normal PR by `infra/modules/managed-repo` | ✅ IaC stages it; a human merges the rollout PR |
| `CLAUDE_CODE_OAUTH_TOKEN` repo secret | `github_actions_secret` in the same module, value from `TF_VAR_claude_code_oauth_token` | ✅ IaC, given the token value |
| Claude GitHub App install | Install the Claude GitHub App on every opted-in repository | ⚠️ Manual, one-time, per repository |
| Codex cloud review enablement | Toggle at `chatgpt.com/codex/settings/code-review` per repo | ❌ Click-ops only — no public API/CLI as of 2026-07 |
| Codex review guidelines | Optional `## Review guidelines` section in the repo's `AGENTS.md` | ✅ Just a file edit, but repo-owned content — not something this infra should overwrite |
| Which repos get Claude review | `enable_claude_review = true` per repo in `infra/repos.auto.tfvars`; set `claude_review_rollout = true` only while staging its workflow PR | ✅ IaC |
| Branch protection / required checks | `infra/modules/managed-repo` ruleset | ✅ IaC (already existed; deliberately NOT extended to include either review bot) |

## One-time manual steps (do these once, in order)

1. **Generate a Claude Code OAuth token** (requires a Claude Pro or Max
   subscription — this is NOT an Anthropic Console API key, and it isn't
   billed per-token like the Console API):
   ```
   claude setup-token
   ```
   This prints a token. It is stored as the `CLAUDE_CODE_OAUTH_TOKEN`
   secret on `vbonnet/dear-agent`. For
   every other repo in the rollout, this same token value becomes
   `TF_VAR_claude_code_oauth_token` for `tofu apply` (see below) — GitHub
   secrets are per-repo, there's no fleet-wide secret on a personal account.

2. **Install the Claude GitHub App in every opted-in rollout repository.**
   The OAuth token authenticates the workflow to Anthropic, but the App grants
   Claude GitHub-side authorization to publish review comments. Complete the
   App installation for each repository selected by `enable_claude_review`
   before merging that repository's rollout PR. Terraform can stage the
   workflow and secret, but it cannot perform this App installation.

3. **Enable Codex cloud review** at
   <https://chatgpt.com/codex/settings/code-review> (per repo, click-ops —
   there is no Terraform provider or public API for this as of 2026-07). This
   installs OpenAI's own GitHub App on the repos you select; you'll be
   prompted for repo access during that flow.

4. **(Optional) Add Codex review guidelines.** Codex reads `## Review
   guidelines` in a repo's `AGENTS.md` if present. Not added automatically by
   this change — edit each repo's `AGENTS.md` by hand if you want
   repo-specific guidance for Codex.

5. **Roll out to other repos** — see `infra/claude_review.tf` for the default
   split. Non-PII repos default to `enable_claude_review = true`
   (ai-tools, codebase-analyzer, gdoc-sync, vbonnet.ai).
   **`engram-research` is also enabled** — a deliberate private-repo opt-in,
   not part of the default non-PII set: it's
   private, and enabling review still ships its code to Anthropic's API on
   every PR, same as the public repos. The remaining PII repos (engram-kb,
   brain-v2, ai-conversation-logs) stay a commented opt-in block —
   enabling review on those is a data-handling call this IaC deliberately
   does not make for you. Once `repos.auto.tfvars` reflects your choice:
   ```
   cd infra
   export GITHUB_TOKEN="$(gh auth token)"
   export TF_VAR_claude_code_oauth_token="<token from step 1>"
   tofu init -backend-config=backend.hcl
   tofu plan   # review before applying
   tofu apply  # opens or updates a rollout PR; review and merge that PR normally
   ```
   After the rollout PR merges, set `claude_review_rollout = false` and apply
   again. GitHub deletes merged PR branches, so this removes the transient
   branch/PR resources from Terraform state without recreating them; the secret
   remains managed by `enable_claude_review = true`.

## Workflow guards

Both workflows use the `claude_code_oauth_token` auth path and the official
`code-review` plugin. Because dear-agent is public, they carry these guards:

- **Same-repo + draft guard** (`claude-code-review.yml`): the privileged
  review action runs only for non-draft PRs from same-repo branches — GitHub
  does not pass Actions secrets to a `pull_request` workflow from a fork.
- **`@claude` author allowlist** (`claude.yml`): the mention trigger fires
  only for OWNER/MEMBER/COLLABORATOR authors, so an arbitrary GitHub user
  cannot spend the repo owner's subscription or exercise the prompt-injection
  surface.
- **IaC reproducibility**: `infra/claude_review.tf` + `modules/managed-repo`
  stage the same setup on every opted-in repo.

## Adjacent workflow: 5-dimension review

`.github/workflows/review.yml` ("AI Code Review (5-dimension)") is a separate,
fail-closed check driven by `cmd/ai-review` (Go); the contract, including the
neutral paths taken while no reviewer credential is configured, is documented
in the comment at the top of the workflow file. It is not a required context
in `.github/rulesets/main.json` — see `docs/branch-protection.md`.
