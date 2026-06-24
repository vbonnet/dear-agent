# Spike: GitHub Webhooks for Push-Based PR Events (ce-b1tt)

**Status:** spike / recommendation
**Bead:** ce-b1tt · **Overlaps:** ce-4m85 (supervisor output filtering, PR #677)
**Date:** 2026-06-22

## Problem

Supervisors poll `gh pr list` on every `/loop` tick (~every 90s) to learn
whether any PR changed state — opened, merged, closed, or had CI flip. The
ce-4m85 spike measured this as the single largest avoidable token sink in the
mesh: reading the rich `gh pr list --json …,statusCheckRollup` payload (~18K
tokens) every tick, ~40 ticks/hour, three supervisors → **~720K tokens/hour
per supervisor**, almost all of it re-reading unchanged state.

ce-4m85 Option A (structured delta pre-filter, cached in
`~/.agm/vroom/state/`) cuts the *token* cost ~98% by emitting only deltas, but
the pre-pass still **polls** GitHub on a timer. Polling is wasted work
whenever nothing changed, and it bounds latency to the poll interval (a PR
that merges 1s after a tick isn't seen for ~90s).

GitHub webhooks invert this: GitHub pushes a PR event to us the instant it
happens. The supervisor reads a compact local event file instead of polling a
remote API. This spike is the third component of the ce-4m85 Option C hybrid:

1. Option A delta pre-filter for `gh pr list` (ships today, highest ROI)
2. Monitor on local FS (`trail.jsonl`, heartbeats) — truly push
3. **ce-b1tt webhooks** — fold in behind the same delta contract to make
   GitHub genuinely push-based (this doc)

## Goal

Replace the polled `gh pr list` PR-state source with a push source, **without
changing the supervisor-facing contract**. The supervisor still reads a
compact, delta-only line; only the producer changes (poll → push). This keeps
the migration drop-in: the Option A pre-pass and the webhook receiver both
write the same `pr-events.jsonl` shape.

## 1. Delivery options

GitHub can deliver a webhook only to a publicly reachable URL. The AGM host is
a laptop/dev box behind NAT, so we need either a tunnel/proxy or a public
endpoint. Three options, ranked for our use case:

### Option 1 — `gh webhook forward` (the `gh-webhook` CLI extension) — RECOMMENDED for local dev

> ⚠️ **Not a built-in.** As of this spike, `gh webhook` is **not** part of the
> core `gh` CLI (`gh webhook --help` → `unknown command "webhook"`). It is an
> **official GitHub extension** that must be installed first:
>
> ```bash
> gh extension install cli/gh-webhook
> ```

Once installed, `gh webhook forward` creates a webhook on the repo *and* opens
a forwarding channel to localhost in one command — no public URL, no manual
webhook config in the GitHub UI, secret handled for you:

```bash
gh webhook forward \
  --repo=vbonnet/dear-agent \
  --events=pull_request \
  --url=http://localhost:9876/webhook
```

- **Pros:** zero public exposure; auth uses the existing `gh` login; one
  command; auto-cleans the webhook when it exits; easiest to start/stop during
  development.
- **Cons:** foreground process tied to the `gh` session; not a persistent
  daemon (wrap in launchd with `RunAtLoad`/`KeepAlive` for persistence — see
  §4); depends on the extension being installed.
- **Best for:** local dev and the first iteration of this work.

### Option 2 — `smee.io` + `smee-client`

`smee.io` is a free public proxy: you register a channel URL as the GitHub
webhook Payload URL, and run `smee-client` locally to forward channel events
to localhost.

```bash
npm install -g smee-client      # `smee` is NOT installed on this host (verified)
smee --url https://smee.io/<channel> --target http://localhost:9876/webhook
```

- **Pros:** no `gh` extension; works with any GitHub webhook config; language-
  agnostic.
- **Cons:** routes our PR events through a **third-party public relay** (the
  channel URL is effectively a bearer secret — anyone with it sees deliveries);
  extra Node dependency; the webhook must be configured manually in the GitHub
  UI.
- **Best for:** environments where the `gh` extension is unavailable, or CI.

### Option 3 — self-hosted public HTTP endpoint

Expose the receiver directly (ngrok/Cloudflare Tunnel/reverse proxy on a host
with a public name) and point the GitHub webhook at it.

- **Pros:** no relay in the path; full control; persistent.
- **Cons:** requires a public ingress (TLS, DNS, firewall) the AGM laptop
  doesn't have; most operational overhead; widest attack surface.
- **Best for:** a future hosted deployment, not the laptop.

**Verified on this host:** `smee` not installed; `gh webhook` not present as a
core subcommand (extension required). So the only zero-setup-cost path today is
installing `cli/gh-webhook` and using `gh webhook forward`.

## 2. Receiver design (minimal Go skeleton)

`cmd/agm-webhook-receiver/main.go` — a tiny HTTP server that the forwarder/
proxy targets. Responsibilities:

- Listen on `:9876` (configurable via `--addr`).
- Validate the GitHub webhook secret (HMAC-SHA256 over the raw body, compared
  against the `X-Hub-Signature-256` header in constant time). Secret comes
  from `$AGM_WEBHOOK_SECRET`.
- Filter to `pull_request` events (the `X-GitHub-Event` header) and extract the
  fields the supervisor cares about.
- Append one compact JSON line per relevant event to
  `~/.agm/vroom/pr-events.jsonl`.

Record shape (mirrors the Option A delta line so the supervisor contract is
unchanged):

```json
{"ts":"2026-06-22T18:04:11Z","action":"closed","merged":true,"pr_number":660,"repo":"vbonnet/dear-agent","sha":"abc1234","title":"…"}
```

Relevant `pull_request` actions: `opened`, `closed` (with `merged` true/false
to distinguish merge from close), `synchronize` (new commits pushed),
`reopened`, `ready_for_review`, `review_requested`. The receiver records the
action verbatim and lets the supervisor decide which transitions matter — same
policy split the decision trail uses (`pkg/vroom/decisiontrail` records
whatever it's given; the supervisor defines "consequential").

The append uses the same append-only JSONL discipline as the vroom decision
trail: one self-contained object per line, `O_APPEND|O_CREATE|O_WRONLY`, a
write lock to serialise concurrent deliveries, so a crash loses at most the
trailing partial line.

This is **scaffolding only** for the spike — the skeleton compiles-by-intent
(syntactically valid Go) but is not wired into a build target or deployed.

## 3. Integration with the supervisor tick

The supervisor stops calling `gh pr list` for PR state. Instead, each tick:

1. Reads `~/.agm/vroom/pr-events.jsonl` from the byte offset it stored last
   tick (tail-since-offset, like `read-since` on the trail), so it only sees
   events that arrived since the previous tick.
2. Collapses them into a compact human line:
   `PR #660 merged, PR #661 CI failed, PR #663 opened`.
3. If the file grew by zero bytes → no-op tick, nothing to read (and, paired
   with Monitor on the file, the tick need not even wake).

**Token math** (consistent with ce-4m85's measurements):

| | Bytes/tick | ~Tokens/tick |
|---|---|---|
| `gh pr list --json …,statusCheckRollup` (40 PRs) | ~72,000 | ~18,000 |
| webhook delta line ("PR #660 merged, PR #661 CI failed") | ~50 | ~15 |

≈ **98%+ reduction** on the PR-state source, and on no-op ticks the cost is
zero rather than the full poll. This matches the Option A savings but removes
the poll: latency drops from "up to one tick interval" to "near-instant", and
the host does no work when GitHub is quiet.

**Migration is drop-in:** the receiver writes the *same* `pr-events.jsonl`
shape the Option A pre-pass would produce. Swap the producer (poll → push)
without touching the supervisor-facing reader. This is exactly the "swap the
polling pre-pass for webhook push without changing the supervisor-facing
contract" step from ce-4m85 Option C.

**Pairs with Monitor (ce-4m85 step 2):** point the existing local-FS Monitor at
`pr-events.jsonl` too. The receiver's append becomes an FS-change event that
wakes the supervisor — so even the *read* is push-driven, eliminating no-op
wakeups for GitHub state the same way it does for `trail.jsonl`.

## 4. Deployment

### launchd (persistent receiver)

Model the daemon on the existing `deploy/launchd/com.dear-agent.*.plist`
agents (e.g. `com.dear-agent.gopls-watchdog`). A new
`com.dear-agent.agm-webhook-receiver.plist` runs the receiver under
`KeepAlive` (a long-lived server, not an interval job):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.dear-agent.agm-webhook-receiver</string>
  <key>ProgramArguments</key>
  <array>
    <string>__HOME__/go/bin/agm-webhook-receiver</string>
    <string>--addr</string>
    <string>:9876</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>__HOME__/go/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    <key>HOME</key>
    <string>__HOME__</string>
    <!-- AGM_WEBHOOK_SECRET injected from the keychain at install time;
         do NOT hardcode the secret in the plist. -->
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>__HOME__/.local/state/dear-agent/agm-webhook-receiver.out.log</string>
  <key>StandardErrorPath</key>
  <string>__HOME__/.local/state/dear-agent/agm-webhook-receiver.err.log</string>
  <key>ProcessType</key>
  <string>Background</string>
</dict>
</plist>
```

A companion `gh webhook forward` (or `smee-client`) process bridges GitHub →
`localhost:9876`; on a NAT'd laptop, that forwarder is what makes the
`KeepAlive` receiver reachable, so it too belongs under launchd
(`com.dear-agent.agm-webhook-forward`) with the same `KeepAlive` policy.

### GitHub repo webhook (manual path, Option 2/3 only)

Settings → Webhooks → Add webhook:

- **Payload URL:** the smee channel (Option 2) or public endpoint (Option 3).
- **Content type:** `application/json`.
- **Secret:** the value in `$AGM_WEBHOOK_SECRET`.
- **Events:** "Let me select individual events" → **Pull requests** only.

With `gh webhook forward` (Option 1) this is automatic — the extension creates
and tears down the webhook for you; **do not** also add one by hand.

> **Spike scope:** this document does not register any real webhook. Wiring a
> live webhook is follow-up implementation work (see Next actions).

## 5. Recommendation

**Adopt `gh webhook forward` for local dev first, launchd for persistence.**

Sequence:

1. **Install `cli/gh-webhook` and use `gh webhook forward`** → `localhost:9876`
   receiver. Fastest path to a real push event; no public exposure; no relay;
   reuses the `gh` login. Validates the receiver and the `pr-events.jsonl`
   contract end-to-end.
2. **Promote to launchd** (`com.dear-agent.agm-webhook-receiver` +
   `com.dear-agent.agm-webhook-forward`, both `KeepAlive`) for a persistent,
   restart-surviving daemon.
3. **Keep the Option A delta pre-pass as the fallback** for when the forwarder
   is down — same `pr-events.jsonl` shape, so the supervisor never notices
   which producer is live. Push when available, poll as backstop.

`smee.io` (Option 2) only if the `gh` extension can't be installed; a
self-hosted public endpoint (Option 3) only for a future hosted deployment.

## Next actions

- **ce-b1tt.1** — Flesh out `cmd/agm-webhook-receiver`: real HMAC validation,
  `pull_request` extraction, append-only JSONL writer reusing the
  `pkg/vroom/decisiontrail` append discipline. (M)
- **ce-b1tt.2** — Reader helper: tail `pr-events.jsonl` since last offset, emit
  the compact delta line; share the contract with the ce-4m85 Option A pre-pass. (S)
- **ce-b1tt.3** — launchd plists for receiver + forwarder (`KeepAlive`), secret
  injected from keychain at install time. (S)
- **ce-b1tt.4** — Add `pr-events.jsonl` to the ce-4m85 Monitor watch so the
  supervisor wakes on push, not a timer. (S)

## References

- ce-4m85 — Spike: Supervisor /loop Output Filtering (PR #677,
  `docs/spike-supervisor-output-filtering.md`) — Option C hybrid this completes.
- `pkg/vroom/decisiontrail` — append-only JSONL discipline reused by the receiver.
- `deploy/launchd/com.dear-agent.gopls-watchdog.plist` — launchd template model.
- `cmd/babysit-prs` — current `gh pr list` consumer this would relieve.
