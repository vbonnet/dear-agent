# Codex Hook Attestation Specification

<!-- Last audited at: 2026-07-29 -->

## Overview

`agm/internal/codexhooks` is the fail-closed verification boundary that AGM
uses before requesting Codex's dangerous per-path hook-trust bypass. It binds
one sandbox hook surface to immutable, content-addressed files from one
reviewed source commit.

## Requirements

**CHOOK-01** When AGM attests repository-scoped Codex hooks, the system shall resolve the canonical source repository and full current commit, read `.codex/hooks.json` and every project-referenced hook only from that commit's Git objects, and produce a deterministic SHA-256 digest over their paths, Git modes, and bytes.

**CHOOK-02** When the sandbox materialization is attested or revalidated, the system shall require its hook manifest and every referenced hook to be regular non-symlink files with executable modes and bytes matching the pinned commit.

**CHOOK-03** When a hook manifest contains an unsupported project-directory reference, references a missing or uncommitted asset, or can be shadowed by a nested unattested hook manifest between the launch directory and repository root, the system shall reject the attestation.

**CHOOK-04** When persisted hook evidence is revalidated, the system shall require the same canonical source-repository identity, a full hexadecimal Git object ID, a hexadecimal SHA-256 digest, reachable committed assets, and an exact sandbox digest match.

**CHOOK-05** When the mutable source working tree changes after attestation, the system shall continue to verify against the pinned Git objects and shall not treat working-tree bytes as reviewed hook evidence.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/codexhooks/verify_test.go`
