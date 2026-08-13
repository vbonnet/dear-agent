# SPEC: internal/hookparity/SPEC.md
# RELATED-SPEC: .opencode/SPEC.md
# RELATED-SPEC: .opencode/hooks/SPEC.md
# RELATED-SPEC: .codex/hooks/SPEC.md
# RELATED-SPEC: .pi/guardrails/SPEC.md
# RELATED-SPEC: agm/internal/permissionparity/piadapter/SPEC.md
# RELATED-SPEC: scripts/git-hooks/SPEC.md
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
# RELATED-SPEC: cmd/pretool-bash-write-guard/SPEC.md
# RELATED-SPEC: cmd/pretool-fs-write-guard/SPEC.md
# RELATED-SPEC: pkg/version/SPEC.md
# RELATED-SPEC: tests/buildstamp/SPEC.md
Feature: Hook harness parity
  Active interactive harnesses should receive equivalent SPEC review outcomes
  through their native capabilities without inventing unsupported events.

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
      | opencode-cli | pretool-spawn-routing      |
      | opencode-cli | pretool-bead-close-guard   |
      | opencode-cli | pretool-bypass-guard       |
      | opencode-cli | pretool-pr-guard           |
      | pi-cli       | pretool-spawn-routing      |
      | pi-cli       | pretool-bead-close-guard   |
      | pi-cli       | pretool-bypass-guard       |
      | pi-cli       | pretool-pr-guard           |
      | pi-cli       | stop-guardrail-feedback    |

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
      | pi-cli       | SessionStart     |
      | pi-cli       | UserPromptSubmit |
      | pi-cli       | PreCompact       |
      | pi-cli       | PostCompact      |

  Scenario Outline: Active harness capabilities expose bounded SPEC review
    Given hook harness "<harness>" is configured
    When AGM validates hook parity for that harness
    Then hook harness "<harness>" should expose bounded SPEC contract review

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario Outline: Capability gaps omit unsupported legacy projections
    Given hook harness "<harness>" is configured
    When AGM validates hook parity for that harness
    Then hook harness "<harness>" should omit unsupported legacy hook projections

    Examples:
      | harness      |
      | agy          |
      | opencode-cli |

  Scenario: Provider projections share the canonical SPEC authoring route
    Given staged SPEC contract feedback is configured
    When AGM exercises the shared reminder across all projected harness adapters
    Then every reminder should route to the canonical authoring page and single-source skill

  Scenario: Sibling continuations preserve fresh bounded SPEC feedback
    Given terminal SPEC feedback identity is configured
    When AGM exercises sibling continuations and repeated SPEC identities across native terminal adapters
    Then fresh SPEC identities should block once while repeats yield without claiming compliance

  Scenario: Installed helper status compares reproducible expected bytes
    Given installed SPEC helper status is configured
    When AGM rebuilds the expected helper with distinct wall-clock inputs
    Then the expected helper bytes should remain identical for unchanged source and provenance

  Scenario: Idle-session fallback bounds recursive SPEC feedback and adapter cleanup
    Given OpenCode idle-session SPEC feedback is configured
    When AGM exercises repeated, synthetic, capacity, deletion, and supervisor lifecycle events
    Then OpenCode feedback and adapter cleanup should remain bounded and identity-safe

  Scenario: Pi terminal aggregation and supervisor cleanup have end-to-end resource bounds
    Given Pi terminal hook aggregation is configured
    When AGM exercises Pi terminal handler and supervisor lifecycle bounds
    Then Pi should fail closed within its budgets while preserving aggregation and identity-safe cleanup

  Scenario Outline: Repository post-merge hook exposes lifecycle safeguards
    Given the repository post-merge hook is configured
    When AGM validates repository post-merge hook coverage
    Then the repository post-merge hook should include lifecycle safeguard "<safeguard>"

    Examples:
      | safeguard               |
      | atomic-binary-install   |
      | trunk-build-context     |
      | agm-companion-coherence |
      | wayfinder-runtime-deploy |
      | host-artifact-deploy    |
      | deployment-verification |
      | bead-transition         |
      | worktree-sweep          |
      | fail-safe-exit          |

  Scenario: Detached archive companion proves coherent startup
    When AGM runs detached archive companion startup regressions
    Then a mixed revision or missing startup acknowledgement should fail before async success

  Scenario: Canonical AGM installation preserves companion coherence
    When AGM renders the canonical AGM companion install plan
    Then the root AGM install plan should build and install the companion pair
