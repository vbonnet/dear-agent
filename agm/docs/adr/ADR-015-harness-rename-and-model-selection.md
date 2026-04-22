# ADR-015: Rename --agent to --harness and Add --model Flag

## Status
Accepted

## Date
2026-03-22

## Context

AGM's `--agent` flag was misleading. It selects the CLI harness tool (Claude Code, Gemini CLI, Codex CLI, OpenCode CLI), not an AI agent. Users confused "agent" (the flag) with "agent" (the Go interface and AI concept). Additionally, there was no way to specify which model to use within a harness.

## Decision

### 1. Rename --agent to --harness

Renamed the CLI flag, all internal types, DB columns, manifest fields, and user-facing text from "agent" to "harness".

New harness IDs:
- `claude-code` (was `claude`)
- `gemini-cli` (was `gemini`)
- `codex-cli` (was `codex`/`gpt`/`openai`)
- `opencode-cli` (was `opencode`)

### 2. Add --model flag

Added per-harness model selection with:
- Model registry in `internal/agent/models.go`
- Per-harness defaults: claude-code→sonnet, gemini-cli→2.5-flash, codex-cli→5.4
- opencode-cli has no default (interactive picker required)
- Unknown models warn but pass through (forward compatibility)
- Tab completion via Cobra ValidArgsFunction

### 3. Dolt migration

Migration 009 renames the `agent` column to `harness` in agm_sessions and agm_messages tables.

### 4. No backward compatibility

This is a prototype. Old `--agent` flag and old harness IDs are removed entirely. Local sessions are wiped via schema migration.

## Consequences

### Positive
- Clear terminology: "harness" = CLI tool, "agent" = Go interface/AI concept
- Model selection gives users control over cost/quality tradeoffs
- Forward-compatible with new models (warn-but-allow policy)
- Tab completion improves discoverability

### Negative
- Breaking change for any scripts using `--agent`
- 107 files changed increases merge conflict risk
- Documentation update needed across 28+ docs files

### Risks
- Package name `internal/agent/` was NOT renamed (too disruptive) — creates terminology mismatch between package and concept
- Model registry becomes stale as providers release new models (mitigated by passthrough policy)

## Alternatives Considered

1. **Keep --agent, add --model only**: Rejected — the terminology confusion was the primary driver
2. **Rename package agent/ to harness/**: Rejected — too many import path changes, Go convention is short package names
3. **Backward-compatible alias (--agent as hidden deprecated flag)**: Rejected — prototype, clean break preferred
4. **Web-based model lookups**: Rejected — adds network dependency, latency, failure modes. Warn-but-allow is simpler.

## References

- Plan: `~/.claude/plans/transient-cooking-ember.md`
- Swarm: `engram-research/swarm/projects/agm-harness-rename/`
- Model registry: `internal/agent/models.go`
- Migration: `internal/dolt/migrations/009_rename_agent_to_harness.sql`
