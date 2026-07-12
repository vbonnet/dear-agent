# AGM Event Bus Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/eventbus` owns canonical AGM runtime event schemas and the WebSocket
hub used by adapters, monitoring, and stall reporting. The package is a parity
contract because event producers and consumers must use the same session and
stall event shapes regardless of harness or model family.

## EARS Requirements

**EVENTBUS-01** When an event is created, the system shall reject unknown event types.

**EVENTBUS-02** When an event is created, the system shall reject an empty session ID.

**EVENTBUS-03** When an event is created, the system shall marshal the typed payload into JSON and attach a non-zero timestamp.

**EVENTBUS-04** When an event is validated, the system shall reject invalid event types, empty session IDs, zero timestamps, and malformed payload JSON.

**EVENTBUS-05** When a payload is parsed, the system shall unmarshal the event payload into the caller-provided payload type.

**EVENTBUS-06** When clients subscribe over WebSocket, the system shall apply the requested session filter and deliver only matching session events unless the filter is `*`.

**EVENTBUS-07** When the maximum WebSocket client count is reached, the system shall reject additional clients with an error message and close the connection.

**EVENTBUS-08** When a client send buffer is full during broadcast, the system shall disconnect that client without blocking delivery to other clients.

**EVENTBUS-09** When the hub shuts down, the system shall close registered client channels and connections and clear the client registry.

**EVENTBUS-10** When stall metrics are emitted, the system shall translate stall detected, recovered, and escalated events into metrics without requiring a harness-specific event schema.

**EVENTBUS-11** When a caller uses nonblocking checked broadcast, the system shall report whether the event entered the hub queue instead of silently hiding backpressure.

**EVENTBUS-12** When subscription readiness is inspected, the system shall report the number of connected clients whose active filter matches the requested session.

## BDD Traceability

- `agm/test/bdd/features/harness_parity.feature`
- `agm/test/bdd/features/stall_detection.feature`

## Package Test Traceability

- `agm/internal/eventbus/schema_test.go`
- `agm/internal/eventbus/websocket_test.go`
- `agm/internal/eventbus/integration_test.go`
- `agm/internal/eventbus/stall_metrics_sink_test.go`
- `agm/internal/eventbus/ci_skip_test.go`
