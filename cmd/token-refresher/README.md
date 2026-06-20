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

- **`apiKeyHelper` (on-demand):** point Claude Code's `apiKeyHelper` setting at
  this binary; the CLI then drives the cadence (every
  `CLAUDE_CODE_API_KEY_HELPER_TTL_MS`, or on HTTP 401). This covers *active*
  sessions.
- **launchd (idle backstop):** schedule `token-refresher -force` so the
  credentials file stays fresh — and the single-use refresh-token family stays
  alive — even when no session is running. This covers *idle* time.
- **In-process:** callers already using `auth.ResolveOAuthToken()` get the same
  refresh for free.

Both the `apiKeyHelper` and the launchd job run the same binary and share its
cross-process credentials lock, so they never race or double-spend the refresh
token (ce-rnpt / ce-f3e3).

### Wire it into the mesh (ce-cs3v)

```
make install-token-refresher-launchagent
```

Stages [`deploy/launchd/com.dear-agent.token-refresher.plist`](../../deploy/launchd/com.dear-agent.token-refresher.plist)
(a 30-minute idle backstop) into `~/Library/LaunchAgents` and prints the two
ask-gated host steps to run yourself:

```
# 1. schedule the idle backstop
launchctl load ~/Library/LaunchAgents/com.dear-agent.token-refresher.plist

# 2. point Claude Code's apiKeyHelper at the refresher (on-demand refresh)
configure-claude-settings set apiKeyHelper '"$HOME/go/bin/token-refresher"'
```

The scheduled job routes stdout (the access token) to `/dev/null` and keeps
structured logs at `~/.local/state/dear-agent/token-refresher.err.log`.

## Build

```
make build-token-refresher && make install-token-refresher
```
