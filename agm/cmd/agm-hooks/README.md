# agm-hooks

Hook binaries that integrate Claude Code lifecycle events with the AGM session state machine.

## Overview

Claude Code fires hook events at key points in a session's lifecycle (`UserPromptSubmit`,
`PostToolUse`, `Stop`, `SessionEnd`). The `agm-hooks` package provides a small binary for each
event. Each binary does one thing: emit the correct session state (and optionally collect context
metrics) via the `agm` CLI.

The binaries are intentionally minimal and single-responsibility so they are fast, auditable, and
easy to extend.

---

## Hook Binaries

| Binary                          | Hook Event          | State Emitted          | Source Label               |
|---------------------------------|---------------------|------------------------|----------------------------|
| `stop-state-reporter`           | `Stop`              | `READY`                | `stop-hook`                |
| `userpromptsubmit-state-reporter` | `UserPromptSubmit` | `THINKING`                | `userpromptsubmit-hook`    |
| `sessionend-state-reporter`     | `SessionEnd`        | `READY`                | `sessionend-hook`          |
| `posttool-context-monitor`      | `PostToolUse`       | `THINKING` + context update| `posttool-hook`            |
| `posttool-worktree-tracker`     | `PostToolUse`       | *(no state change)*    | `posttool-hook`            |

`posttool-context-monitor` additionally reads the context-window percentage from the Claude Code
environment and writes it to the status-line file, subject to a 5 s debounce.

`posttool-worktree-tracker` watches Bash tool calls for `git worktree add` and `git worktree remove`
commands. When detected, it records the worktree path and lifecycle event in the `agm_worktrees` Dolt
table. The hook fails open (exit 0) when Dolt is unavailable, so it never blocks the agent.

---

## Installation

```sh
go install ./cmd/agm-hooks/...
```

This installs all four binaries into `$GOPATH/bin` (or `$GOBIN`). Ensure that directory is on
your `PATH` before configuring Claude Code.

---

## Configuration

Add each hook to `~/.claude/settings.json` under the `hooks` key. Claude Code merges hook lists
per event, so you can add these alongside any existing hooks.

```jsonc
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "stop-state-reporter" }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "userpromptsubmit-state-reporter" }
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "sessionend-state-reporter" }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "posttool-context-monitor" }
        ]
      }
    ]
  }
}
```

You also need a `statusLine` section so the binaries know where to write state:

```jsonc
{
  "statusLine": {
    "command": "agm-statusline-capture"
  }
}
```

---

## State Vocabulary Note

The AGM state machine has two vocabularies in active use:

| New CLI vocabulary  | Old manifest constant | Used by                          |
|---------------------|-----------------------|----------------------------------|
| `READY`             | `DONE`                | `stop-state-reporter`, `sessionend-state-reporter` |
| `THINKING`              | `WORKING`             | `userpromptsubmit-state-reporter`, `posttool-context-monitor` |
| `PERMISSION_PROMPT` | `USER_PROMPT`         | *(permission detection path)*    |

All binaries in this package emit the **new** vocabulary (`READY`/`THINKING`). The display layer
handles both sets for backward compatibility, so mixed environments (e.g. an older state file plus
new hooks) work correctly without manual intervention.

---

## Testing

```sh
go test ./cmd/agm-hooks/...
```

Unit tests cover state-emission logic and debounce behaviour. Integration behaviour (actual hook
invocation by Claude Code) is covered by the contract tests in `.github/workflows/contract-tests.yml`.

---

## Adding New Hooks

Follow the `stop-state-reporter` pattern:

1. Create a new directory under `cmd/agm-hooks/` named after the binary.
2. Write a `main.go` that:
   - Reads any required Claude Code environment variables.
   - Calls `agm session state set <SESSION> <STATE>` (or the equivalent Go helper).
   - Exits 0 on success, non-zero on error (Claude Code surfaces non-zero exits as warnings).
3. Add the binary to the installation command and the configuration snippet above.
4. Keep it single-responsibility: one hook event → one binary.

Avoid putting business logic in hooks. If logic is needed, put it in a shared package and call
it from the binary; keep the `main.go` as thin as possible.
