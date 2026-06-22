# AGM + Antigravity configuration

Repo-scoped rules for the `antigravity` AGM harness (Google AI Ultra via the
`agy` CLI). This spreads AI load off the Claude/Gemini/Codex harnesses.

```yaml
harness: antigravity
binary: ~/.local/bin/agy   # agy v1.0.10
max_tokens: auto
workspace: oss
```

## Status

`antigravity` is registered as a known harness in
`agm/internal/manifest/v3.go` and recognized by the post-create dispatch in
`agm/cmd/agm/new_postcreate.go`. Post-create prompt-readiness wait and prompt
delivery are still a stub — see the PR description for the required follow-up
wiring (resume support, permission-mode support, MCP-server harness hints).

## Beads lifecycle hooks

`hooks.json` mirrors `.codex/hooks.json`, registering Beads context hooks
(`SessionStart`, `PreCompact`, `PostCompact`, `UserPromptSubmit`) for the
antigravity harness.
