# A2A Discovery Checker Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/discovery` polls active A2A markdown channels, tracks the last
seen message per channel, and emits notifications when a channel has a new
message awaiting response.

## EARS Requirements

**A2A-DIS-01** When checker state is missing, unreadable, or malformed, the system shall initialize an empty state map instead of failing discovery.

**A2A-DIS-02** When checker state is saved, the system shall create the state directory and write indented JSON with `0600` permissions.

**A2A-DIS-03** When parsing a channel, the system shall extract the latest message header and a bounded proposal preview from the markdown channel format.

**A2A-DIS-04** When a channel's latest message has already been seen, the system shall not emit a duplicate notification.

**A2A-DIS-05** When a channel's latest status is not `awaiting-response`, the system shall update state without notifying the agent.

**A2A-DIS-06** When all channels are checked in dry-run mode, the system shall return notifications without writing updated state.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/discovery/checker_test.go`
