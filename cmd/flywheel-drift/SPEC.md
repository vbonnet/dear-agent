# Flywheel Drift Command Specification

<!-- Last audited at: 2026-08-01 -->

## Overview

`cmd/flywheel-drift` correlates stale Beads with recent observability evidence
to expose work that has stopped feeding the delivery flywheel.

## EARS Requirements

**FDC-01** When Beads are listed, the command shall decode typed identifiers, titles, states, priorities, and update timestamps.

**FDC-02** When a Bead is closed or updated within the configured threshold, the command shall not classify it as stale.

**FDC-03** When an open Bead exceeds the stale threshold, the command shall preserve it as a drift candidate.

**FDC-04** When observability lookup runs, the command shall apply the configured service and lookback window.

**FDC-05** When Jaeger or Beads evidence cannot be read, the command shall report that failure rather than declaring the flywheel healthy.

**FDC-06** When JSON output is selected, the command shall emit the same stale and observability evidence represented in text output, preserve the four declared health-status wire strings, and return the shared health summary exit code.

**FDC-07** When no stale work exists, the command shall report a healthy steady state and exit successfully.

**FDC-08** When evidence originates from different harnesses or models, the command shall correlate shared Bead and trace identifiers without provider-specific assumptions.

**FDC-09** When the health runner canonicalizes malformed check output, the JSON report shall emit a non-fixable error result and return critical exit code 2.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `cmd/flywheel-drift/*_test.go`
