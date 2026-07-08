# Engram Built-in Hook Checks Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/hooks/builtin` contains built-in verification checks for cleanup,
documentation, and test execution. These checks emit `engram/hooks`
verification results so they can be run through the same runtime and reporting
contract as custom hooks.

## EARS Requirements

**EHB-01** When cleanup checks find temporary files matching the configured root-level patterns, the system shall return a warning result listing relative file paths and remediation guidance.

**EHB-02** When cleanup checks find merged branches other than `main` or `master`, the system shall return a warning result with branch cleanup guidance.

**EHB-03** When cleanup checks find existing secondary worktrees, the system shall return a warning result asking the operator to review unused worktrees.

**EHB-04** When cleanup checks find no cleanup findings, the system shall return a passing verification result.

**EHB-05** When documentation checks are run, the system shall require `README.md`, `SPEC.md`, and `ARCHITECTURE.md` in the project root.

**EHB-06** When recent code changes occur without documentation changes, the system shall warn that existing documentation may be stale.

**EHB-07** When documentation checks cannot inspect git history, the system shall skip stale-document detection rather than failing the hook.

**EHB-08** When test framework detection walks a project, the system shall detect Go, Python, JavaScript, and Rust test conventions while skipping hidden directories.

**EHB-09** When no test framework is detected, the system shall return a warning asking for tests.

**EHB-10** When a detected framework's test command fails, the system shall return a high-severity violation.

**EHB-11** When parsed coverage is below the configured threshold, the system shall return a high-severity violation with coverage remediation guidance.

**EHB-12** When the test execution hook emits a failing verification result, the system shall exit non-zero after writing JSON output.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks/builtin/*_test.go`

