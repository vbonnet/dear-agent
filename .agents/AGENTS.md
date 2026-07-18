@import ../AGENTS.md

# AGM + Antigravity configuration

Configuration for the `antigravity` harness driven by the `agy` CLI
(Google Antigravity, backed by Google AI Ultra). This spreads AI load
off the Anthropic and OpenAI harnesses onto Google's quota.

```yaml
harness: antigravity
binary: ~/.local/bin/agy
version: 1.0.10
max_tokens: auto
workspace: oss
```

## Harness notes

- `agy` is the Antigravity CLI. It supports non-interactive `--print` / `-p`
  runs, interactive `--prompt-interactive` / `-i` sessions, and conversation
  resume via `--continue` / `--conversation`.
- Model selection uses `--model`; list available models with `agy models`.
- Permission auto-approval (for unattended AGM workers) uses
  `--dangerously-skip-permissions`; pair with `--sandbox` when restricting
  terminal access.
- AGM registers `antigravity` as a known harness in
  `agm/internal/manifest/v3.go` (`knownHarnesses`).

## Beads integration

Beads lifecycle hooks for this harness live in `.agents/hooks.json`,
mirroring the `.codex/hooks.json` pattern. They keep durable project
task context through the Beads CLI in sync across the session lifecycle.
