# Bead Pull Request Synchronization Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/bead-pr-sync` reconciles closed Beads with the actual merge state of their
referenced pull requests.

## EARS Requirements

**BPS-01** When reconciliation starts, the command shall inspect closed Beads from the selected Beads directory.

**BPS-02** When Bead detail references pull requests, the command shall extract local-repository references, deduplicate numbers, and ignore cross-repository references.

**BPS-03** When any referenced pull request remains open, the command shall reopen the Bead as in progress and append a comment naming those pull requests.

**BPS-04** When referenced pull requests are merged or closed, the command shall not reopen the Bead.

**BPS-05** When pull-request state cannot be verified, the command shall report the error and shall not claim successful reconciliation.

**BPS-06** When `BEADS_DIR` is overridden, the command shall provide the selected directory to every `bd` subprocess through its environment.

**BPS-07** When dry-run mode is enabled, the command shall report intended reopen operations without mutating Beads.

**BPS-08** When reconciliation completes, the command shall report scanned, reopened, clean, skipped, and error counts.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/bead-pr-sync/*_test.go`
