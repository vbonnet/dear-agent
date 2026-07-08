# Engram Hook Analyzer Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/hooks-bin/internal/analyzer` parses hook denial logs, correlates denied
Bash tool uses with Claude transcript entries, classifies what the agent did
after a denial, and reports whether a denial likely caused wasted retries or a
productive tool switch.

## EARS Requirements

**EHA-01** When denial logs are parsed, the system shall extract denial entries with timestamp, pattern, command, tool-use ID, and transcript path fields.

**EHA-02** When denial classification receives a missing or unreadable transcript, the system shall classify the outcome as transcript missing.

**EHA-03** When the denied tool-use ID cannot be found in the transcript, the system shall classify the outcome as unknown.

**EHA-04** When no subsequent tool action is found within the look-ahead window, the system shall classify the outcome as gave up with low confidence.

**EHA-05** When the next action is a successful Bash retry, the system shall classify the outcome as retry success and mark it as a likely false positive.

**EHA-06** When the next action uses an alternative non-Bash tool, the system shall classify the outcome as switched tool and shall not mark it as a false positive.

**EHA-07** When the next action is another denied Bash command, the system shall classify the outcome as retry denied and count wasted Bash retries.

**EHA-08** When transcript content cannot be decoded as structured content blocks, the system shall ignore that entry rather than failing classification.

**EHA-09** When reports are generated, the system shall summarize denial counts, classifications, false-positive rates, and remediation patterns.

**EHA-10** When transcript data is cached, the system shall reuse cached transcript entries for repeated classifications of the same path.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/analyzer/*_test.go`

