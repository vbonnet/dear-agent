# Engram Document Store Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/internal/document` defines Engram's stateless knowledge layer:
immutable, versioned, authored reference documents such as specs,
architecture notes, research findings, and ADRs. It is deliberately separate
from mutable extracted memories so canonical knowledge is not degraded by
decay, consolidation, or partial updates.

The filesystem store is the reference implementation. It writes each document
version to its own JSON file under `{root}/{namespace...}/{id}/vN.json`, making
append-only history the default storage shape.

## EARS Requirements

**EDO-01** When a document kind is validated, the system shall accept only `spec`, `architecture`, `research`, `reference`, and `adr` as recognized document kinds.

**EDO-02** When content is hashed, the system shall compute the lowercase hexadecimal SHA-256 digest of the exact content bytes.

**EDO-03** When a filesystem store is created with an empty root, the system shall reject the store configuration.

**EDO-04** When a document is stored, the system shall reject empty namespaces and namespace components that can escape the store root.

**EDO-05** When a document is stored, the system shall reject empty document IDs and IDs containing path traversal or path separator characters.

**EDO-06** When a document is stored with an unknown non-empty kind, the system shall reject the document before writing a version file.

**EDO-07** When a document is stored, the system shall assign the next monotonic version number, set the current schema version, copy the requested namespace, compute the content hash, and stamp `CreatedAt` when the caller omitted it.

**EDO-08** When a document is stored for an ID that already has versions, the system shall append a new version file and shall not modify older version files.

**EDO-09** When the latest document is requested, the system shall return the highest stored version and shall return `ErrNotFound` when no version exists.

**EDO-10** When a specific version is requested, the system shall reject non-positive version numbers and shall return `ErrNotFound` for missing version files.

**EDO-11** When versions are listed, the system shall return valid versions oldest-first and skip unreadable or invalid version files.

**EDO-12** When documents are listed in a namespace, the system shall return the latest version of each matching document sorted by ID and shall apply kind, case-insensitive title, and positive limit filters.

**EDO-13** When a document is deleted, the system shall remove every version for that document ID and shall return `ErrNotFound` if the document directory does not exist.

**EDO-14** When any store operation receives a canceled context, the system shall return the context error before performing filesystem work.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_knowledge_guardrails.feature`
- Package tests: `engram/internal/document/document_test.go`
- Package tests: `engram/internal/document/fsstore_test.go`

