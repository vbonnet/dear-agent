# ADR-017: Hook-assisted pending message delivery

Status: Accepted (2026-03-24; amended 2026-07-17)

## Context

The durable queue and daemon can delay delivery until their next readiness
check. Harness tool hooks provide a natural low-latency checkpoint, but only on
harnesses that support them.

## Decision

`agm/internal/messages` may write atomically named pending files under the AGM
state directory. A supported harness hook consumes them on its next tool call.
This path supplements the SQLite queue; it does not replace queue retention,
retry, acknowledgement, or cross-harness delivery.

Hook errors do not block the user's tool call. Unsupported harnesses use the
daemon path.

## Alternatives

One-second polling adds constant work. A socket requires a persistent listener.
Treating hook files as the queue loses queryable retry and acknowledgement
state.

## Consequences

Hook delivery is best-effort and may repeat after a crash. Pending-file and hook
tests verify atomicity and cleanup behavior.
