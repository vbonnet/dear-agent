# SPEC: agm/internal/harnessexec/SPEC.md
Feature: Private sandbox launch authority
  Private Claude Code and Codex CLI executors must consume launch authority
  without reviving stale environment state or skipping final disk admission.

  Scenario: Private handoffs replace stale authority without command authority encoding
    When AGM validates private sandbox authority handoffs
    Then Claude and Codex should consume exact authority without FSGUARD assignments, authority flags, or the stale inherited value in command text

  Scenario: Ordinary Codex handoffs bind the exact launch arguments
    When AGM validates ordinary Codex handoff argument binding
    Then changed ordinary Codex launch arguments should be refused without requiring override proofs

  Scenario: Managed executors revalidate the physical working directory
    When AGM validates managed private executor workspace containment
    Then Claude and Codex should reject a workspace symlink retargeted outside the exact workspace or a retargeted workspace authority

  Scenario: Private executors repeat disk admission at the final launch boundary
    When AGM validates the final private executor disk recheck
    Then the private executor should recheck the configured volume or default reader with the caller threshold after executable preparation and before optional proof commitment or process replacement
