# ADR-0004: Skill permission pattern syntax

Status: Accepted (verified 2026-07-17)

## Context

Skill permissions are executable configuration. Colon-separated patterns look
plausible but do not match Claude Code command permission syntax, leaving a
skill blocked despite declaring the command.

## Decision

Generated and hand-written skill permissions use the governed command schema,
including space-separated command wildcards such as `Bash(agm session list *)`.
The skill generator owns emitted frontmatter, and permission lint tests reject
unsupported syntax.

## Consequences

Permission declarations remain harness-specific extensions; the skill body must
still describe a harness-neutral command path. Generator and skill-lint tests
own verification.
