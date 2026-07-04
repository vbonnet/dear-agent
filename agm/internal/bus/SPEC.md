# AGM Bus Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/bus` implements the long-lived local AGM bus daemon. It routes
newline-delimited JSON frames between live sessions, persists frames for
offline sessions, enforces optional ACL policy, and supports permission relay
frames for supervisor and external-channel workflows.

## EARS Requirements

**BUS-01** When a bus frame is validated, the system shall require the fields needed by that frame type and shall reject unknown frame types.

**BUS-02** When a frame is written, the system shall serialize one JSON object followed by a newline and shall set a timestamp if none is present.

**BUS-03** When a frame is read, the system shall skip blank lines, reject malformed JSON, and report truncated partial frames at EOF.

**BUS-04** When the bus server starts, the system shall bind the configured Unix socket, restrict socket permissions to the owner, and remove stale socket files.

**BUS-05** When a client connects, the system shall require a valid hello frame before registering the session.

**BUS-06** When a session registers twice without unregistering, the system shall reject the duplicate registration instead of silently replacing the existing delivery.

**BUS-07** When a target session is offline and a queue is configured, the system shall persist the frame for later replay instead of dropping it.

**BUS-08** When offline frames are drained, the system shall parse valid frames, truncate the queue file, and report parse errors without losing successfully parsed frames.

**BUS-09** When ACL policy is configured, the system shall allow self-sends, apply ordered allow rules, and default-deny unmatched cross-session sends unless default-allow is set.

**BUS-10** When ACL reload fails to parse a new policy, the system shall preserve the previous in-memory policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/bus/wire_test.go`
- Package tests: `agm/internal/bus/server_test.go`
- Package tests: `agm/internal/bus/queue_test.go`
- Package tests: `agm/internal/bus/acl_test.go`
