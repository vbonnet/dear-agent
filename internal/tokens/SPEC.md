# Token Estimation Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`internal/tokens` calculates context-size estimates for engram files and other
text artifacts. It combines an always-present `char/4` baseline with optional
registered tokenizers so quota monitoring can expose truthful context estimates
across harnesses and model families even when provider-specific tokenizers are
unavailable.

## EARS Requirements

**TOKENS-01** When token calculation is requested without any engram paths, the system shall return an error.

**TOKENS-02** When token calculation reads engram files, the system shall clean each path before reading it.

**TOKENS-03** When token calculation succeeds, the system shall report total character count and the `char/4` token baseline.

**TOKENS-04** When registered tokenizers are available, the system shall run them concurrently and include successful counts by tokenizer name.

**TOKENS-05** When a tokenizer is unavailable, fails, or panics, the system shall skip that tokenizer without failing the whole calculation.

**TOKENS-06** When estimates are serialized to JSON, the system shall include `char_count`, `tokens_char4`, and successful tokenizer counts.

## BDD Traceability

- `agm/test/bdd/features/quota_parity.feature`

## Package Test Traceability

- `internal/tokens/calculator_test.go`
- `internal/tokens/real_engrams_test.go`
