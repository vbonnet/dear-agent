# SPEC: .github/workflows/SPEC.md
# RELATED-SPEC: agm/test/coverage/SPEC.md
# RELATED-SPEC: .agents/skills/beads/agents/SPEC.md
# RELATED-SPEC: .github/SPEC.md
# RELATED-SPEC: .github/act/SPEC.md
# RELATED-SPEC: .github/rulesets/SPEC.md
# RELATED-SPEC: agm/.claude-plugin/SPEC.md
# RELATED-SPEC: spec-governance/.claude-plugin/SPEC.md
# RELATED-SPEC: agm/.github/workflows/SPEC.md
# RELATED-SPEC: agm/agm-plugin/.claude-plugin/SPEC.md
# RELATED-SPEC: agm/agm-plugin/channels/agm-bus/SPEC.md
# RELATED-SPEC: agm/cmd/agm/schedules/SPEC.md
# RELATED-SPEC: agm/contracts/SPEC.md
# RELATED-SPEC: agm/schemas/SPEC.md
# RELATED-SPEC: agm/systemd/SPEC.md
# RELATED-SPEC: agm/test/e2e/docker/SPEC.md
# RELATED-SPEC: agm/youtube-plugin/.claude-plugin/SPEC.md
# RELATED-SPEC: cmd/dear-agent-bumblebee/templates/SPEC.md
# RELATED-SPEC: config/SPEC.md
# RELATED-SPEC: configs/workflows/SPEC.md
# RELATED-SPEC: deploy/SPEC.md
# RELATED-SPEC: deploy/launchd/SPEC.md
# RELATED-SPEC: pkg/codeintel/rules/go/SPEC.md
# RELATED-SPEC: pkg/codeintel/rules/python/SPEC.md
# RELATED-SPEC: pkg/codeintel/rules/typescript/SPEC.md
# RELATED-SPEC: spec-governance/SPEC.md
# RELATED-SPEC: wayfinder/.claude-plugin/SPEC.md
Feature: Declarative runtime guardrails
  Runtime configuration is executable product behavior. Plugin manifests,
  workflows, contracts, schemas, schedules, deployment services, and analysis
  rules must remain strict, traceable, and route-neutral.

  Scenario Outline: Declarative runtime directories have executable contracts
    Given declarative runtime directory "<directory>" is configured
    When AGM validates declarative runtime coverage
    Then declarative runtime directory "<directory>" should have a co-located SPEC

    Examples:
      | directory                                                                    |
      | .agents/skills/beads/agents                                                  |
      | .github                                                                      |
      | .github/act                                                                  |
      | .github/rulesets                                                             |
      | .github/workflows                                                            |
      | agm/.claude-plugin                                                           |
      | agm/.github/workflows                                                        |
      | agm/agm-plugin/.claude-plugin                                                |
      | agm/agm-plugin/channels/agm-bus                                              |
      | agm/cmd/agm/schedules                                                        |
      | agm/contracts                                                                |
      | agm/schemas                                                                  |
      | agm/systemd                                                                  |
      | agm/test/e2e/docker                                                          |
      | agm/youtube-plugin/.claude-plugin                                            |
      | cmd/dear-agent-bumblebee/templates                                           |
      | config                                                                       |
      | configs/workflows                                                            |
      | deploy                                                                       |
      | deploy/launchd                                                               |
      | pkg/codeintel/rules/go                                                       |
      | pkg/codeintel/rules/python                                                   |
      | pkg/codeintel/rules/typescript                                               |
      | wayfinder/.claude-plugin                                                     |

  Scenario Outline: Declarative contracts are invariant across active routes
    Given declarative runtime coverage runs through "<harness>" with "<family>"
    When AGM validates declarative runtime route parity
    Then every declarative runtime contract should retain strict SPEC and BDD traceability

    Examples:
      | harness       | family    |
      | claude-code   | anthropic |
      | claude-code   | openai    |
      | claude-code   | gemini    |
      | claude-code   | glm       |
      | claude-code   | deepseek  |
      | claude-code   | nemotron  |
      | claude-code   | qwen      |
      | codex-cli     | anthropic |
      | codex-cli     | openai    |
      | codex-cli     | gemini    |
      | codex-cli     | glm       |
      | codex-cli     | deepseek  |
      | codex-cli     | nemotron  |
      | codex-cli     | qwen      |
      | agy           | anthropic |
      | agy           | openai    |
      | agy           | gemini    |
      | agy           | glm       |
      | agy           | deepseek  |
      | agy           | nemotron  |
      | agy           | qwen      |
      | opencode-cli  | anthropic |
      | opencode-cli  | openai    |
      | opencode-cli  | gemini    |
      | opencode-cli  | glm       |
      | opencode-cli  | deepseek  |
      | opencode-cli  | nemotron  |
      | opencode-cli  | qwen      |
      | pi-cli        | anthropic |
      | pi-cli        | openai    |
      | pi-cli        | gemini    |
      | pi-cli        | glm       |
      | pi-cli        | deepseek  |
      | pi-cli        | nemotron  |
      | pi-cli        | qwen      |

  Scenario: CI schedules credential-free Codex contract evidence
    Given the repository CI workflow is configured
    When AGM validates the Codex contract CI job
    Then CI should run portable active harness parity
    And CI should run the isolated source-built Codex lifecycle
    And CI should enforce critical lifecycle coverage
    And scheduled CI should run the credential-free tagged graphs
