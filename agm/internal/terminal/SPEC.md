# AGM Terminal PTY Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/terminal` owns the PTY abstraction used by terminal-facing AGM
code. It wraps real PTY allocation while keeping a mock provider for tests.

## Requirements

**AGM-TERMINAL-01** When a real PTY starts a command, the system shall allocate the command through `pty.Start` and retain the returned PTY file.

**AGM-TERMINAL-02** When a real PTY is closed before it has started, the system shall return no error.

**AGM-TERMINAL-03** When a real PTY is read before it has started, the system shall return EOF.

**AGM-TERMINAL-04** When a real PTY is written before it has started, the system shall return EOF.

**AGM-TERMINAL-05** When the mock PTY starts a command without a configured start error, the system shall start the command normally for tests.

**AGM-TERMINAL-06** When the mock PTY writes data, the system shall append written bytes to its captured buffer unless a write error is configured.

**AGM-TERMINAL-07** When the mock PTY reads data, the system shall consume bytes from its configured read buffer unless a read error is configured.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
