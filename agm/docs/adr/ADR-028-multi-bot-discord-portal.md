# ADR-028: Multi-bot Discord portal

Status: Accepted (2026-06-09; renumbered 2026-07-17)

## Context

One Discord bot can prefix responses with agent names but cannot provide
mentionable, structurally distinct agent identities. Webhooks can vary display
names and avatars, but Discord does not expose them as mentionable users. This
record originally collided with the older session-archival ADR-026.

## Decision

`agm-bus` supports an opt-in `MultiBotDiscordAdapter`:

- one real Discord bot application per agent for mentionability and outbound
  identity;
- one gateway listener, with the other bot sessions used for REST posting;
- routing through the existing bus registry, ACL, queue, and pseudo-sessions;
- configuration in `~/.agm/discord-agents.yaml`, outside the repository;
- destructive channel reset only through an explicit confirmed subcommand.

The existing single-bot direct-message adapter remains a separate surface.

## Alternatives

Webhooks fail mentionability. A single bot with prefixes weakens attribution.
Opening a gateway connection for every bot adds duplicate inbound handling.

## Consequences

Operators manage multiple bot applications and tokens, and Discord message
chunking remains mandatory. The adapter, configuration parser, and command
wiring are tested under `agm/internal/bus` and `agm/cmd/agm-bus`.
