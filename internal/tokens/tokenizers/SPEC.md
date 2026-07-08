# Tokenizer Registry Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`internal/tokens/tokenizers` defines the pluggable tokenizer interface and
registry used by token estimation. It provides a simple deterministic baseline
and optional tiktoken support while keeping quota monitoring resilient when a
provider-specific encoding cannot initialize.

## EARS Requirements

**TOKENIZER-01** When a tokenizer is registered, the system shall reject duplicate tokenizer names.

**TOKENIZER-02** When callers list registered tokenizers, the system shall return a copy that is safe to iterate while the registry is locked.

**TOKENIZER-03** When callers request a tokenizer by name, the system shall return the registered tokenizer or nil when it is absent.

**TOKENIZER-04** When the simple tokenizer counts text, the system shall split on non-letter and non-number runes and return zero for empty text.

**TOKENIZER-05** When tiktoken cannot initialize its encoding, the system shall report it unavailable so token estimation can skip it.

**TOKENIZER-06** When tiktoken is available, the system shall count tokens with the `cl100k_base` encoding.

**TOKENIZER-07** When tokenizer implementations are used concurrently, the system shall keep registration and lookup thread-safe.

## BDD Traceability

- `agm/test/bdd/features/quota_parity.feature`

## Package Test Traceability

- `internal/tokens/tokenizers/tokenizers_test.go`
