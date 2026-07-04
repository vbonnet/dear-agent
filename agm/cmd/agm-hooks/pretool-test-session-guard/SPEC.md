# PreTool Test Session Guard Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`pretool-test-session-guard` prevents accidental production creation of
`test-*` AGM sessions unless explicit test isolation or override is requested.

## EARS Requirements

**PTS-01** When the tool call is not Bash, the system shall allow execution.

**PTS-02** When a Bash command is not `agm session new`, the system shall allow execution.

**PTS-03** When an `agm session new` command creates a name that does not start with `test-`, the system shall allow execution.

**PTS-04** When a `test-*` session is created with `--test`, the system shall allow execution.

**PTS-05** When a `test-*` session is created with `--allow-test-name`, the system shall allow execution.

**PTS-06** When a `test-*` session is created without `--test` or `--allow-test-name`, the system shall block execution and print remediation.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/pretool-test-session-guard/main_test.go`

