Feature: Instruction entrypoint parity
  AGENTS.md is the canonical shared instruction file. Harness-specific
  entrypoints should import it first so Claude Code, Codex CLI, AGY, OpenCode,
  and Gemini compatibility sessions inherit the same repository rules.

  Scenario Outline: Instruction entrypoints import AGENTS.md first
    Given instruction entrypoint "<file>" is configured
    When AGM validates instruction entrypoint parity
    Then instruction entrypoint "<file>" should import "<import>"
    And instruction entrypoint "<file>" should not duplicate shared policy

    Examples:
      | file               | import               |
      | CLAUDE.md          | @import AGENTS.md    |
      | GEMINI.md          | @import AGENTS.md    |
      | CODEX.md           | @import AGENTS.md    |
      | AGY.md             | @import AGENTS.md    |
      | OPENCODE.md        | @import AGENTS.md    |
      | .claude/CLAUDE.md  | @import ../AGENTS.md |
      | .agents/AGENTS.md  | @import ../AGENTS.md |
      | .deepsec/AGENTS.md | @import ../AGENTS.md |
