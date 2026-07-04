# A2A Token Budget Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/token` provides lightweight token-budget estimation for A2A
messages so coordination content stays compact enough for repeated agent turns.

## EARS Requirements

**A2A-TOK-01** When text is counted, the system shall estimate tokens from rune count using the package's configured characters-per-token ratio.

**A2A-TOK-02** When text exceeds the maximum token budget, the system shall reject budget validation with a message that includes actual and maximum tokens.

**A2A-TOK-03** When text is within the maximum budget, the system shall report it as within budget.

**A2A-TOK-04** When text is between the minimum and maximum budget bounds, the system shall report it as optimal.

**A2A-TOK-05** When remaining budget would be negative, the system shall clamp remaining budget to zero.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/token/counter_test.go`
