# Dear Agent API Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/dear-agent-api` serves the JSON HTTP API over a Tailscale tsnet listener
or an unauthenticated loopback development listener. It wires SQLite stores,
audit store configuration, caller identity, graceful shutdown, and workflow
runner spawning into the reusable `pkg/api` server.

## Requirements

**DEAR-AGENT-API-01** When command-line parsing fails, the system shall return exit code 2 and print flag usage diagnostics.

**DEAR-AGENT-API-02** When the runs database cannot be opened or pinged, the system shall return exit code 1.

**DEAR-AGENT-API-03** When the audit database flag is empty, the system shall disable audit endpoints without failing startup.

**DEAR-AGENT-API-04** When loopback mode is selected, the system shall use the anonymous `loopback` identity and listen on the requested address.

**DEAR-AGENT-API-05** When loopback mode is selected, the system shall print that it is listening without authentication.

**DEAR-AGENT-API-06** When Tailscale mode has no explicit state directory, the system shall derive the state directory from the user's home directory and hostname.

**DEAR-AGENT-API-07** When Tailscale mode starts, the system shall pass `TS_AUTHKEY` to the tsnet server auth key.

**DEAR-AGENT-API-08** When Tailscale WhoIs returns no user profile, the system shall reject caller identification.

**DEAR-AGENT-API-09** When Tailscale mode listens successfully, the system shall serve HTTPS on the tailnet listener with TLS 1.2 or newer.

**DEAR-AGENT-API-10** When the process context is cancelled, the system shall run bounded graceful HTTP shutdown before exiting.

**DEAR-AGENT-API-11** When WhoIs receives an IPv6 remote address with a zone identifier, the system shall strip the zone before calling the local client.

## BDD Traceability

- `agm/test/bdd/features/api_gateway_package_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
