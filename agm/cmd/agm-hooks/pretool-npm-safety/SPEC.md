# PreTool npm Safety Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`pretool-npm-safety` blocks high-risk npm, npx, and node command forms before
Bash execution.

## EARS Requirements

**PNS-01** When the tool call is not Bash, the system shall allow execution.

**PNS-02** When the tool input is JSON containing a command field, the system shall extract that command for validation.

**PNS-03** When a command invokes high-risk npm account, access, publish, deprecate, token, or unpublish operations, the system shall block execution.

**PNS-04** When a command invokes npx with an unknown package, the system shall block execution.

**PNS-05** When a command invokes npx with a known-safe package, the system shall exempt the command from the unknown-npx rule.

**PNS-06** When a command opens Node.js inspector ports or uses node eval, the system shall block execution.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/pretool-npm-safety/main_test.go`

