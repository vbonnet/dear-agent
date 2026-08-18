# Bead Close Guard Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/bead-close-guard` enforces that referenced pull requests are merged and
touched deploy targets are current before a Bead can be closed.

## EARS Requirements

**BCG-01** When no Bead identifier is supplied, the command shall reject the request before querying external systems.

**BCG-02** When a Bead references no pull requests, the merged gate shall pass.

**BCG-03** When a referenced pull request is not merged, the command shall block closure and report that pull request's state.

**BCG-04** When pull-request state cannot be verified, the command shall return an error rather than treating unknown evidence as merged.

**BCG-05** When merged changes touch configured deploy targets, the command shall compare committed source artifacts with deployed host artifacts.

**BCG-06** When a touched required deploy target is stale or missing, the command shall block closure and print deduplicated remediation commands.

**BCG-07** When deployed-gate configuration or the repository root is unavailable, the command shall explain the skipped best-effort gate without manufacturing drift.

**BCG-08** When an abandon reason overrides a blocked closure, the command shall append a private structured audit record containing the reason and affected pull requests.

**BCG-09** When closure is blocked, the command shall append a best-effort VROOM decision-trail event without weakening the block if trail persistence fails.

**BCG-10** When verify-only mode is selected, the command shall report a passing or failing Definition-of-Done verdict without closing the Bead.

**BCG-11** When closure is blocked by policy, the command shall exit with code 2; when infrastructure fails, it shall exit with code 1.

**BCG-12** When the guard is installed for attested Codex hooks, the system shall require typed confirmation of the complete artifact SHA-256 and fresh interactive operator authentication, copy the approved bytes into unique root-owned staging, verify the staged digest, atomically activate the fixed hook path only on an exact match, and retain the user-path installation for ordinary harness and CLI use.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/bead-close-guard/*_test.go`
