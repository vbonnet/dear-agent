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
stderr), so it composes cleanly with any caller that wants to capture the token.
(This is *not* an invitation to wire it up as an `apiKeyHelper` — see
"Retired wiring" below.)

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-credentials` | `~/.claude/.credentials.json` | credentials file path |
| `-endpoint` | built-in / `$CLAUDE_OAUTH_TOKEN_ENDPOINT` | OAuth token endpoint |
| `-client-id` | built-in / `$CLAUDE_OAUTH_CLIENT_ID` | OAuth client ID |
| `-lock-timeout` | `10s` | max wait for the cross-process credentials lock |
| `-quiet` | `false` | suppress structured stderr logs |
| `-audit-log` | `~/.local/state/dear-agent/token-refresher-audit.jsonl` | JSONL audit (empty disables) |
| `-quarantine` | `~/.local/state/dear-agent/refresh-token-quarantine.json` | refresh-token quarantine marker (empty disables) |
| `-clear-quarantine` | `false` | clear the quarantine and exit (operator override) |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | success |
| `1` | generic / usage error |
| `2` | token family dead (`invalid_grant`) — re-authenticate (`claude /login` / `claude setup-token`) |
| `3` | refresh succeeded on the server but could not be persisted (critical — investigate disk/permissions) |
| `4` | refresh token quarantined — an earlier refresh may have spent it (see below) |

## Refresh-token quarantine

Refresh tokens here are single-use and rotating, so a refresh whose request
reached the server but whose response did not is genuinely ambiguous: the server
may have consumed the token and issued a replacement that never arrived. The
on-disk token then looks valid but is spent, and presenting it again is a replay
— which rotation treats as proof of theft, revoking the whole family and forcing
a `claude /login`.

That is exactly how the 2026-07-18 family death happened (ce-77ip.7): a
`Client.Timeout exceeded while awaiting headers` at 08:58:37Z, then the same
token presented again at 10:29:06Z.

So the refresher distinguishes the two network failure modes precisely, using
`httptrace` rather than error text:

- **Request never transmitted** (TLS handshake timeout, connection refused, DNS):
  the token is untouched. Ordinary retryable error.
- **Request transmitted, no usable response** (timeout awaiting headers, 5xx, an
  unreadable 200): the token may be spent. It is **quarantined** — recorded by
  fingerprint and never presented again automatically.

A quarantine clears itself as soon as the on-disk token changes, so if any client
rotates successfully, refreshing resumes with no intervention. To inspect or
override:

```sh
token-refresher -check              # reports an active quarantine
token-refresher -clear-quarantine   # override: re-arm automatic refresh
```

Holding back is the safer failure. If the server did rotate, replaying guarantees
family revocation and takes down every OAuth client on the host at once;
quarantining instead lets the current access token live out its expiry while the
operator is alerted. If the server never processed the request, the cost is one
stalled refresh cycle.

## Wiring options

- **launchd (idle backstop):** schedule `token-refresher -force` so the
  credentials file stays fresh — and the single-use refresh-token family stays
  alive — even when no session is running. This covers *idle* time. **This is
  the only sanctioned wiring.**
- **In-process:** callers already using `auth.ResolveOAuthToken()` get the same
  refresh for free.

### Retired wiring: `apiKeyHelper` — do not reintroduce

This binary was previously wired in as Claude Code's `apiKeyHelper` (ce-cs3v,
PR #560) and no longer is. **That wiring was removed from the host on
2026-07-10 and must not be restored.**

Since claude-code 2.1.205, a configured `apiKeyHelper` is treated as an external
API key that takes precedence over — and *shadows* — a healthy `claude.ai` OAuth
login, and the CLI refuses to fall back to that login. The result is that
`claude -p` fails with `Invalid API key · claude.ai connectors are disabled
because ANTHROPIC_API_KEY or another auth source is set` even when
`~/.claude/.credentials.json` is perfectly fresh. Upstream:
[#11587](https://github.com/anthropics/claude-code/issues/11587),
[#9694](https://github.com/anthropics/claude-code/issues/9694),
[#23568](https://github.com/anthropics/claude-code/issues/23568).

It also masked real errors: with the helper in front, a dead token family
surfaced as `apiKeyHelper failed: exited 2` and not as a legible auth failure,
which cost days of misdirected debugging (see the ce-77ip epic's "four-layer
auth failure" retro).

The stdout contract below is unchanged and still useful — it is what makes the
binary composable — but it must **never** be consumed via a retired
`apiKeyHelper` setting.

### Wire it into the mesh (ce-cs3v)

```
make install-token-refresher-launchagent
```

Stages [`deploy/launchd/com.dear-agent.token-refresher.plist`](../../deploy/launchd/com.dear-agent.token-refresher.plist)
(a 30-minute idle backstop) into `~/Library/LaunchAgents` and prints the single
ask-gated host step to run yourself:

```
launchctl load ~/Library/LaunchAgents/com.dear-agent.token-refresher.plist
```

The scheduled job routes stdout (the access token) to `/dev/null` and keeps
structured logs at `~/.local/state/dear-agent/token-refresher.err.log`.

## Build

```
make build-token-refresher && make install-token-refresher
```
