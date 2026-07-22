# Runbook: OAuth token family death (recurring manual `/login`)

Epic: `ce-77ip` — Eliminate recurring manual `/login` for the 24/7 mesh.
Related: `ce-de4v` (hot-reload for long-lived sessions), `ce-cknn`, `ce-rnpt`, `ce-cs3v`.

## Symptom

Every Claude Code client on the host starts failing to authenticate at once, and
`~/.local/state/dear-agent/token-refresher.err.log` fills with:

```
oauth refresh token rejected (invalid_grant): token family is dead, re-authentication required (status 400)
```

Only a human running `claude /login` clears it.

## Why it happens

Claude Code OAuth refresh tokens are **single-use and rotating**. Spending one
returns a replacement and invalidates the old one. If any client later presents
an already-spent token, the authorisation server reads that as a replay attack
and revokes the **entire token family** — every client dies at once, not just
the one that replayed.

The design that was supposed to prevent this: `token-refresher` performs the
whole read-check-exchange-write cycle under a cross-process file lock
(`~/.claude/.credentials.lock`), so no two refreshes overlap.

**That guarantee has been void since 2026-07-10.** The lock only binds callers
of `token-refresher` itself. It was wired in as Claude Code's `apiKeyHelper` so
that CLI sessions refreshed *through* it, but apiKeyHelper was removed because
Claude Code >= 2.1.205 treats a configured helper as an external API key that
shadows healthy OAuth and refuses to fall back
([anthropics/claude-code#11587](https://github.com/anthropics/claude-code/issues/11587)).

Since then the host runs roughly 15 independent, unlocked OAuth clients against
one `~/.claude/.credentials.json`:

| Client | Observed count | Notes |
| --- | --- | --- |
| Desktop-embedded claude-code runtime | ~9 | `Library/Application Support/Claude/claude-code/<ver>/` — often a *different version* from the CLI |
| Claude Code CLI sessions | ~2+ | `~/.local/share/claude/versions/<ver>` |
| agm sandbox spawns | varies | `agm session` / supervisor workers |
| `token-refresher` LaunchAgent | 1 | the only one that takes the lock |

Any of them can spend the refresh token. None of them coordinate.

## Do NOT "fix" this by re-adding apiKeyHelper

It shadows healthy OAuth on >= 2.1.205 and reintroduces a worse failure
(anthropics/claude-code#11587, #9694, #23568). Any broker-style fix must not
reintroduce that shadowing.

## Do NOT assume `-force` is the culprit

The 30-minute `-force` cadence rotates the token ~48x/day, which looks like an
obvious suspect. The evidence contradicts it: the audit log shows ~380 forced
rotations across 2026-07-10 → 2026-07-18 with **zero** deaths. Steady-state
rotation is not what kills the family, and `-force` may in fact be protective —
it keeps the access token so fresh that other clients rarely need to refresh at
all. Removing it without evidence would widen the window in which some other
client decides to refresh on its own.

Both observed deaths instead sit across a **machine sleep/wake boundary**, which
is consistent with a wake-time thundering herd: every client wakes with an
expired access token, they all read the same refresh token before any of them
writes back, one wins and the rest replay. This is a hypothesis, not a confirmed
cause — see "Identifying the culprit" below.

## Identifying the culprit

Every audit line in `~/.local/state/dear-agent/token-refresher-audit.jsonl` now
carries `refresh_token_fp` (a short SHA-256 prefix of the refresh token that was
on disk when the tick started) and `credentials_mtime`. Compare the lines around
a `token_family_dead` outcome:

- **Fingerprint CHANGED between our ticks while `refreshed` was false** — another
  client on this host rotated the token and wrote the new one back. That client
  is the rotator; we were about to present a stale token.
- **Fingerprint UNCHANGED right up to the death** — the token was spent by
  something that never wrote back to this file: a different credential store
  (e.g. the Keychain item `Claude Code-credentials`) or another machine on the
  same account.

Check it with:

```sh
grep -E 'token_family_dead|"refreshed":true' ~/.local/state/dear-agent/token-refresher-audit.jsonl | tail -20
```

## Recovery

```sh
claude /login

# The cadence job may have been throttled off launchd's schedule by the
# exit-2 storm (this is what -cadence prevents going forward). Re-arm it:
launchctl kickstart -k gui/$(id -u)/com.dear-agent.token-refresher
launchctl print gui/$(id -u)/com.dear-agent.token-refresher | grep -E 'runs|last exit'
```

## Why the outage used to be silent

Two compounding failures, both fixed by `-cadence`:

1. The death was written only to a log file nobody tails, so it was discovered
   by failing to authenticate hours later. `-cadence` raises a desktop
   notification on the first tick of each death episode.
2. launchd throttles a `StartInterval` job that exits non-zero quickly and
   repeatedly. After a death, the job exited 2 every 30 minutes until launchd
   stopped scheduling it entirely — observed on 2026-07-19, when it ran 26 times
   and then went silent for 17 hours. So even after the operator re-authenticated,
   nothing was keeping credentials fresh, which made the next death certain.
   `-cadence` reports success so the schedule survives.

## Open work

Restoring a genuine single-writer guarantee needs one of:

- a supported Claude Code setting that disables per-session refresh (read the
  shared credentials file, never rotate) — does not exist today;
- a non-shadowing credential broker;
- reducing the client count (e.g. not running Claude Desktop alongside the mesh,
  which alone removes ~9 of ~15 clients);
- keeping sessions shorter than the access-token TTL so none outlives its token.

Tracked in `ce-77ip`.
