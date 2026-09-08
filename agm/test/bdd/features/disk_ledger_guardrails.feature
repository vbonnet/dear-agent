# SPEC: pkg/diskledger/SPEC.md
Feature: Disk ledger package guardrails
  The disk ledger attributes disk growth to the run that caused it and decides
  whether the reclaim path is reclaiming anything. It must keep executable SPEC
  traceability because a detector that drifts from its contract fails the way
  the thing it replaced failed: silently, while reporting OK.

  Scenario Outline: Disk ledger packages declare SPEC coverage
    Given disk ledger package "<package>" is configured
    When AGM validates disk ledger package coverage
    Then disk ledger package "<package>" should have a co-located SPEC

    Examples:
      | package         |
      | pkg/diskledger  |
