# Portable SPEC Governance Package Specification

**Status:** Active
**Scope:** Harness-neutral staging and validation of the canonical SPEC
governance skills with one prebuilt `specaudit` executable.

This specification owns the closed staged distribution. It does not own native
loader metadata, trusted installation, catalog discovery, provider invocation,
running-image attestation, or maintainer approval.

## Terms

- **Distribution root:** The root later authenticated by a native loader or
  installer. This contract validates a private staged instance but does not
  authenticate its ancestry or bind it to a running process.
- **Package payload:** Exactly the `audit-specs` and `write-spec` skill
  entrypoints, their four canonical support documents, and `bin/specaudit`.
- **Receipt:** The deterministic record recomputed from payload bytes and
  logical metadata. Its manifest digest may be pinned by later installation.
- **Target repository input:** Caller-selected instructions, Git objects,
  specifications, BDD, and policies being read or audited. These are not
  distribution dependencies and do not authorize checkout search.
- **Retained failed root:** An originally allocated private path whose staging
  attempt failed. It is diagnostic, not a valid package receipt, and remains
  untouched until a separately authorized liveness-aware reaper handles it.
  Its identity is trusted only when the failure receipt explicitly says it was
  still verified at return time.
- **Handle-capture boundary:** Replacement rejection starts once an opened
  object identity has been captured and continues through return. Portable
  POSIX `mkdirat` cannot atomically return a directory handle, so the unique
  96-bit-random private root narrows but cannot eliminate the pre-capture
  interval against a hostile process running as the same user. The stager
  rechecks each retained parent immediately before mutation and rejects later
  observable changes, but POSIX cannot stop that same-UID process from
  reparenting an already-open inode after the check. This package is
  race-detecting, not a same-UID sandbox boundary.

## EARS Requirements

**SPEC-GOV-16** When an installed audit workflow invokes `specaudit`, the system shall select the executable only as the absolute `bin/specaudit` child of the loader-authenticated distribution root and shall not resolve executable code through process working directory, PATH, repository ancestry, a shell launcher, or a runtime compiler.

**SPEC-GOV-20** When SPEC governance skills are staged, the system shall export exactly `audit-specs` and `write-spec` with their four canonical package-relative support documents and shall reject every additional, missing, aliased, escaping, unresolved, nonregular, or wrongly named skill or support entry.

**SPEC-GOV-21** When the system stages a SPEC governance distribution, the system shall accept a caller-supplied prebuilt `specaudit` artifact, reject a staging parent whose handle ancestry contains the source root before allocation, create one unique private package root without replacing a caller-named destination, retain verified handles for every created directory and file, write each child through its retained parent handle, write the canonical manifest last, validate the completed root through the shared closure seam, and return the root only after every opened and visible identity is reverified.

**SPEC-GOV-22** When the system reads source or staged package content, the system shall use directory-handle-anchored no-follow and nonblocking inspection, require stable regular-file identity with one hard link, and reject symlinks, FIFOs, sockets, devices, unsafe modes, replacement races within the handle-capture boundary, and bounds violations without waiting on special-file content.

**SPEC-GOV-23** When the system validates a staged distribution, the system shall require the fixed package layout and exact canonical manifest bytes, shall recompute bytewise-sorted path, role, logical-mode, size, and SHA-256 receipts independently of absolute root, working directory, ownership, time, and enumeration order, and shall expose the resulting manifest digest without labelling self-consistency as authentication.

**SPEC-GOV-24** When the staged `specaudit` executable runs from a working directory outside the source, package, and target roots, the system shall perform real pinned inventory behavior without consulting a dear-agent checkout or a Go toolchain and shall retain runtime status `UNVERIFIED` in the source-audit artifact.

**SPEC-GOV-25** When package readiness is reported, the system shall distinguish source, build, staged-package, install, discovery, invocation, runtime, running-image, provider-visible, and human-approval evidence and shall not mark a native catalog available from an earlier layer's receipt.

**SPEC-GOV-26** If staging or validation encounters a missing manifest, content or metadata mutation, unresolved dependency, unexpected entry, unsafe filesystem identity, cancelled context, or bounded-resource failure, then the system shall return no successful package receipt and, within the documented handle-capture boundary, shall not modify source, target repository, sibling staging roots, installed package state, catalog state, or provider state, and shall leave every allocated path untouched rather than unlink through concurrently mutable pathnames; the system shall report the originally allocated path as diagnostic state, distinguish whether its original identity remained verified at return time, and reserve cleanup for a separately authorized liveness-aware operation.

**SPEC-GOV-27** If the package command successfully stages a distribution but cannot deliver its JSON receipt, then the system shall exit nonzero, leave the valid staged root untouched, and report that exact retained root on standard error for separately authorized lifecycle handling.

**SPEC-GOV-28** When package validation inspects skill Markdown, the system shall accept only the exact versioned payload bytes named by one closed per-file approval registry, and before accepting those bytes shall parse Markdown and JSON to require a closed static POSIX command grammar rooted at the authenticated distribution's `bin/specaudit`, decoded JSON `specaudit` values limited to the declared logical inventory identity or HTTP or HTTPS references, and closed package links; it shall reject every other payload mutation, dynamic or additional shell command, mutated flag or redirection, blockquoted, untyped, or indented executable context, checkout path, case or wrapper alias, unsupported code-block language, raw HTML, and unresolved or escaping package link without copying the approved skills' normative prose into the validator.

## Bounds

The implementation bounds file count, path depth and length, Markdown file
size, binary size, total package bytes, parser work, and context duration. A
ceiling breach is a fail-closed validation result rather than truncation.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
