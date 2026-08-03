# AGM Statusline Command Specification

<!-- Last audited at: 2026-08-01 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm-statusline` Claude Code status line compositor.

## Overview

`agm-statusline` captures Claude Code status-line JSON, persists the raw session
payload for AGM lookup, and composes optional provider script output into a
single terminal status line. Providers are isolated by timeout and failure
handling so the editor status line remains responsive even when an optional
provider is missing, slow, or broken.

## EARS Requirements

**STATUSLINE-01** When stdin is empty, the system shall produce no persisted session payload.

**STATUSLINE-02** When stdin contains valid session JSON with a session id, the system shall persist the raw JSON under the session id.

**STATUSLINE-03** When persisting session JSON, the system shall write with private permissions and atomically rename the temporary file into place.

**STATUSLINE-04** When persisted provider output is still fresh, the system shall return the cached status line without rerunning providers.

**STATUSLINE-05** When provider output is stale or absent, the system shall discover executable providers from the configured provider directory.

**STATUSLINE-06** When a provider is disabled by full name or stripped name, the system shall skip that provider.

**STATUSLINE-07** When providers are discovered, the system shall execute them in lexical order and compose non-empty successful output with the configured separator.

**STATUSLINE-08** When a provider exits nonzero, times out, or emits empty output, the system shall omit that provider's segment.

**STATUSLINE-09** When provider scripts run, the system shall pass the raw session JSON on stdin.

**STATUSLINE-10** When provider scripts run, the system shall expose session name, session id, and workspace through AGM statusline environment variables.

**STATUSLINE-11** When configuration is absent or incomplete, the system shall fall back to the default separator and timeout settings.

**STATUSLINE-12** When composed output is empty for a session, the system shall cache the empty output to avoid repeated provider execution during the cache TTL.

**STATUSLINE-13** When a provider deadline expires, the system shall prioritize that deadline over ready completion signals and return after bounded cancellation cleanup even if a descendant retains inherited I/O.

**STATUSLINE-14** When a provider exits successfully before its deadline, the system shall accept its output until inherited stdout reaches EOF or the configured deadline expires.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/cmd/agm-statusline`
