# Claude UI Session Store Specification

<!-- Last audited at: 2026-07-18 -->

## Overview

`agm/internal/claudeui` locates and safely updates Claude desktop session JSON.
This package is a Claude-only storage adapter and does not define AGM's shared
archive or lifecycle contract.

## EARS Requirements

**CUI-01** When the Claude UI store has one device and account directory, the system shall autodetect that store.

**CUI-02** When store selection is ambiguous or an explicit selector is absent, the system shall refuse to guess a device or account directory.

**CUI-03** When a session file has an unknown or ambiguous schema, the system shall report and skip it without rewriting the file.

**CUI-04** When archive state changes, the system shall replace only the single validated `isArchived` token using an atomic write that preserves file permissions.

**CUI-05** When the requested archive state already matches the file, the system shall return an idempotent no-op without touching disk or creating a backup.

**CUI-06** When backup is requested before mutation, the system shall write a byte-identical copy before changing the source file.

**CUI-07** When AGM archives a Claude session with a persisted UUID, the system shall select only desktop records whose `cliSessionId` exactly equals that UUID, including across multiple local device/account stores.

**CUI-08** When an individual Claude desktop device or account store cannot be read, the system shall record that store as a load error and continue scanning the remaining stores.

**CUI-09** When store discovery finds zero or multiple candidate device or account directories without an explicit selector, the system shall refuse the operation rather than select an arbitrary store.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/claudeui/*_test.go`
