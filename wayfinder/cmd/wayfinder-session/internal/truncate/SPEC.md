# Truncate SPEC

<!-- Last audited at: 2026-08-01 -->

## Purpose

Provide deterministic output compression for LLM contexts.

## EARS Requirements

**TRN-01** The system shall enforce output length limits during truncation.

**TRN-02** The system shall align truncation cuts to line breaks.

**TRN-03** The system shall insert transparent omission markers when truncating.

## BDD Traceability

- Feature: `docs/features/truncate.feature`
