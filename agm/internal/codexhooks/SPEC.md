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

**CHOOK-06** When AGM prepares a sandboxed Codex session, the reviewed source repository shall remain outside every agent-writable root and shall not be forwarded to Codex as an `--add-dir`.

**CHOOK-07** When AGM authorizes the hook-trust bypass, the system shall materialize the exact attested manifest and referenced hooks in a content-addressed, read-only directory outside every agent-writable root, reject missing or unexpected assets, and make that immutable root the project-hook execution root for the full Codex process lifetime.

**CHOOK-08** When AGM launches or cold-resumes a bypassed Codex session, the private executor shall require a clean absolute attested hook root, inject it as `AGM_CODEX_HOOK_ROOT`, and reject a hook root when the bypass is absent.

**CHOOK-09** When the private executor receives a Codex hook-trust bypass, the system shall reject direct requests and ordinary handoffs, require a one-shot prepared capability bound to the exact hook root and complete launch request, and stage that capability outside the workspace and every agent-writable root.

**CHOOK-10** When an ordinary non-bypassed Codex session runs from a repository subdirectory, every project hook shall fall back to the repository project root instead of resolving relative to that subdirectory.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/codexhooks/verify_test.go`
