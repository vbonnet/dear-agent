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

Run `token-refresher -help` for the current option inventory and defaults.
Refresh safety state is credential-scoped: the default Claude credentials use
the managed dear-agent state directory for their quarantine, while a
non-default credentials file uses `<credentials>.refresh-quarantine.json`.
Every credential set also has a durable `<credentials>.refresh-stop` marker
beside its canonical credentials path. That stop is honored by the CLI and
in-process `auth` callers alike until the operator remediates the persistence
failure and explicitly clears the quarantine.

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

So the refresher observes whether the transport consumed the refresh-token body
rather than parsing error text. This is deliberately conservative: consumption
does not prove bytes reached the server.

- **Request never transmitted** (TLS handshake timeout, connection refused, DNS):
  the token is untouched. Ordinary retryable error.
- **Request body consumed, no usable response** (timeout awaiting headers, 5xx, an
  unreadable 200): the token may be spent. It is **quarantined** — recorded by
  fingerprint and never presented again automatically.

The mechanism **fails closed** everywhere it can. Only "the marker file is not
there" counts as no quarantine; a marker that exists but cannot be read or parsed
blocks the refresh, because it may be naming the token on disk. And if a
possibly-spent token cannot be *recorded*, that is reported as a critical
non-persistence failure (exit 3) rather than logged and forgotten — the
protection lives in that file, not in the running process, so a failed write
means the next tick would replay the token.

If a server-successful refresh cannot persist the rotated credentials, the
refresher writes the credential-scoped quarantine before returning the critical
error. Every shared resolver entry point consults that quarantine, preventing
another process using `auth.ResolveOAuthToken()` from replaying the token.

A quarantine clears itself as soon as the on-disk token changes, so if any client
rotates successfully, refreshing resumes with no intervention. To inspect or
override:

```sh
# Reuse the exact selectors printed by the failing invocation:
token-refresher -credentials "/path/to/credentials.json" -quarantine "/path/to/quarantine.json" -check
token-refresher -credentials "/path/to/credentials.json" -quarantine "/path/to/quarantine.json" -clear-quarantine
```

For the default credential set and default quarantine path, the selectors may
be omitted. For any non-default credentials file or explicit quarantine path,
omitting them inspects or clears a different protection set.

Holding back is the safer failure. If the server did rotate, replaying guarantees
family revocation and takes down every OAuth client on the host at once;
quarantining instead lets the current access token live out its expiry while the
operator is alerted. If the server never processed the request, the cost is one
stalled refresh cycle.

## Wiring options

- **launchd (idle backstop):** schedule `token-refresher -cadence` so the
  credentials file is checked every 30 minutes and refreshed only when the
  access token is near expiry. Do not use `-force` in the scheduled job: Claude
  Code runtimes also refresh this shared file without taking dear-agent's lock,
  and forced 30-minute refresh-token rotations increase the chance that another
  client presents a rotated-away token. This covers *idle* time. **This is the
  only sanctioned wiring.**

  Pair `-cadence` with an `-expiry-skew` **larger than the scheduler's tick**
  (the shipped plist uses `45m` against a 1800s `StartInterval`). The default
  skew is 60s, so a 30-minute tick will almost always find the token "fresh"
  with minutes of life left, decline to refresh, and return after it has already
  expired. Observed on 2026-08-15: expiry at 09:21:24Z, a tick at 08:53:43Z that
  did nothing, and a tick at 09:23:43Z that refreshed 2m19s late — every request
  in between got a 401 "login expired".

  The wide skew is also the closest thing to a single-writer guarantee that is
  available today. Refreshing well ahead of expiry means no other OAuth client
  on the host ever observes a near-expiry token, so none of them has cause to
  rotate it. This is a *probabilistic* mitigation, not a lock — those clients
  still do not take `~/.claude/.credentials.lock`, and a real single-writer
  guarantee remains tracked in ce-77ip.
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
