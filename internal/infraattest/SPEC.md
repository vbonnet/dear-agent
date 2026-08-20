# Infrastructure Attestation Package Specification

<!-- Last audited at: 2026-08-18 -->

## Overview

`internal/infraattest` is the single private-to-public seam for deciding
whether an OpenTofu plan is routine enough to apply to production without a
human in the loop, and for issuing a reconciliation receipt once that apply
has been observed.

Everything crossing into this package is private: the encrypted saved plan,
the plaintext plan JSON projected from it, the pinned OpenTofu and provider
binaries, the dependency and toolchain locks, and five evidence documents
(repository inventory, backend configuration, state snapshot, migration
surface, provider snapshot). Everything crossing back out is public-safe:
canonical `AuthorizationClaims` / `ReceiptClaims` JSON carrying nonce-bound
HMAC commitments instead of the evidence itself, or a bare `Code` from a
fixed rejection catalog.

The decision is deliberately closed rather than open. A plan is routine only
when it is a complete normal refresh plan that either changes nothing at all
or restores exactly one already-existing repository ruleset in place, with
every trusted binding matching. Any action, resource type, plan field, or
toolchain identity the package does not explicitly recognise is critical,
because an unrecognised field is indistinguishable from an unreviewed one.

Bounds precede parsing. Private JSON is read through a bounded, duplicate-key
rejecting canonicaliser (`CanonicalPrivateJSON`) with explicit byte, depth,
node, string, and number limits, so a hostile or merely enormous plan is
rejected before it can be interpreted or can exhaust memory. Number bounds are
charged against the normalised form rather than the input, because canonical
numbers are exponent-free and a nine-byte literal such as `1e4000000` spells a
four-million-byte decimal; a per-number bound and a whole-document aggregate
allowance are both priced before any expansion is built.

This package implements the `INFRA-ATTEST-01` .. `INFRA-ATTEST-21` contract in
`infra/SPEC.md`; the requirements below are the package-level obligations that
contract resolves to.

## EARS Requirements

### Reproducible toolchain

**INFRAATTEST-01** When an authorization request is evaluated, the package shall require the OpenTofu binary, provider binary, dependency lockfile, and toolchain manifest to hash to the exact pinned release, source tag commit, platform archive, and lock digests recorded in this package's constants.

**INFRAATTEST-02** If a tool, provider, platform, archive, binary, source identity, checksum manifest, or dependency lock is absent, unsupported, or mismatched, the package shall reject with `unsupported-toolchain` or `malformed-lockfile` and shall issue no authorization.

**INFRAATTEST-03** When the toolchain manifest declares platform locks, the package shall select the lock for the requested platform only, and shall reject a manifest whose platform locks are duplicated, incomplete, or absent for that platform.

### Encrypted plan subject

**INFRAATTEST-04** When an encrypted saved plan is read, the package shall require an `encryption_version` of `v0`, between one and sixteen `key_provider.<provider>.<name>` metadata entries, and a strictly-encoded ciphertext payload of at least the minimum ciphertext length.

**INFRAATTEST-05** When authorization claims are produced, the package shall bind them to the SHA-256 of the exact encrypted plan bytes, so the subject of the decision is the ciphertext that will be applied rather than its plaintext projection.

**INFRAATTEST-06** The package shall not persist, return, or embed the plaintext plan projection, the private evidence documents, or the HMAC key in claims, errors, or logs.

### Bounded private evaluation

**INFRAATTEST-07** When private JSON is consumed, the package shall enforce the declared raw-byte, nesting-depth, structural-node, and per-string byte bounds before any structural interpretation of the document.

**INFRAATTEST-25** When a private JSON number is normalised, the package shall reject it before constructing its plain-decimal form if that form would exceed `MaxJSONNumberBytes`, or if it would exceed the bytes left in the document's aggregate normalised-number allowance, which starts at the document's declared raw-byte bound; consequently a document cannot spend more memory on normalised numbers than it was permitted to occupy raw, however compactly its exponents were written, and no over-budget expansion is ever materialised or retained.

