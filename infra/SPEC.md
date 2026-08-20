# Repository Infrastructure Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**INFRA-01** When repository infrastructure is planned or applied, the system shall derive managed resources from versioned Terraform inputs.

**INFRA-02** If an import or apply operation cannot identify the intended resource, the system shall fail before mutating unrelated infrastructure state.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`

## Exact-plan authorization

The following contract defines provider-independent behavior for deciding
whether an infrastructure plan is routine enough for unattended production
application. It intentionally does not prescribe a CI system, agent harness,
programming language, provider transport, secret store, or deployment
implementation.

### Reproducible inputs

**INFRA-ATTEST-01** When an infrastructure plan is evaluated for production application, the system shall require the exact declared tool, provider, source tag commit, release checksum manifest, platform archive, extracted binary, and dependency-lock identities.

**INFRA-ATTEST-02** If a tool, provider, platform, archive, binary, source identity, checksum manifest, or dependency lock is missing, unsupported, or mismatched, the system shall withhold routine authorization.

**INFRA-ATTEST-03** When an infrastructure plan is created, the system shall enforce encryption of the saved plan before private configuration, state, or provider values can be persisted.

**INFRA-ATTEST-04** When private plan evidence is evaluated, the system shall derive plaintext plan JSON directly from the exact encrypted subject with the exact pinned OpenTofu binary, keep the saved plan encrypted, and not persist or publish the plaintext projection.

### Bounded private evaluation

**INFRA-ATTEST-05** When private plan or infrastructure evidence is accepted, the system shall enforce explicit byte, nesting, node, and string bounds before classification.

**INFRA-ATTEST-06** If private JSON is malformed, duplicated, trailing, oversized, structurally unknown, or uses an unsupported format, the system shall withhold routine authorization without exposing the private input.

**INFRA-ATTEST-07** When a complete normal refresh plan has no changes, no drift, no output changes, no deferred work, and no non-passing checks, the system shall classify the plan as routine no-op.

**INFRA-ATTEST-08** When a complete normal refresh plan restores exactly one existing repository ruleset in place, the system shall classify the plan as routine only if its provider, resource address, repository, string state identifier, numeric provider identifier, before identity, after identity, and full desired after-state all match the trusted bindings.

**INFRA-ATTEST-09** If a plan contains creation, deletion, forgetting, replacement, movement, import, generated configuration, refresh-only reads, targeting, exclusion, replacement selection, unknown values, sensitive changes, output changes, deferred work, failed checks, ambiguous drift, multiple updates, or any unrecognized action, the system shall classify the plan as critical and withhold routine authorization.

**INFRA-ATTEST-10** If the backend, workspace, provider set, state-encryption mode, plan-encryption mode, moved declarations, removed declarations, or import declarations differ from the trusted migration surface, the system shall classify the plan as critical and withhold routine authorization.

### Authorization binding

**INFRA-ATTEST-11** When routine authorization is issued, the system shall bind it to the canonical main reference, source commit, source tree, canonical ruleset blob, exact toolchain, encrypted plan digest, state lineage, state serial, and complete private evidence set.

**INFRA-ATTEST-12** When routine authorization is issued, the system shall require a verified prior receipt for the same toolchain, state lineage, state serial, inventory, backend, state snapshot, migration surface, and provider snapshot.

**INFRA-ATTEST-13** When private evidence is represented in public claims, the system shall use nonce-bound, domain-separated keyed commitments and shall not publish raw inventory, backend, state, migration, provider, or change-projection content.

**INFRA-ATTEST-14** When equivalent trusted inputs are evaluated with the same nonce and key, the system shall emit byte-identical canonical public claims.

**INFRA-ATTEST-15** If the plan generation time, issuance time, not-before time, expiration time, or verification time is absent, non-canonical, inconsistent, expired, or outside the bounded authorization lifetime, the system shall withhold routine authorization.

**INFRA-ATTEST-16** If an authorization subject, source binding, toolchain binding, state binding, nonce, canonical encoding, or freshness window differs at verification time, the system shall reject the authorization as mismatched.

### Post-application receipt

**INFRA-ATTEST-17** When a production application is observed, the system shall issue a receipt only if the authorization remains fresh, the state serial advances on the same lineage, and provider visibility, no-drift verification, source parity, and the behavioral canary all pass.

**INFRA-ATTEST-18** When a receipt is issued, the system shall bind it to the exact authorization claims, encrypted plan digest, source, toolchain, advanced state, observation time, and a fresh nonce-bound private evidence set.

**INFRA-ATTEST-19** If a receipt reuses the authorization nonce or differs from the expected authorization, plan, source, toolchain, state, nonce, or canonical encoding, the system shall reject the receipt as mismatched.

**INFRA-ATTEST-20** If any evaluation or receipt precondition fails, the system shall emit only a bounded public-safe rejection code and shall not expose private plan, inventory, backend, state, migration, provider, key, or lineage content through claims, output, errors, or logs.

**INFRA-ATTEST-21** If a future plan, evidence, toolchain, or receipt field is not explicitly supported by this contract, the system shall withhold routine authorization until the field is deliberately classified.
