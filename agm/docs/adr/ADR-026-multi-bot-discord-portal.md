# ADR-026: Multi-Bot Discord Portal with Per-Agent Identity

- **Status:** Proposed
- **Date:** 2026-05-16
- **Deciders:** Federation initiative (beads epic `ce-zx9`, `ce-zx9.2`)
- **Supersedes (partial):** the single-bot DM model in `agm/internal/bus/discord.go`
  is retained for HITL DMs; this ADR adds a parallel guild-channel portal.
- **Related:** ADR-006 (message-queue), ADR-009 (eventbus multi-agent),
  ADR-017 (gateway-platform-adapters — this is the first concrete chat Adapter),
  ADR-019 (a2a-agent-cards — future source of per-bot avatar/description).

## Context

The federation initiative needs a single Discord channel where the user can
`@mention` distinct agents — `@Claude`, `@Codex`, `@HermesAgent`, `@BrainV2` —
and see, per message, which agent produced which reply (attribution-aware).

The existing `DiscordAdapter` (`agm/internal/bus/discord.go`) is **single-bot,
DM-only**: one bot token, `IntentsDirectMessages` only, guild messages dropped
(`m.GuildID != ""` returns early), outbound attribution is a `[from]` text
prefix posted as the one bot. It cannot satisfy the requirement.

Discord API constraint that drives the decision: **a webhook is not a user**
— it has no user ID, cannot be `@mentioned`, and does not appear in mention
autocomplete. Webhooks give per-message `username`/`avatar` flexibility but
**lose mentionability**. The requirement is explicitly to `@mention` each
agent, so the inbound side must use real bot users.

## Decision

Add a **new, parallel** `MultiBotDiscordAdapter` (new file
`agm/internal/bus/discord_multibot.go`); do **not** modify the DM adapter.

1. **N separate Discord bot applications** (one per agent). Each is a real
   guild member → each is truly `@mentionable`, with autocomplete and
   notifications. Outbound replies are posted **by that agent's own bot**
   (its name, avatar, role colour) → attribution is *structural*, not a text
   prefix. Replies use Discord message references (native "replying to" UI)
   for extra attribution.
2. **One inbound listener gateway connection.** Only one bot session opens the
   gateway (`IntentGuildMessages | IntentMessageContent`). It sees `m.Mentions`
   for *all* bots, so a single listener routes every mention — no N× dedupe.
   The other agents' sessions are used **REST-only** for posting (discordgo
   REST needs only the token, not an open gateway), minimising gateway
   connections.
3. **Routed through the existing agm-bus broker.** Reuse `Registry`, `ACL`,
   `Queue`, and the `Frame.Extra` map. Each agent gets a pseudo-session
   `discord:agent:<name>`; inbound mentions become `FrameDeliver` to the
   agent's bus session; the agent replies via the agm-bus MCP `send` tool
   targeting `discord:agent:<name>`, and the adapter posts that reply as the
   agent's bot. Identity + correlation ride in `Frame.Extra`
   (`agent`, `discord_channel`, `discord_msg_id`, `discord_guild`).
4. **Channel reset is a separate, flag-gated maintenance subcommand**
   (`agm-bus discord-reset --channel <id> --confirm`), never run automatically
   on `serve`, scoped to one channel (no guild-wide deletion).
5. **Secrets out of the repo.** Per-agent tokens live in
   `~/.agm/discord-agents.yaml` (chmod 600, gitignored), referenced by flag —
   same pattern as the existing `DISCORD_BOT_TOKEN`.

## Alternatives considered

| Option | Distinct identity | `@mention`-able | Verdict |
|--------|-------------------|-----------------|---------|
| Webhooks (1 app, per-msg username/avatar) | Yes | **No** (not a user) | Rejected — fails the explicit @mention requirement |
| Single bot + text/embed prefix | Weak | No | Rejected — weakest attribution |
| **N bot apps + bus routing (chosen)** | **Yes (own bot user)** | **Yes** | **Chosen** — only option that is both attributed and mentionable |
| Official Claude `discord` plugin | single-bot | n/a | Rejected — single-bot, needs Bun (absent), bypasses the bus (dogfooding) |

## Consequences

**Positive:** true per-agent `@mention` + structural attribution; reuses all
existing broker machinery (no parallel state store); local now, distributed
later unchanged (broker is already cross-process); DM adapter untouched.

**Negative / costs:** N Discord apps must be created and `MESSAGE_CONTENT`
(privileged intent) enabled per app in the Developer Portal — a manual
one-time setup (documented in the runbook). N tokens to manage. Agent outputs
routinely exceed Discord's 2000-char limit → chunking is required, not
optional.

**Risks / mitigations:** (a) one listener is a single point of inbound failure
— acceptable for v1; revisit if it flaps. (b) Reply correlation depends on
threading `discord_msg_id` through `Frame.Extra` → MCP → the agent's `send`;
if the agent doesn't echo it, fall back to a non-threaded but still
bot-attributed post. (c) `discord-reset` is destructive — kept opt-in,
out of the hot path, requires `--confirm`.

## Implementation

`agm/internal/bus/discord_multibot.go` + `_test.go` (mock `guildBotClient`,
no live Discord); `agm-bus serve -discord-multibot -discord-agents <yaml>
-discord-channel <id> [-discord-guild <id>]`; `agm-bus discord-reset`. A
sibling `guildBotClient` interface keeps the DM adapter's `discordClient`
and its mock untouched.
