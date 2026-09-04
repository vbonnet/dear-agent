# SPEC: SPEC.md
# RELATED-SPEC: .agents/hooks/SPEC.md
# RELATED-SPEC: .claude/hooks/SPEC.md
# RELATED-SPEC: .deepsec/SPEC.md
# RELATED-SPEC: .opencode/hooks/SPEC.md
# RELATED-SPEC: agm/.githooks/SPEC.md
# RELATED-SPEC: agm/agm-plugin/channels/agm-bus/src/SPEC.md
# RELATED-SPEC: agm/cmd/agm-bus/contrib/SPEC.md
# RELATED-SPEC: agm/cmd/agm/hooks/SPEC.md
# RELATED-SPEC: agm/docs/hooks/SPEC.md
# RELATED-SPEC: agm/hooks/SPEC.md
# RELATED-SPEC: agm/hooks/cmd/SPEC.md
# RELATED-SPEC: agm/internal/dolt/migrations/SPEC.md
# RELATED-SPEC: agm/migrations/SPEC.md
# RELATED-SPEC: agm/scripts/hooks/SPEC.md
# RELATED-SPEC: agm/test/bdd/steps/SPEC.md
# RELATED-SPEC: agm/test/e2e/SPEC.md
# RELATED-SPEC: agm/test/e2e/docker/scripts/SPEC.md
# RELATED-SPEC: agm/test/e2e/lib/SPEC.md
# RELATED-SPEC: agm/test/e2e/suites/SPEC.md
# RELATED-SPEC: agm/tests/SPEC.md
# RELATED-SPEC: engram/ecphory/diagrams/SPEC.md
# RELATED-SPEC: engram/hooks-bin/SPEC.md
# RELATED-SPEC: engram/hooks-bin/lib/SPEC.md
# RELATED-SPEC: engram/mcp/SPEC.md
# RELATED-SPEC: engram/mcp/src/SPEC.md
# RELATED-SPEC: infra/SPEC.md
# RELATED-SPEC: infra/modules/managed-repo/SPEC.md
# RELATED-SPEC: scripts/SPEC.md
# RELATED-SPEC: tests/bats/SPEC.md
# RELATED-SPEC: tools/devlog/diagrams/SPEC.md
Feature: Cross-language implementation guardrails
  Executable behavior outside Go packages is part of the same product and
  governance boundary. Its contracts must remain strict, executable, and
  invariant across every active harness and supported model family.

  Scenario Outline: Implementation directories declare executable contracts
    Given cross-language implementation directory "<directory>" is configured
    When AGM validates cross-language implementation coverage
    Then cross-language implementation directory "<directory>" should have a co-located SPEC

    Examples:
      | directory                                                                    |
      | .                                                                            |
      | .agents/hooks                                                                |
      | .claude/hooks                                                                |
      | .deepsec                                                                     |
      | .opencode/hooks                                                              |
      | agm/.githooks                                                                |
      | agm/agm-plugin/channels/agm-bus/src                                          |
      | agm/cmd/agm-bus/contrib                                                      |
      | agm/cmd/agm/hooks                                                            |
      | agm/docs/hooks                                                               |
      | agm/hooks                                                                    |
      | agm/hooks/cmd                                                                |
      | agm/internal/dolt/migrations                                                 |
      | agm/migrations                                                               |
      | agm/scripts/hooks                                                            |
      | agm/test/e2e/docker/scripts                                                  |
      | agm/test/e2e/lib                                                             |
      | agm/test/e2e/suites                                                          |
      | agm/tests                                                                    |
      | engram/ecphory/diagrams                                                      |
      | engram/hooks-bin                                                             |
      | engram/hooks-bin/lib                                                         |
      | engram/mcp/src                                                               |
      | infra                                                                        |
      | infra/modules/managed-repo                                                   |
      | pkg/workspace/dolt/testdata/migrations                                       |
      | scripts                                                                      |
      | tests/bats                                                                   |
      | tools/devlog/diagrams                                                        |
      | wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/eslint-flat    |

  Scenario Outline: Cross-language contracts are invariant across active routes
    Given cross-language coverage runs through "<harness>" with "<family>"
    When AGM validates cross-language route parity
    Then every cross-language implementation should retain strict SPEC and BDD traceability

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

  Scenario: End-to-end harness lookup supports the platform system shell
    Given the AGM end-to-end harness detection helper is configured
    When AGM validates portable harness command lookup
    Then the exact harness mapping should run under macOS system Bash
