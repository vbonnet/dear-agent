# Pull Request Linkifier Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`internal/prlinkify` converts and extracts explicit pull-request references
without rewriting protected code or existing Markdown links.

## EARS Requirements

**PRL-01** When a singular `PR #N` reference is found, the system shall convert it to a Markdown link for the configured repository.

**PRL-02** When a plural pull-request reference is found, the system shall expand each referenced number into its own link.

**PRL-03** When references occur inside fenced code, inline code, or an existing Markdown link, the system shall preserve the protected text unchanged.

**PRL-04** When references are extracted, the system shall ignore unrelated issue-number tokens and deduplicate repeated pull-request numbers.

**PRL-05** When a custom repository is configured, the system shall use it in every generated URL.

**PRL-06** When no repository is configured, the system shall use `vbonnet/dear-agent` as the compatibility default.

**PRL-07** When input contains no pull-request references, the system shall return unchanged text and an empty reference list.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `internal/prlinkify/*_test.go`
