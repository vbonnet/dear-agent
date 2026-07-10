# Engram Tokenizer Registry Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/tokens/tokenizers` provides a concurrent tokenizer registry,
a dependency-free baseline, and optional `cl100k_base` tokenization.

## EARS Requirements

**ETK-01** When a tokenizer is registered under a new name, the system shall expose it through name lookup and registry snapshots.

**ETK-02** When a tokenizer name is registered more than once, the system shall panic rather than silently replace the existing implementation.

**ETK-03** When registry snapshots are returned, the system shall return an independent slice safe for iteration during concurrent registry access.

**ETK-04** When the simple tokenizer counts content, the system shall split on non-letter and non-number runes and support Unicode text without external dependencies.

**ETK-05** When the simple tokenizer receives empty content, the system shall return zero tokens.

**ETK-06** When tiktoken is first used, the system shall initialize `cl100k_base` once, use a private Engram cache directory by default, and remain safe under concurrent calls.

**ETK-07** When tiktoken initialization fails, the system shall mark it unavailable and return an explicit count error on subsequent calls.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_security_token_guardrails.feature`
- Package tests: `engram/internal/tokens/tokenizers/*_test.go`
