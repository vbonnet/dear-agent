# AGM Compaction Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/compaction` generates identity-preserving compact prompts,
records compaction history, and prevents repeated compaction loops. It also
loads session-specific AGM state to build preserve prompts without borrowing
identity from another session.

## Requirements

**AGM-COMPACTION-01** When a session state file is loaded, the system shall read only `<session>-state.json` from the configured base directory.

**AGM-COMPACTION-02** When no matching session state file exists, the system shall return an error naming the session and attempted path instead of falling back to another state file.

**AGM-COMPACTION-03** When a preserve prompt is generated, the system shall use the target session name as identity and fall back to `worker` only when the target name is empty.

**AGM-COMPACTION-04** When state includes scan-loop interval, policy rules, managed sessions, completed count, queued items, or focus text, the system shall include those values in the preserve prompt.

**AGM-COMPACTION-05** When a generic compaction prompt is generated without any metadata, the system shall return `/compact`.

**AGM-COMPACTION-06** When a generic compaction prompt includes session metadata, the system shall format the metadata as structured preservation bullets.

**AGM-COMPACTION-07** When prompt audit files are saved, the system shall create `compaction-prompts` with owner-only directory permissions and write prompt files with owner-only file permissions.

**AGM-COMPACTION-08** When preflight runs while a session is already compacting, working, waiting for user input, or offline, the system shall mark compaction unsafe and return an explanatory error.

**AGM-COMPACTION-09** When anti-loop checks are not forced, the system shall block compaction during the cooldown window or after the maximum compactions in the rolling window.

**AGM-COMPACTION-10** When anti-loop checks are forced, the system shall allow compaction and preflight shall report a force-bypass warning.

**AGM-COMPACTION-11** When a compaction is recorded, the system shall update last compaction time, increment count, and append a history record with prompt file and force flag.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
