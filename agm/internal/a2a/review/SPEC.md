# A2A Review Aggregator Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/review` extracts review scores from A2A channels, computes
consensus state, and writes review metadata back into channel headers.

## EARS Requirements

**A2A-REV-01** When creating a review aggregator, the system shall require a non-empty channels directory and normalize an `active` directory to its parent.

**A2A-REV-02** When extracting scores, the system shall read `Review-Score` values from channel content and ignore scores outside the inclusive 1 through 10 range.

**A2A-REV-03** When fewer than the minimum reviews are present, the system shall report awaiting-review with the current count, mean, and remaining-review message.

**A2A-REV-04** When enough reviews are present, the system shall compare the mean against consensus and escalation thresholds to produce consensus-reached, blocked, or escalate status.

**A2A-REV-05** When updating channel metadata, the system shall write review count, mean, status, and first consensus timestamp while preserving other metadata.

**A2A-REV-06** When the channel file is missing or unreadable, the system shall return review data with an error message instead of panicking.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/review/aggregator_test.go`
