# SPEC: agm/test/SPEC.md
# RELATED-SPEC: agm/examples/SPEC.md
# RELATED-SPEC: agm/internal/testcontext/SPEC.md
# RELATED-SPEC: agm/internal/testutil/SPEC.md
# RELATED-SPEC: agm/scripts/SPEC.md
# RELATED-SPEC: agm/test/bdd/SPEC.md
# RELATED-SPEC: agm/test/bdd/steps/SPEC.md
# RELATED-SPEC: agm/test/contract/SPEC.md
# RELATED-SPEC: agm/test/e2e/SPEC.md
# RELATED-SPEC: agm/test/helpers/SPEC.md
# RELATED-SPEC: agm/test/integration/SPEC.md
# RELATED-SPEC: agm/test/integration/helpers/SPEC.md
# RELATED-SPEC: agm/test/integration/isolated/SPEC.md
# RELATED-SPEC: agm/test/integration/portable/SPEC.md
# RELATED-SPEC: agm/test/performance/SPEC.md
# RELATED-SPEC: agm/test/regression/SPEC.md
# RELATED-SPEC: agm/test/unit/SPEC.md
# RELATED-SPEC: engram/hooks-bin/cmd/integration_test/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/integration_test/SPEC.md
# RELATED-SPEC: engram/internal/testutil/SPEC.md
# RELATED-SPEC: internal/testutil/SPEC.md
Feature: Test support package guardrails
  Test infrastructure is part of the product's enforcement boundary. Its
  contracts must remain strict, executable, and provider-neutral.

  Scenario Outline: Test support packages declare strict executable contracts
    Given test support package "<package>" is configured
    When AGM validates test support package coverage
    Then test support package "<package>" should have a co-located SPEC

    Examples:
      | package                                                   |
      | agm/examples                                              |
      | agm/internal/testcontext                                  |
      | agm/internal/testutil                                     |
      | agm/scripts                                               |
      | agm/test                                                  |
      | agm/test/bdd                                              |
      | agm/test/bdd/steps                                        |
      | agm/test/contract                                         |
      | agm/test/e2e                                              |
      | agm/test/helpers                                          |
      | agm/test/integration                                      |
      | agm/test/integration/helpers                              |
      | agm/test/integration/isolated                             |
      | agm/test/integration/portable                             |
      | agm/test/performance                                      |
      | agm/test/regression                                       |
      | agm/test/unit                                             |
      | engram/hooks-bin/cmd/integration_test                     |
      | engram/hooks-bin/internal/integration_test                |
      | engram/internal/testutil                                  |
      | internal/testutil                                         |

  Scenario Outline: Test enforcement is invariant across active routes
    Given test support coverage runs through "<harness>" with "<family>"
    When AGM validates residual support package parity
    Then every residual support package should retain strict SPEC and BDD traceability

    Examples:
      | harness      | family    |
      | claude-code  | anthropic |
      | claude-code  | openai    |
      | claude-code  | gemini    |
      | claude-code  | glm       |
      | claude-code  | deepseek  |
      | claude-code  | nemotron  |
      | claude-code  | qwen      |
      | codex-cli    | anthropic |
      | codex-cli    | openai    |
      | codex-cli    | gemini    |
      | codex-cli    | glm       |
      | codex-cli    | deepseek  |
      | codex-cli    | nemotron  |
      | codex-cli    | qwen      |
      | agy          | anthropic |
      | agy          | openai    |
      | agy          | gemini    |
      | agy          | glm       |
      | agy          | deepseek  |
      | agy          | nemotron  |
      | agy          | qwen      |
      | opencode-cli | anthropic |
      | opencode-cli | openai    |
      | opencode-cli | gemini    |
      | opencode-cli | glm       |
      | opencode-cli | deepseek  |
      | opencode-cli | nemotron  |
      | opencode-cli | qwen      |
      | pi-cli       | anthropic |
      | pi-cli       | openai    |
      | pi-cli       | gemini    |
      | pi-cli       | glm       |
      | pi-cli       | deepseek  |
      | pi-cli       | nemotron  |
      | pi-cli       | qwen      |

  Scenario: Live harness contracts use canonical guarded CLI routes
    Given live harness contract sources are configured
    When AGM validates live harness contract command construction
    Then live harness contracts should use canonical session and harness arguments
    And unavailable live harness dependencies should be skipped explicitly
    And the credential-free active registry contract should always remain runnable
    And mock-only Pact tests should not be reported as adapter coverage

  Scenario: Trust protocol hooks restore process state and owned storage
    When AGM validates trust protocol scenario isolation
    Then trust protocol setup should run only for trust scenarios
    And trust protocol hooks should restore HOME and shared Go cache variables
    And trust protocol cleanup should remove read-only owned module trees

  Scenario: Performance workloads wait for observed client readiness
    Given AGM performance workload sources are configured
    When AGM validates performance client readiness
    Then performance workloads should use bounded hub client readiness
    And churn cleanup should be observed before stable clients disconnect

  Scenario: Real Codex lifecycle tests own their complete runtime
    Given isolated Codex lifecycle test sources are configured
    When AGM validates real lifecycle isolation
    Then the lifecycle should use a source-built AGM and unique tmux socket
    And the lifecycle should exercise send kill resume and archive through the source-built AGM
    And unexpected lifecycle setup failures should fail the test
    And cleanup should target only owned test resources
    And legacy suite opt-outs should not suppress required integration contracts

  Scenario: Named test environments remain inside one owned root
    Given named test environment lifecycle sources are configured
    When AGM validates named test environment ownership
    Then canonical creation reconstruction discovery and cleanup should share one root
    And the canonical short root should be private and scoped to the effective user
    And existing retired named environments should activate in place
    And new canonical creation should refuse a retired same-name collision
    And retired named environment paths should be discovered and removed exactly
    And overlong names should be rejected only for new environments
    And unsafe named test environment paths should be rejected before mutation
