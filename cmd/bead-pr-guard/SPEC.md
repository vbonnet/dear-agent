# Bead Pull Request Guard Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/bead-pr-guard` prevents multiple open pull requests from claiming the same
Bead identifier.

## EARS Requirements

**BPG-01** When a Bead identifier is supplied, the command shall search open pull-request titles, bodies, and branch names for exact token claims.

**BPG-02** When a candidate contains only a longer substring of the Bead identifier, the command shall not treat it as a claim.

**BPG-03** When the current pull request number is supplied, the command shall exclude that pull request from duplicate-claim findings.

**BPG-04** When another open pull request claims the Bead, the command shall block creation and print the existing pull request for inspection.

**BPG-05** When GitHub state cannot be queried within the bounded timeout, the command shall return an infrastructure error rather than allowing an unverified duplicate.

**BPG-06** When repository detection is required, the command shall accept GitHub HTTPS and SSH origin URLs and reject unrecognized remotes.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/bead-pr-guard/*_test.go`
