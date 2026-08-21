# Tofu Import Plan Command Specification

<!-- Last audited at: 2026-08-19 -->

## Overview

`cmd/tofu-import-plan` is the decision surface `infra/import.sh` calls. It
turns recorded evidence about GitHub and OpenTofu state into a deterministic
import plan, so the script keeps only evidence collection and execution. The
command contacts no network and mutates no state.

## EARS Requirements

**TIPC-01** When invoked with `repos`, the command shall print the validated active repositories one per line, so the caller knows which ruleset listings to collect.

**TIPC-02** When invoked with `plan`, the command shall emit one record per step, each naming a verb of `import`, `skip` or `create`, the state address, the import identifier, and the reason.

**TIPC-03** When a ruleset listing file is missing for an active repository, the command shall fail rather than plan that repository as having no ruleset.

**TIPC-04** When the OpenTofu state file is absent or empty, the command shall treat the state as empty rather than fail, since that is the first-run case.

**TIPC-05** When the canonical ruleset document has no name, the command shall fail rather than plan against an unnamed policy.

**TIPC-06** When invoked with `classify`, the command shall exit zero only if the recorded provider output matches a recognized absent-object message.

**TIPC-07** When invoked without a recognized subcommand, or without a required flag, the command shall fail with a usage error.

**TIPC-08** When planning fails for any reason, the command shall emit no plan records, so a caller cannot execute a partial plan.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package tests: `internal/tofuimport/*_test.go`
- Script behavior: `tests/bats/infra-import.bats`
