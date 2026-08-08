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

## Hook integration

`.agents/hooks.json` uses Antigravity's named-hook schema. Its
`spec-contract-guard` Stop hook names the operator-installed,
digest-bound `/usr/local/libexec/dear-agent-spec-contract-hook` helper rather
than launching checkout code. The helper derives its sole Git worktree root
from Antigravity's absolute `workspacePaths` input; absent, invalid, or
multi-root input emits at most one native cooperative `decision: "continue"`
envelope for a stable conversation identity. A repeated failure for that
conversation, an input without a stable identity, or unavailable private retry
state emits `decision: "allow"` so a malformed envelope cannot create an
unbounded Stop loop.
Antigravity has no `SubagentStop` or Beads lifecycle hook events, and its
`PreToolUse` contract has no neutral decision that preserves the harness's
ordinary permission flow. This source therefore does not project unrelated
legacy guardrails onto unsupported or semantically lossy events. The repository
does not claim that this source configuration or helper is installed, loaded,
or provider-required; artifact installation and provider rollout remain
separate admission steps.
