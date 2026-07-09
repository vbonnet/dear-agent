# Verification Skip Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/skipdetect` identifies source comments and directives that bypass
or weaken required verification.

## Requirements

**SKD-01** When source lines contain a maintained skip or bypass directive, the system shall report its filename, line number, matched rule, and source text.

**SKD-02** When directive-like text appears in an allowed explanatory context, the system shall avoid reporting a false positive.

**SKD-03** When multiple findings exist, the system shall preserve deterministic source order.

**SKD-04** When no maintained skip pattern matches, the system shall return an empty finding set.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/skipdetect/*_test.go`
