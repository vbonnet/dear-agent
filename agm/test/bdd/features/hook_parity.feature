# SPEC: internal/hookparity/SPEC.md
# RELATED-SPEC: agm/internal/hooks/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/posttool-context-monitor/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/posttool-response-masking/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/posttool-worktree-tracker/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/pre-merge-commit/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/pretool-bash-blocker/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/pretool-interrupt-check/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/pretool-npm-safety/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/pretool-test-session-guard/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/sessionend-state-reporter/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/stop-state-reporter/SPEC.md
# RELATED-SPEC: agm/cmd/agm-hooks/userpromptsubmit-state-reporter/SPEC.md
# RELATED-SPEC: agm/hooks/cmd/posttool-cost-guard/SPEC.md
# RELATED-SPEC: agm/hooks/cmd/sessionstart-chezmoi-drift/SPEC.md
# RELATED-SPEC: agm/hooks/cmd/stop-session-guard/SPEC.md
Feature: Hook harness parity
  Active interactive harnesses should receive the same repository guardrails
  through their native hook configuration surfaces.

  Scenario Outline: Active harness hook manifests expose shared guardrails
    Given hook harness "<harness>" is configured
    When AGM validates hook parity for that harness
    Then hook harness "<harness>" should include guardrail hook "<guardrail>"

    Examples:
      | harness      | guardrail                  |
      | claude-code  | pretool-spawn-routing      |
      | claude-code  | pretool-bead-close-guard   |
      | claude-code  | pretool-bypass-guard       |
      | claude-code  | pretool-pr-guard           |
      | claude-code  | stop-guardrail-feedback    |
      | codex-cli    | pretool-spawn-routing      |
      | codex-cli    | pretool-bead-close-guard   |
      | codex-cli    | pretool-bypass-guard       |
      | codex-cli    | pretool-pr-guard           |
      | codex-cli    | stop-guardrail-feedback    |
      | agy          | pretool-spawn-routing      |
      | agy          | pretool-bead-close-guard   |
      | agy          | pretool-bypass-guard       |
      | agy          | pretool-pr-guard           |
      | agy          | stop-guardrail-feedback    |
      | opencode-cli | pretool-spawn-routing      |
      | opencode-cli | pretool-bead-close-guard   |
      | opencode-cli | pretool-bypass-guard       |
      | opencode-cli | pretool-pr-guard           |
      | opencode-cli | stop-guardrail-feedback    |

  Scenario Outline: Non-Claude hook manifests expose Beads lifecycle hooks
    Given hook harness "<harness>" is configured
    When AGM validates hook parity for that harness
    Then hook harness "<harness>" should include Beads lifecycle hook "<event>"

    Examples:
      | harness      | event            |
      | codex-cli    | SessionStart     |
      | codex-cli    | UserPromptSubmit |
      | codex-cli    | PreCompact       |
      | codex-cli    | PostCompact      |
      | agy          | SessionStart     |
      | agy          | UserPromptSubmit |
      | agy          | PreCompact       |
      | agy          | PostCompact      |
      | opencode-cli | SessionStart     |
      | opencode-cli | UserPromptSubmit |
      | opencode-cli | PreCompact       |
      | opencode-cli | PostCompact      |
