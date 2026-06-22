# AGM + Antigravity configuration

Repo-level configuration for the **antigravity** harness (the `agy` CLI, which
routes work to Google AI Ultra). AGM recognizes `antigravity` as a harness in
`agm/internal/manifest/v3.go` and `agm/internal/agent/validate.go`; this file
documents the harness-scoped rules and routing defaults.

```yaml
harness: antigravity
binary: ~/.local/bin/agy
max_tokens: auto
workspace: oss
```

## Auth

The `agy` binary manages its own auth against Google AI Ultra. AGM treats the
harness as available whenever `agy` is on `PATH`; otherwise it falls back to the
`GEMINI_API_KEY` environment variable. See <https://antigravity.google/>.

## Beads task tracking

This project uses **bd** (beads) for durable issue tracking — the same source of
truth used by every other harness in this repo. Run `bd prime` for full workflow
context. Lifecycle hooks that load and refresh Beads context for `agy` sessions
live in [`hooks.json`](./hooks.json) and mirror `.codex/hooks.json`.

### Rules

- Beads is the durable source of truth for project work; local scratch files are
  not. Do not create markdown TODO files when Beads is available.
- Use `bd update <id> --claim` to claim work atomically and `bd close <id>` to
  finish it. Do not auto-close tasks unless the work is actually complete.
- Do not use `bd edit` (interactive); use `bd update` flags instead.
- Prefer `--json` when parsing `bd` output programmatically.
