# Gateway CLI Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/gateway/adapters/cli` implements the stdin/stdout gateway adapter. It
reads line-delimited JSON commands, stamps default caller identity when needed,
dispatches through the gateway, and writes line-delimited JSON responses while
preserving input order.

## Requirements

**GATEWAY-CLI-01** When the CLI adapter reports its name, the system shall return `cli`.

**GATEWAY-CLI-02** When the CLI adapter reads an empty input line, the system shall skip the line.

**GATEWAY-CLI-03** When the CLI adapter reads malformed JSON, the system shall write a response with `CodeInvalidArgs` and continue the stream.

**GATEWAY-CLI-04** When an inbound command has no caller login, the system shall stamp the adapter's configured caller onto the command.

**GATEWAY-CLI-05** When an inbound command already has caller identity, the system shall preserve that caller identity.

**GATEWAY-CLI-06** When the input reader reaches EOF, the system shall return nil.

**GATEWAY-CLI-07** When the context is cancelled before processing the next line, the system shall return the context error.

**GATEWAY-CLI-08** When responses are written, the system shall serialize writes and encode one JSON response per line with HTML escaping disabled.

**GATEWAY-CLI-09** When OS user lookup fails for the default caller, the system shall return a synthetic `cli` caller.

## BDD Traceability

- `agm/test/bdd/features/api_gateway_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
