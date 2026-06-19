# token-refresher

Single-owner, file-locked refresher for the Claude Code OAuth credentials file
(`~/.claude/.credentials.json`). It keeps the VROOM supervisor mesh — three
Claude Code sessions in separate tmux panes sharing one credentials file —
alive across access-token expiry without a human re-running `/login`.

The refresh logic lives in [`pkg/llm/auth`](../../pkg/llm/auth/README.md); this
binary is the thin scheduler/CLI around it.

## Modes

```
token-refresher              # ensure fresh, print the access token to stdout
token-refresher -check       # report status only — no network, no mutation
token-refresher -force       # refresh even if the current token is still fresh
```

Default (print) mode emits **only** the access token on stdout (all logs go to
stderr), so it satisfies the Claude Code `apiKeyHelper` contract.

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-credentials` | `~/.claude/.credentials.json` | credentials file path |
| `-endpoint` | built-in / `$CLAUDE_OAUTH_TOKEN_ENDPOINT` | OAuth token endpoint |
| `-client-id` | built-in / `$CLAUDE_OAUTH_CLIENT_ID` | OAuth client ID |
| `-lock-timeout` | `10s` | max wait for the cross-process credentials lock |
| `-quiet` | `false` | suppress structured stderr logs |
| `-audit-log` | `~/.local/state/dear-agent/token-refresher-audit.jsonl` | JSONL audit (empty disables) |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | success |
| `1` | generic / usage error |
| `2` | token family dead (`invalid_grant`) — re-authenticate (`claude /login` / `claude setup-token`) |
| `3` | refresh succeeded on the server but could not be persisted (critical — investigate disk/permissions) |

## Wiring options

- **`apiKeyHelper` (preferred):** point Claude Code's `apiKeyHelper` setting at
  this binary; the CLI then drives the cadence (every
  `CLAUDE_CODE_API_KEY_HELPER_TTL_MS`, or on HTTP 401).
- **launchd / cron:** run `token-refresher -force` (or plain) on a 15-minute
  interval.
- **In-process:** callers already using `auth.ResolveOAuthToken()` get the same
  refresh for free.

## Build

```
make build-token-refresher && make install-token-refresher
```
