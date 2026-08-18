# SPEC: agm/test/golden/SPEC.md
# RELATED-SPEC: agm/internal/testdata/file-provenance/SPEC.md
# RELATED-SPEC: agm/internal/testdata/mock-manifests/SPEC.md
# RELATED-SPEC: agm/internal/testdata/orphan-recovery/SPEC.md
# RELATED-SPEC: agm/test/golden/agent-interactions/SPEC.md
# RELATED-SPEC: agm/tests/e2e-install/Dockerfiles/SPEC.md
# RELATED-SPEC: benchmarks/baselines/SPEC.md
# RELATED-SPEC: engram/internal/health/testdata/SPEC.md
# RELATED-SPEC: pkg/config-loader/testdata/SPEC.md
# RELATED-SPEC: pkg/workspace/testdata/SPEC.md
# RELATED-SPEC: tools/dod-enforcer/examples/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/eslint/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/golangci/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/python/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/status/testdata/SPEC.md
Feature: Declarative fixture guardrails
  Golden data and deliberately valid, invalid, archived, corrupt, and
  cross-platform fixtures define observable behavior. Their contracts must
  remain strict and invariant across every active route.

  Scenario Outline: Declarative fixture directories have executable contracts
    Given declarative fixture directory "<directory>" is configured
    When AGM validates declarative fixture coverage
    Then declarative fixture directory "<directory>" should have a co-located SPEC

    Examples:
      | directory                                                                        |
      | agm/internal/testdata/file-provenance                                            |
      | agm/internal/testdata/mock-manifests                                             |
      | agm/internal/testdata/orphan-recovery                                            |
      | agm/test/golden                                                                  |
      | agm/test/golden/agent-interactions                                               |
      | agm/tests/e2e-install/Dockerfiles                                                |
      | benchmarks/baselines                                                             |
      | engram/internal/health/testdata                                                  |
      | pkg/config-loader/testdata                                                       |
      | pkg/workspace/testdata                                                           |
      | tools/dod-enforcer/examples                                                      |
      | wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/eslint             |
      | wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/golangci           |
      | wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/python             |
      | wayfinder/cmd/wayfinder-session/internal/status/testdata                         |

  Scenario Outline: Fixture contracts are invariant across active routes
    Given declarative fixture coverage runs through "<harness>" with "<family>"
    When AGM validates declarative fixture route parity
    Then every declarative fixture contract should retain strict SPEC and BDD traceability

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
