# Code Intelligence Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/code-intel` detects repository languages, configures indexers, runs
bounded indexing, and reports index health without depending on a model family.

## EARS Requirements

**CIC-01** When language detection runs, the command shall derive languages from repository files through the shared code-intelligence detector.

**CIC-02** When configuration is initialized, the command shall preserve explicit repository and index paths and shall not overwrite an existing configuration without intent.

**CIC-03** When indexing is requested, the command shall use the configured language adapters and persist index state through the shared code-intelligence package.

**CIC-04** When health is checked, the command shall report missing, stale, or healthy index state with actionable guidance.

**CIC-05** When a requested language adapter is unavailable, the command shall report that gap rather than claiming a complete index.

**CIC-06** When a repository path is invalid or unreadable, the command shall return a contextual error.

**CIC-07** When help or an unknown command is requested, the command shall print the supported command surface without performing indexing.

**CIC-08** When code intelligence is consumed by any harness or model family, the command shall return the same repository-derived results.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_intelligence_command_guardrails.feature`
- Package tests: `cmd/code-intel/*_test.go`
