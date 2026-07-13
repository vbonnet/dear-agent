# Hook Output Limiter Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/limiter` bounds per-hook and cumulative advisory
output so hooks cannot consume an unbounded model context budget.

## EARS Requirements

**EHOL-01** When output remains within the per-hook budget, the limiter shall pass it through and account for its approximate tokens.

**EHOL-02** When output exceeds the budget, the limiter shall write only the fitting prefix, emit one named truncation notice, and acknowledge later writes without forwarding them.

**EHOL-03** When multiple goroutines use a limiter or session tracker, the system shall serialize mutable accounting state.

**EHOL-04** When persisted session budget state is missing or malformed, the tracker shall initialize empty maps and a zero token count.

**EHOL-05** When cumulative usage reaches 5,000 tokens, the tracker shall reduce enabled hooks to a 250-token invocation budget.

**EHOL-06** When a hook truncates output three consecutive times, the tracker shall disable that hook until session budget state is reset.

**EHOL-07** When a hook completes without truncation, the tracker shall reset that hook's consecutive violation count.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/limiter/*_test.go`
