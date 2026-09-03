# AGM Test Context Specification

<!-- Last audited at: 2026-07-23 -->

## Requirements

**TCTX-01** When a test context is created, the system shall allocate isolated home, session, database, lock, and tmux socket paths under a run-specific temporary root.

**TCTX-02** When a test context exports its environment, the system shall include the run identifier, isolated paths, and isolated home consistently for in-process and subprocess use.

**TCTX-03** If no test run marker exists in the environment, then the system shall report that no test context can be reconstructed.

**TCTX-04** When a test context is cleaned up, the system shall remove its socket and run-specific directory tree without affecting unrelated test runs.

**TCTX-05** When a test context exports its environment, the system shall route runtime readiness and lock state through its run-specific state directory instead of the user's AGM state directory.

**TCTX-06** When a named test environment is constructed or reconstructed, the system shall reject empty, absolute, path-separated, or control-character names before deriving any filesystem path.

**TCTX-07** When a new named test environment is created, the system shall derive its paths beneath one short effective-user root shared by activation, discovery, and cleanup, with canonical entries taking precedence over legacy duplicates.

**TCTX-08** When a named environment exists beneath a retired short or host temporary root, the system shall activate that exact validated environment during reconstruction and remove its directory and socket during explicit destroy without mutating sibling paths.

**TCTX-09** When a new named environment exceeds the socket-length budget, the system shall reject creation while retaining discovery and cleanup access for a path-safe legacy name.

**TCTX-10** When the canonical short test root is resolved, created, reused, or cleaned, the system shall verify it is a real directory owned by the effective user and enforce owner-only permissions before traversing environment state beneath it.

**TCTX-11** When a retired host temporary root is considered for compatibility, the system shall require a real owner-only directory owned by the effective user before resolving or cleaning any named child.

**TCTX-12** If an owned retired environment already uses a requested name, then the system shall reject new canonical creation while retaining reconstruction and explicit-destroy access to the existing environment.

### Inherited authentication

The approved credential leaves are exactly:

- `.claude/.credentials.json`
- `.codex/auth.json`
- `.local/share/opencode/auth.json`
- `.config/gcloud/application_default_credentials.json`

The approved compatibility configuration leaves are exactly:

- `.config/gcloud/configurations/config_default`
- `.config/opencode/opencode.json`
- `.config/opencode/opencode.jsonc`
- `.config/opencode/tui.json`
- `.config/opencode/tui.jsonc`

**TCTX-13** While inherited authentication is selected, the system shall expose each present approved credential leaf at the same relative path in the selected home through one exact symbolic link that can relay later provider reads or writes only to that host credential leaf.

**TCTX-14** When the host replaces an approved credential leaf after successful projection, the system shall expose the replacement through the selected home's existing credential link without exposing sibling host state.

**TCTX-15** While inherited authentication is selected, the system shall install each present approved compatibility configuration leaf at the same relative path in the selected home as a `0600` regular-file snapshot detached from later host changes.

**TCTX-16** When an approved compatibility configuration leaf is prepared, the system shall accept at most 1 MiB of snapshot content, probe at most 1 MiB plus one byte per verification read to detect overflow, and reject a larger or identity-changing source before destination mutation.

**TCTX-17** If an approved credential leaf is not owner-private or any approved source or its ancestry is redirected, wrong-owner, writable by group or other, or not the required real directory or regular-file type, then the system shall reject inherited authentication before destination mutation.

**TCTX-18** If the selected home or an existing destination parent is redirected, wrong-owner, non-private, or not a real directory, then the system shall reject inherited authentication before mutation begins or, when the change occurs during apply, before the next destination mutation.

**TCTX-19** The system shall not project a provider directory, wildcard match, recursively discovered file, gcloud active or nondefault profile, OpenCode data or extension, or provider project, transcript, session, log, plugin, hook, cache, database, configuration, or trust state outside the approved leaves.

**TCTX-20** If inherited-authentication plan application fails after destination creation begins, then the system shall remove only unchanged nodes created by that attempt in reverse order and preserve pre-existing or identity-changed state.

**TCTX-21** When a synthetic provider-onboarding process or the real Codex trust-state writer runs with the selected home selected through `HOME`, the system shall keep those writes below the selected home and leave synthetic host-home sentinels unchanged.

**TCTX-22** The system shall not include credential or compatibility configuration contents in an inherited-authentication error.

**TCTX-23** If an approved source leaf is absent, then the system shall skip that leaf without error.

**TCTX-24** If a legacy whole-provider link or pre-existing target leaf exists in the selected home, then the system shall reject inherited authentication before destination mutation.

**TCTX-25** While destination preflight or apply is active, the system shall retain an opened selected-home root, serialize cooperating inherited-authentication projections with an advisory directory lock, and perform creation through descriptors rooted below that opened home.

**TCTX-26** While a transaction-created node can still be compensated, the system shall retain its opened identity handle; when rollback begins, the system shall move each candidate through a no-replace rename into the opened selected-home root, verify it against that retained identity, delete only a matching node, and restore an identity-changed node without replacing another entry.

**TCTX-27** The transaction's concurrency guarantee applies to cooperating inherited-authentication projectors. Arbitrary mutation by another process running as the same Unix user is outside that serialization boundary, but replacement of the selected-home pathname shall not redirect transaction writes outside the opened selected home.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/internal/testcontext/*_test.go`
