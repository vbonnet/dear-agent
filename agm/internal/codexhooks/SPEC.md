# Codex Hook Attestation Specification

<!-- Last audited at: 2026-07-29 -->

## Overview

`agm/internal/codexhooks` is the fail-closed verification boundary that AGM
uses before requesting Codex's dangerous per-path hook-trust bypass. It binds
one sandbox hook surface to immutable, content-addressed files from one
reviewed source commit.

This boundary governs Codex processes launched or resumed through AGM. Direct
execution of an external Codex binary is outside AGM's process boundary and
requires operator-owned executable policy; a repository hook cannot mediate a
descendant script's later process execution and is not treated as that control.

## Requirements

**CHOOK-01** When AGM attests repository-scoped Codex hooks, the system shall use a fixed OS-owned Git executable with caller-supplied Git repository/configuration environment removed, resolve the canonical source repository and full current commit, read `.codex/hooks.json` and every trusted command asset referenced through `AGM_CODEX_HOOK_ROOT` only from that commit's Git objects, and produce a deterministic SHA-256 digest over their paths, Git modes, and bytes.

**CHOOK-02** When the sandbox materialization is attested or revalidated, the system shall require its hook manifest and every referenced hook to be regular non-symlink files with executable modes and bytes matching the pinned commit.

**CHOOK-03** When a trusted hook command or any executable asset it invokes references a mutable project, working, home, temporary, relative, or non-system absolute runtime path instead of a committed asset through `AGM_CODEX_HOOK_ROOT`, references a missing or uncommitted asset, or can be shadowed by a nested unattested hook manifest between the launch directory and repository root, the system shall reject the attestation; every materialized-root dependency shall be recursively pinned, and commands for events that are replaced with an OS-owned no-op before bypassed launch are excluded from runtime-asset attestation.

**CHOOK-04** When persisted hook evidence is revalidated, the system shall require the same canonical source-repository identity, a full hexadecimal Git object ID, a hexadecimal SHA-256 digest, reachable committed assets, and an exact sandbox digest match.

**CHOOK-05** When the mutable source working tree changes after attestation, the system shall continue to verify against the pinned Git objects and shall not treat working-tree bytes as reviewed hook evidence.

**CHOOK-06** When AGM prepares a sandboxed Codex session, the reviewed source repository shall remain outside every agent-writable root and shall not be forwarded to Codex as an `--add-dir`.

**CHOOK-07** When AGM authorizes the hook-trust bypass, the system shall materialize the exact attested manifest and referenced hooks in a content-addressed, read-only directory outside every agent-writable root, reject missing or unexpected assets, load those hooks through immutable session configuration, disable their mutable project-layer copies through Codex hook state, and preserve the project's trusted status for the full Codex process lifetime.

**CHOOK-08** When AGM launches or cold-resumes a bypassed Codex session, the private executor shall require a clean absolute attested hook root, inject it as `AGM_CODEX_HOOK_ROOT`, and reject a hook root when the bypass is absent.

**CHOOK-09** When the private executor receives a Codex hook-trust bypass, the system shall reject direct requests and ordinary handoffs, require a one-shot prepared capability bound to the exact hook root and complete launch request, and stage that capability outside the workspace and every agent-writable root.

**CHOOK-10** When an ordinary non-bypassed Codex session runs from a repository subdirectory, the system shall resolve every project hook relative to the repository project root instead of that subdirectory.

**CHOOK-11** When AGM loads attested command hooks into a bypassed Codex session, the system shall invoke every command through an absolute OS shell with a fixed hook-only executable search path, independent of the caller's interactive `PATH`, reject every existing search-path directory or ancestor that is not root-owned and non-writable by group or other, reject mutable runtime paths and executable referenced hook assets whose interpreter is not an allowed absolute OS interpreter, and keep required hook helpers reachable only through an operator-owned absolute installation path or the content-addressed hook root.

**CHOOK-12** When a bypassed Codex session reaches a trusted Stop or SubagentStop hook, the system shall not execute project-root programs, build recipes, or other mutable workspace code automatically.

**CHOOK-13** When a reviewed hook event intentionally executes workspace code or depends on non-system workspace tooling, the system shall preserve and disable the mutable project handler identity but replace the bypassed session handler with an OS-owned no-op; ordinary non-bypassed Codex sessions retain the reviewed project behavior.

**CHOOK-14** When an attested input-inspection hook parses Codex JSON, the system shall require the fixed `/usr/local/libexec/dear-agent-codex-hook-json` identity to resolve to a root-owned, non-writable executable regular file behind root-owned, non-writable ancestors before launch.

**CHOOK-15** When an operator approves a Codex hook-trust source, the system shall resolve and display the canonical repository, full current commit, and committed hook-byte digest without trusting mutable working-tree bytes, and every private executor shall revalidate that exact persisted identity, materialization, and sandbox asset set before launch.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/codexhooks/verify_test.go`
