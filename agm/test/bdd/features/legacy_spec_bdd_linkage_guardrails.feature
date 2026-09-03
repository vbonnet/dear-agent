# SPEC: wayfinder/SPEC.md
# RELATED-SPEC: internal/telemetry/SPEC.md
# RELATED-SPEC: internal/deploy/SPEC.md
# RELATED-SPEC: pkg/audit/SPEC.md
# RELATED-SPEC: pkg/audit/checks/SPEC.md
# RELATED-SPEC: cmd/fd-pressure/SPEC.md
# RELATED-SPEC: internal/fsguard/SPEC.md
# RELATED-SPEC: internal/earsbdd/SPEC.md
# RELATED-SPEC: cmd/vroom-dispatch-direct/SPEC.md
# RELATED-SPEC: cmd/agm-job/SPEC.md
# RELATED-SPEC: cmd/disk-watchdog/SPEC.md
# RELATED-SPEC: pkg/workflow/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/daemon/SPEC.md
# RELATED-SPEC: cmd/dear-deploy/SPEC.md
# RELATED-SPEC: cmd/routing-guard/SPEC.md
# RELATED-SPEC: cmd/vroom-mesh/SPEC.md
# RELATED-SPEC: agm/internal/codexarchive/SPEC.md
# RELATED-SPEC: agm/internal/circuitbreaker/SPEC.md
# RELATED-SPEC: agm/cmd/agm/SPEC.md
# RELATED-SPEC: agm/internal/codexcontrol/SPEC.md
# RELATED-SPEC: agm/cmd/agm/parity/SPEC.md
# RELATED-SPEC: agm/internal/reaper/SPEC.md
# RELATED-SPEC: agm/internal/modelrouter/SPEC.md
# RELATED-SPEC: cmd/pretool-supervisor-guard/SPEC.md
Feature: Legacy specification BDD linkage guardrails
  Strict existing requirements must link to an executable BDD surface so
  package behavior cannot drift as unreferenced documentation.

  Scenario Outline: Strict legacy specifications link to executable BDD
    Given strict legacy specification "<spec>" is selected
    When AGM validates strict legacy BDD linkage
    Then the strict legacy specification should pass EARS and reciprocal linkage

    Examples:
      | spec                                    |
      | wayfinder/SPEC.md                       |
      | internal/telemetry/SPEC.md              |
      | internal/deploy/SPEC.md                 |
      | pkg/audit/SPEC.md                       |
      | pkg/audit/checks/SPEC.md                |
      | cmd/fd-pressure/SPEC.md                 |
      | internal/fsguard/SPEC.md                |
      | internal/earsbdd/SPEC.md                |
      | cmd/vroom-dispatch-direct/SPEC.md       |
      | cmd/agm-job/SPEC.md                     |
      | cmd/disk-watchdog/SPEC.md               |
      | pkg/workflow/SPEC.md                    |
      | agm/internal/sentinel/daemon/SPEC.md    |
      | cmd/dear-deploy/SPEC.md                 |
      | cmd/routing-guard/SPEC.md               |
      | cmd/vroom-mesh/SPEC.md                  |
      | agm/internal/codexarchive/SPEC.md       |
      | agm/internal/circuitbreaker/SPEC.md     |
      | agm/cmd/agm/SPEC.md                     |
      | agm/internal/codexcontrol/SPEC.md       |
      | agm/cmd/agm/parity/SPEC.md              |
      | agm/internal/reaper/SPEC.md             |
      | agm/internal/modelrouter/SPEC.md         |
      | cmd/pretool-supervisor-guard/SPEC.md     |

  Scenario Outline: Legacy BDD linkage is provider-neutral
    Given strict legacy linkage runs through "<harness>" with "<family>"
    When AGM validates strict legacy linkage route parity
    Then every strict legacy specification should retain executable linkage

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
