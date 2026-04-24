# Skill Model/Effort Tiers

Every Claude Code skill (`agm/agm-plugin/commands/*.md`, `wayfinder/**/commands/*.md`,
and any `SKILL.md`) must declare `model:` and `effort:` in its YAML frontmatter.
The `skill-lint` tool and `pkg/skilllint` test enforce this in CI.

## Why

Skills run inside the parent Claude Code session. Without an explicit tier,
they inherit the parent's model — which is Sonnet by default but often Opus
in practice. A bulk refactor of docs running on Opus costs ~5× what it would
on Sonnet, silently. Pinning every skill to the cheapest tier that still
works prevents that drift.

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
| `sonnet` + `low`  | Light judgment over structured output                | `agm-exit` (delegates to `/bow`) |
| `sonnet` + `medium` | Multi-step reasoning, synthesis                     | `wiki-query-save`            |
| `sonnet` + `high` | Complex research / planning                          | (rare — prefer splitting)    |
| `opus` + any      | Reserved for unavoidable high-capability reasoning   | (avoid — document why)       |

## Lint

CI runs `go test ./pkg/skilllint/...` which walks the skill directories and
fails the build on any skill missing tier metadata. The same check is
available as a CLI: `go run ./tools/skill-lint agm/agm-plugin/commands`.
