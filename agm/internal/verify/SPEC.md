# Completion Verification Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/verify` extracts verifiable claims from session purpose text and
checks repository state to detect false completion.

## Requirements

**VER-01** When purpose text declares file creation, directory deletion, dependency removal, or code changes, the system shall extract the corresponding typed assertions.

**VER-02** When checking a positive pattern assertion, the system shall require matching non-binary repository content outside excluded hidden directories.

**VER-03** When checking a negative pattern or directory-removal assertion, the system shall fail if the forbidden content or path still exists.

**VER-04** When assertions are verified, the system shall return a report containing every individual result and an aggregate pass state.

**VER-05** When a claimed outcome is not supported by repository evidence, the system shall mark the report as false completion.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/verify/*_test.go`
