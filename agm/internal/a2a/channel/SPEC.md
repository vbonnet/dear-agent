# A2A Channel Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/a2a/channel` owns the retained markdown channel format used for
file-backed A2A coordination. It creates individual channel files, appends
validated protocol messages, and parses messages from existing channels.

## EARS Requirements

**A2A-CHN-01** When a channel is created, the system shall create parent directories, reject duplicate channel files, and write a protocol header.

**A2A-CHN-02** When a message is appended to a channel, the system shall validate the A2A protocol message before writing formatted markdown.

**A2A-CHN-03** When reading channel messages, the system shall parse message headers, status values, numbered messages, context, proposal, questions, blockers, and next steps from the markdown format.

**A2A-CHN-04** When a channel path ends in a date suffix, the system shall derive its topic without the date or Markdown extension.

**A2A-CHN-05** When a channel cannot be read or opened for append, the system shall return a diagnostic that identifies the failed channel operation.

**A2A-CHN-06** When a parsed message has an invalid status, the system shall exclude that malformed message from the returned message set.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/wayfinder_parity.feature`
- Package tests: `agm/internal/a2a/channel/channel_test.go`
