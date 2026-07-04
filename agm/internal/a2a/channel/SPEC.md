# A2A Channel Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/channel` owns the markdown channel format used for
file-backed A2A coordination. It creates channel files, appends validated
protocol messages, parses existing messages, archives inactive channels, and
creates template-backed channels with optional Wayfinder metadata.

## EARS Requirements

**A2A-CHN-01** When a channel is created, the system shall create parent directories, reject duplicate channel files, and write a protocol header.

**A2A-CHN-02** When a message is appended to a channel, the system shall validate the A2A protocol message before writing formatted markdown.

**A2A-CHN-03** When reading channel messages, the system shall parse message headers, status values, numbered messages, context, proposal, questions, blockers, and next steps from the markdown format.

**A2A-CHN-04** When a manager lists channels, the system shall create the base directory if needed and report topic, path, modification time, and message count for markdown channel files.

**A2A-CHN-05** When a topic has multiple channel files, the system shall return the most recently modified matching channel.

**A2A-CHN-06** When a channel is archived, the system shall move it into a year-month archive directory without changing the active channel contents.

**A2A-CHN-07** When a template channel is created with a Wayfinder project path, the system shall verify the project path exists before writing the channel.

**A2A-CHN-08** When a template channel is created without an explicit agent ID or participants value, the system shall infer the agent identity and use it as the default participant set.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/wayfinder_parity.feature`
- Package tests: `agm/internal/a2a/channel/channel_test.go`
