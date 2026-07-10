# Engram Plugin Signing Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/signing` defines plugin signature, trust-store, revocation, and
blocklist contracts and supplies deterministic mock implementations for tests.

## EARS Requirements

**ESG-01** When a plugin is signed, the system shall hash its governed files, include signer and certificate metadata, and sign the serialized payload.

**ESG-02** When a plugin signature is verified, the system shall require a parseable non-empty certificate chain trusted by the configured trust store.

**ESG-03** When signature payload verification runs, the system shall verify the decoded RSA SHA-256 signature over the serialized payload.

**ESG-04** When a governed plugin file is missing or its hash differs from the signed payload, the system shall reject the signature.

**ESG-05** When a certificate authority is added or removed, the system shall address it by SHA-256 fingerprint and reflect the change in subsequent trust checks.

**ESG-06** When a certificate chain is empty, the system shall reject the trust check.

**ESG-07** When a certificate is revoked, the system shall record its serial number, fingerprint, timestamp, and revocation reason and expose it through revocation checks and the CRL.

**ESG-08** When plugins or certificate serial numbers are blocked or unblocked, the system shall update the corresponding reasoned blocklist safely under concurrent access.

**ESG-09** When blocklist snapshots are returned, the system shall return independent maps so callers cannot mutate stored policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_security_token_guardrails.feature`
- Package tests: `engram/internal/signing/*_test.go`
