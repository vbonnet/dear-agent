# Skill Model/Effort Tiers

<!-- Last audited at: 2026-08-11 -->

Every provider command (`agm/agm-plugin/commands/*.md` and
`wayfinder/**/commands/*.md`) must declare `model:` and `effort:` in its YAML
frontmatter. Portable `SKILL.md` files may omit both fields so the active
harness can select its own tier; if either field is present, both are required.
The `skill-lint` tool and `pkg/skilllint` tests enforce this contract in CI.

## Why

Provider commands run inside the parent Claude Code session. Without an
explicit tier, they inherit the parent's model and can silently spend more than
their task warrants. Pinning commands to the cheapest sufficient tier prevents
that drift. Portable skills serve multiple harnesses, so provider-neutral
metadata is preferable unless a compatible model/effort pair is intentional.

## Allowed values

```yaml
model:  haiku | sonnet | opus
effort: low   | medium | high
```

Opus is allowed because some skills genuinely need it, but it should be rare.
The default sweet spot is `sonnet` + `low`; use `haiku` + `low` for pure
mechanical skills (string formatting, data extraction, simple CLI wrappers).

## Tier guide

| Tier              | When to use                                          | Example skills              |
|-------------------|------------------------------------------------------|------------------------------|
| `haiku` + `low`   | Mechanical wrapper around a deterministic command    | `agm-list`, `agm-status`, `agm-assoc`, `agm-new` |
| `sonnet` + `low`  | Light judgment over structured output                | `agm-exit`                   |
| `sonnet` + `medium` | Multi-step reasoning, synthesis                     | `wiki-query-save`            |
| `sonnet` + `high` | Complex research / planning                          | (rare — prefer splitting)    |
| `opus` + any      | Reserved for unavoidable high-capability reasoning   | (avoid — document why)       |

## Lint

CI runs `go test ./pkg/skilllint/...` and the repository-mode linter. It rejects
provider commands missing tier metadata and portable skills with incomplete or
invalid optional tier pairs. The same check is available as a CLI:
`go run ./tools/skill-lint -repo .`.
