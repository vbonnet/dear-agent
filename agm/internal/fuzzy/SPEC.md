# Fuzzy Matching Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/fuzzy` provides deterministic, case-insensitive candidate
suggestions using normalized Levenshtein similarity.

## Requirements

**FUZ-01** When comparing an input with candidates, the system shall compute case-insensitive Levenshtein distance while returning original candidate spelling.

**FUZ-02** When a candidate's normalized similarity is below the supplied threshold, the system shall exclude that candidate.

**FUZ-03** When multiple candidates qualify, the system shall order results by descending similarity and return at most five matches.

**FUZ-04** When the candidate set is empty, the system shall return an empty result without error.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/fuzzy/*_test.go`
