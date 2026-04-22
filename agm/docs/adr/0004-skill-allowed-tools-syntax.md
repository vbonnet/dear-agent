# ADR-0004: Skill allowed-tools Syntax Standard

## Status
Accepted

## Context
AGM skill files define `allowed-tools` in YAML frontmatter to auto-approve specific Bash commands. Some skills used colon-separated syntax (`Bash(agm session list:*)`), but Claude Code's permission system uses space-separated syntax (`Bash(agm session list *)`).

The mismatch caused permission prompts to appear even though commands should have been auto-approved, blocking session initialization.

Affected files: `agm-assoc.md`, `agm-list.md`, `agm-search.md`, `agm-status.md`, `agm-send.md`, `agm-new.md`, `agm-resume.md`

## Decision
Standardize on space-separated syntax for all `allowed-tools` patterns:
- Correct: `Bash(command *)`
- Incorrect: `Bash(command:*)`

Add a lint test (`allowed_tools_test.go`) to prevent future regressions.

## Consequences
**Positive**:
- Permission prompts no longer block skill execution
- Lint test prevents regressions
- Consistent syntax across all skills

**Negative**:
- None (colon syntax never worked correctly)
