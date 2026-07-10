# Engram Prompt Boundary Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/prompt` separates trusted instructions, user intent, and
untrusted retrieved data while rejecting prompt-injection patterns.

## EARS Requirements

**EPM-01** When a query contains reserved XML instruction tags, the system shall reject it rather than allowing hierarchy injection.

**EPM-02** When a query contains a known prompt-injection pattern, the system shall reject it with a security error.

**EPM-03** When a query passes validation, the system shall escape XML-significant characters before interpolation.

**EPM-04** When a secure prompt is rendered, the system shall place the user query in the user boundary and external content in the untrusted-data boundary.

**EPM-05** When prompt values are empty, the system shall preserve the complete instruction hierarchy and both data boundaries.

**EPM-06** When external data contains instructions, the system shall render it as data without granting it system or user authority.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/prompt/sanitizer_test.go`
