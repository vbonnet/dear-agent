# AGM Freshness Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/freshness` verifies that running binaries correspond to the
expected source checkout and trunk ancestry. Freshness checks distinguish
definitive provenance failures, which must fail loudly, from infrastructure
indeterminacy, which fails open with an error.

## Requirements

**AGM-FRESHNESS-01** When a binary commit is empty or `unknown`, the simple freshness check shall mark the binary stale.

**AGM-FRESHNESS-02** When repo HEAD cannot be resolved during the simple freshness check, the system shall fail open with `Stale` false and the git error recorded.

**AGM-FRESHNESS-03** When comparing binary and repo commits, the system shall ignore a `-dirty` suffix and compare the common short-hash prefix.

**AGM-FRESHNESS-04** When trunk ancestry is verified, the system shall prefer `origin/HEAD` and fall back to `origin/main`.

**AGM-FRESHNESS-05** When a binary has no embedded VCS revision, the system shall return `AncestryUnknownCommit` from ancestry verification and classify it as fail-loud.

**AGM-FRESHNESS-06** When the trunk ref cannot be resolved, the system shall return `AncestryIndeterminate` from ancestry verification and classify it as fail-open.

**AGM-FRESHNESS-07** When the binary commit object is missing from the repository, ancestry verification shall return `AncestryCommitMissing` and classify it as fail-loud.

**AGM-FRESHNESS-08** When the binary commit is not an ancestor of trunk, the system shall return `AncestryNotAncestor` from ancestry verification and classify it as fail-loud.

**AGM-FRESHNESS-09** When the binary commit is an ancestor of trunk, the system shall return `AncestryVerified` from ancestry verification.

**AGM-FRESHNESS-10** When a Go binary is inspected for VCS revision, the system shall use `go version -m`, return a shortened revision, and append `-dirty` when the binary is marked modified.

**AGM-FRESHNESS-11** When source repository paths are resolved, the system shall honor the documented environment override before trying the known default path.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
