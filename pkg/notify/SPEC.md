# Notification Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**NOTIFY-01** When notification configuration is loaded, the system shall parse enabled dispatcher definitions and reject unsupported dispatcher types.

**NOTIFY-02** When an event is converted to a notification, the system shall preserve title, body, severity, source, timestamp, and structured data.

**NOTIFY-03** When a notification sink dispatches, the system shall send the notification to every configured dispatcher and report dispatch failures.

**NOTIFY-04** When a webhook dispatcher sends a notification, the system shall use bounded HTTP behavior and reject non-success responses.

**NOTIFY-05** When a tmux dispatcher sends a notification, the system shall target the configured session rather than a fixed harness session.

**NOTIFY-06** When a platform desktop dispatcher is unavailable, the system shall return an explicit unsupported-platform error.

**NOTIFY-07** When a notification sink closes, the system shall close every dispatcher and preserve the first close error.

**NOTIFY-08** While notifications originate from any supported harness and model family, the system shall preserve the same event and dispatcher contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/notify`
