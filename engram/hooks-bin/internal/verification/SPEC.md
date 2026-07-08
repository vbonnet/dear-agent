# Engram Hook Verification State Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/hooks-bin/internal/verification` tracks unaddressed verification
findings and escalates them after repeated tool use. The state file is a
session-local reminder mechanism for hook findings such as bead-close checks
and notification follow-ups.

## EARS Requirements

**EHG-01** When the default state path is requested and `HOME` is set, the system shall store verification state under `$HOME/.claude/verification-state.json`.

**EHG-02** When the default state path is requested and `HOME` is unset, the system shall fall back to `/tmp/.claude/verification-state.json`.

**EHG-03** When state is loaded from a missing, unreadable, or malformed file, the system shall return an empty state.

**EHG-04** When state is saved, the system shall create parent directories with private permissions and write indented JSON with private file permissions.

**EHG-05** When a pending verification is added, the system shall stamp the creation time, reset the tool-use counter to zero, and append it to state.

**EHG-06** When tool use is recorded, the system shall increment the tool-use counter for every pending verification.

**EHG-07** When pending verifications are removed by type and ID, the system shall retain only non-matching verifications.

**EHG-08** When pending bead-close verifications are removed by swarm label, the system shall retain pending entries of other types and other swarms.

**EHG-09** When pending verifications are older than the configured maximum age, the system shall prune them from state.

**EHG-10** When a pending verification reaches the escalation threshold, the system shall return an escalation result with a formatted operator message.

**EHG-11** When escalation results are written, the system shall write only escalated messages and return the count written.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/verification/*_test.go`

