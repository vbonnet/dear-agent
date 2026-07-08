# Engram Hook Bash Validator Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/hooks-bin/internal/validator` validates Bash commands before tool
execution. It recognizes forbidden command patterns, maps patterns to policy
categories, supports warn or block modes for configurable categories, and
always hard-blocks dangerous patterns.

## EARS Requirements

**EHV-01** When no validator config is supplied, the system shall use the default config where every category blocks.

**EHV-02** When a dangerous category is queried, the system shall return block mode regardless of caller configuration.

**EHV-03** When an unknown pattern index is classified, the system shall treat it as dangerous.

**EHV-04** When command text is scanned, the system shall strip quoted content so prose inside prompts does not trigger shell-syntax patterns.

**EHV-05** When double-quoted command text contains `$(`, the system shall preserve the command substitution marker for validation.

**EHV-06** When pipe-exempt file-operation patterns are evaluated, the system shall check only the first pipeline segment and shall not split on logical OR.

**EHV-07** When a blocked pattern matches, the system shall return `Allowed=false`, the pattern name, the selected remediation, and block mode.

**EHV-08** When a warn-mode category pattern matches, the system shall return `Allowed=true`, the pattern name, the selected remediation, and warn mode.

**EHV-09** When no forbidden pattern matches, the system shall allow the command without a pattern name or remediation.

**EHV-10** When validation logging cannot open its log file, the system shall fail open and continue validation.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/validator/validator_test.go`
- Package tests: `engram/hooks-bin/internal/validator/validator_bdd_test.go`

