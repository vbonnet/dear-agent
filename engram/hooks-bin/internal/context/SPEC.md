# Hook Session Context Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/context` persists bounded operational context for
hook adapters, including workers, Beads, active tasks, and recent notes.

## EARS Requirements

**EHSC-01** When no home directory is available, the Claude compatibility adapter shall place its default context under `/tmp`.

**EHSC-02** When a context file is missing, unreadable, or malformed, the hook shall return an empty non-nil context so advisory hooks remain non-blocking.

**EHSC-03** When context is saved, the system shall update its timestamp, create private parent directories, and write a private JSON file.

**EHSC-04** When workers are added or removed, the context shall preserve their task metadata and mark matching workers completed.

**EHSC-05** When more than 20 notes are added, the context shall discard the oldest notes and retain the newest 20.

**EHSC-06** When old contexts are pruned, the system shall consider only JSON files and shall leave directories and unrelated files untouched.

**EHSC-07** When a summary is rendered, the system shall include only populated operational sections and active workers.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/context/*_test.go`