**INFRAATTEST-08** If private JSON is malformed, contains a duplicated object key, carries trailing bytes after the top-level value, exceeds a bound, or declares an unsupported plan format version, the package shall reject with the corresponding bounded code and shall not echo the offending input.

**INFRAATTEST-09** When a complete normal refresh plan reports no resource changes, no drift, no output changes, no deferred changes, and no non-passing checks, the package shall classify it as a routine no-op.

**INFRAATTEST-10** When a complete normal refresh plan updates exactly one existing repository ruleset in place, the package shall classify it as routine only if its provider, resource address, repository, string state identifier, numeric provider identifier, before identity, after identity, and full desired after-state projection all match the request's trusted bindings.

**INFRAATTEST-11** If a plan contains a create, delete, forget, replace, move, import, deposed object, generated configuration, refresh-only read, targeting, exclusion, unknown value, sensitive value, output change, deferred change, failing check, ambiguous drift, more than one update, or any action the package does not recognise, the package shall classify the plan as critical and shall issue no authorization.

**INFRAATTEST-12** If the backend, workspace, provider set, state-encryption mode, plan-encryption mode, moved declarations, removed declarations, or import declarations differ from the trusted migration surface, the package shall reject with `critical-migration-surface`.

**INFRAATTEST-13** If the repository inventory is not declared complete, the package shall reject with `inventory-incomplete`, because an incomplete inventory cannot prove that the plan covers every managed resource.

### Authorization binding and freshness

**INFRAATTEST-14** When authorization claims are issued, the package shall bind them to the canonical source reference, source commit, source tree, canonical ruleset blob, toolchain identity, encrypted plan digest, state lineage, state serial, and the complete commitment set over all five private evidence documents plus the change projection.

**INFRAATTEST-15** When authorization claims are issued, the package shall require a verified baseline receipt covering the same toolchain, state lineage, state serial, inventory, backend, state snapshot, migration surface, and provider snapshot, and shall reject with `baseline-missing` or `baseline-mismatch` otherwise.

**INFRAATTEST-16** When private evidence is represented publicly, the package shall emit only domain-separated HMAC-SHA-256 commitments keyed by a caller key of at least the minimum key length and salted by a nonce of exactly the required nonce length.

**INFRAATTEST-17** When equivalent trusted inputs are evaluated with the same nonce and key, the package shall emit byte-identical canonical claims, so an independent re-evaluation is a bytewise comparison rather than a semantic one.

**INFRAATTEST-18** If the plan generation time, issuance time, not-before time, expiration time, or evaluation time is absent, non-canonical, out of order, or spans more than the maximum commitment lifetime, the package shall reject with `freshness-invalid`.

**INFRAATTEST-19** If the subject digest, source binding, toolchain binding, state binding, nonce, canonical encoding, or freshness window presented at verification differs from the claims under verification, the package shall reject with `authorization-mismatch`.

### Post-apply receipt

**INFRAATTEST-20** When a receipt is requested, the package shall issue one only if the referenced authorization is still fresh, the state serial strictly advances on the same lineage, and the provider-visibility, no-drift, source-parity, and behavioral-canary observations are all affirmative.

**INFRAATTEST-21** When a receipt is issued, the package shall bind it to the exact authorization claims digest, applied plan digest, source, toolchain, advanced state, observation time, and a freshly drawn nonce distinct from the authorization nonce.

**INFRAATTEST-22** If a receipt reuses the authorization nonce, or differs from the expected authorization digest, plan digest, source, toolchain, state, nonce, or canonical encoding, the package shall reject with `receipt-mismatch`.

### Public-safe failure

**INFRAATTEST-23** If any precondition fails, the package shall surface a single stable `Code` drawn from the fixed rejection catalog, and shall not expose plan, inventory, backend, state, migration, provider, key, or lineage content through claims, return values, error strings, or logs.

**INFRAATTEST-24** If a plan, evidence document, toolchain manifest, or receipt carries a field this contract does not explicitly classify, the package shall withhold routine authorization rather than ignore the field.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`
- Governing contract: `infra/SPEC.md` (`INFRA-ATTEST-01` .. `INFRA-ATTEST-21`)
- Package tests: `internal/infraattest/*_test.go`
