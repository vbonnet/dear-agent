# Claude UI Session Store Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/claudeui` locates and safely updates Claude desktop session JSON.
This package is a Claude-only storage adapter and does not define AGM's shared
archive or lifecycle contract.

## Requirements

**CUI-01** When the Claude UI store has one device and account directory, the system shall autodetect that store.

**CUI-02** When store selection is ambiguous or an explicit selector is absent, the system shall refuse to guess a device or account directory.

**CUI-03** When a session file has an unknown or ambiguous schema, the system shall report and skip it without rewriting the file.

**CUI-04** When archive state changes, the system shall replace only the single validated `isArchived` token using an atomic write that preserves file permissions.

**CUI-05** When the requested archive state already matches the file, the system shall return an idempotent no-op without touching disk or creating a backup.

**CUI-06** When backup is requested before mutation, the system shall write a byte-identical copy before changing the source file.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/claudeui/*_test.go`
