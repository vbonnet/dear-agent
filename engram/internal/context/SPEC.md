# Engram Context Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/context` derives a stable default retrieval query from the
current repository or working directory.

## EARS Requirements

**ECD-01** When the current directory has a configured Git origin, the system shall derive context from the repository name before considering the directory name.

**ECD-02** When the Git origin uses SSH or HTTPS syntax, the system shall remove any trailing `.git` suffix from the repository name.

**ECD-03** When no usable Git repository name is available, the system shall derive context from the current directory name.

**ECD-04** When neither repository nor directory context is usable, the system shall return the generic coding-best-practices fallback.

**ECD-05** When deriving a context name, the system shall normalize separators, remove unsupported characters, collapse whitespace, and lowercase the result.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/context/detect_test.go`
