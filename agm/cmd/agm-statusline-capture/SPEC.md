# AGM Statusline Capture Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm-statusline-capture` legacy Claude Code status-line capture.

## Overview

`agm-statusline-capture` is the legacy capture command for Claude Code status
line JSON. It writes the raw session payload to AGM's status-line context
directory and always prints the default prompt shape so status-line failures do
not break the interactive terminal display.

## EARS Requirements

**STATUSCAP-01** When stdin is empty, the system shall treat the invocation as a no-op capture.

**STATUSCAP-02** When stdin contains invalid JSON, the system shall return a parse error from the capture path.

**STATUSCAP-03** When stdin lacks a non-empty session id, the system shall return a missing-session error from the capture path.

**STATUSCAP-04** When stdin contains valid session JSON, the system shall persist the full raw payload under the session id.

**STATUSCAP-05** When the context directory does not exist, the system shall create it with private directory permissions.

**STATUSCAP-06** When writing captured JSON, the system shall write to a temporary file and rename it into place.

**STATUSCAP-07** When the command main path encounters capture errors, the system shall still print the default prompt output.

**STATUSCAP-08** When the command prints the default prompt, the system shall include user, host, and current working directory segments.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/cmd/agm-statusline-capture`
