# Engram Token Estimation Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/tokens` computes backward-compatible and tokenizer-specific
size estimates for one or more Engram files.

## EARS Requirements

**ETC-01** When no Engram paths are supplied, the system shall return an explicit no-input error.

**ETC-02** When one or more Engram paths are supplied, the system shall clean each path, read every file, and fail with path context if any file cannot be read.

**ETC-03** When file content is loaded, the system shall aggregate the exact character count and always report the integer character-count-divided-by-four estimate.

**ETC-04** When registered tokenizers are available, the system shall execute them concurrently over the combined content.

**ETC-05** When a tokenizer is unavailable, fails, or panics, the system shall omit that tokenizer's result without discarding successful estimates.

**ETC-06** When multiple callers calculate estimates concurrently, the system shall return independent result maps without data races.

**ETC-07** When an empty file is estimated, the system shall report zero characters and zero baseline tokens while retaining successful tokenizer results.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_security_token_guardrails.feature`
- Package tests: `engram/internal/tokens/*_test.go`
