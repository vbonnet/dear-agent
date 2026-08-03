# Governed Build GOFLAGS Guard Specification

<!-- Last audited at: 2026-08-03 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/buildstamp`.

## Overview

`internal/buildstamp` is the non-shipped admission guard for root-Makefile
builds that carry protected provenance. It resolves effective GOFLAGS without
interpolating caller text into a shell, parses the leading-quote field grammar
used by the pinned Go toolchain, and rejects only a true top-level linker flag.

## EARS Requirements

**BUILDSTAMP-GUARD-01** When direct GOFLAGS is byte-non-empty, the guard shall parse that exact value and shall not consult persisted GOENV configuration.

**BUILDSTAMP-GUARD-02** When direct GOFLAGS is empty, the guard shall resolve persisted GOFLAGS with a shell-free `go env` child using the captured original GOENV setting.

**BUILDSTAMP-GUARD-03** When the guard parses GOFLAGS, the guard shall treat space, tab, line feed, and carriage return as separators and shall recognize a single or double quote only at the beginning of a field.

**BUILDSTAMP-GUARD-04** When a parsed top-level field has the exact flag name `-ldflags` or `--ldflags`, the guard shall reject the governed build and direct linker customization to `EXTRA_GO_LDFLAGS`.

**BUILDSTAMP-GUARD-05** When linker-shaped text appears only in a different top-level flag value or a longer flag name, the guard shall admit the GOFLAGS value for normal Go validation.

**BUILDSTAMP-GUARD-06** When a leading quoted field is unterminated, the guard shall reject the governed build without printing the caller's GOFLAGS value.

**BUILDSTAMP-GUARD-07** The guard shall not use caller GOFLAGS, persisted GOFLAGS, workspace selection, or cross-compilation dimensions while compiling its own transient bootstrap process.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- `internal/buildstamp/main_test.go` covers BUILDSTAMP-GUARD-01 through
  BUILDSTAMP-GUARD-06 and the isolated GOENV query used by
  BUILDSTAMP-GUARD-07.
- `tests/buildstamp/buildstamp_test.go` proves the Make prerequisite and real
  Go toolchain preserve a quoted `-toolexec` field while rejecting actual
  linker ingress before producing the requested artifact.
