# PreTool Bash Blocker Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`pretool-bash-blocker` blocks known dangerous Bash command patterns before
Claude Code or another hook-enabled harness executes the command.

## EARS Requirements

**PBB-01** When the tool call is not Bash, the system shall allow execution.

**PBB-02** When the tool input is JSON containing a command field, the system shall extract that command for validation.

**PBB-03** When the embedded pattern file contains active rules, the system shall compile and evaluate them in rule order.

**PBB-04** When a command matches a blocking rule without an exemption, the system shall return the hook protocol blocking exit code.

**PBB-05** When brace expansion appears in a safe Go-style path context, the system shall exempt it unless the expansion is dangerous.

**PBB-06** When no blocking rule matches, the system shall allow execution.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/pretool-bash-blocker/main_test.go`

