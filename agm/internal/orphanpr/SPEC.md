# Orphaned Pull Request Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/orphanpr` detects open pull requests whose referenced Beads work
is already closed, missing, or otherwise no longer active.

## Requirements

**OPR-01** When pull request text contains canonical Beads references, the system shall extract each unique referenced bead ID.

**OPR-02** When an open pull request references an active bead, the system shall not classify that reference as orphaned.

**OPR-03** When every referenced bead is closed, missing, or unavailable, the system shall report the pull request with the evidence used for classification.

**OPR-04** When pull request or Beads state cannot be loaded, the system shall preserve an unknown status instead of claiming closure.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/orphanpr/*_test.go`
