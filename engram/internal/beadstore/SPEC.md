# Engram Bead Store Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`engram/internal/beadstore` is the bd-backed persistence boundary for Engram
MCP bead tools. It replaces the legacy JSONL-only write path that could report
successful `beads_create` calls without creating rows in the bd/dolt database
used by the rest of the system.

The package is deliberately small: it shells out to `bd` with an explicit
database path, verifies successful writes by reading the created bead back from
the same store, and backfills legacy JSONL rows through the same verified write
path.

## EARS Requirements

**BST-01** When a bead store is used without a configured database path, the system shall return a hard configuration error and shall not write to a fallback store.

**BST-02** When the store invokes bd, the system shall pass `--db <path>` on every command invocation.

**BST-03** When the store invokes bd, the system shall bound the command execution time with a context timeout.

**BST-04** When a create request omits title or description, the system shall reject the request before invoking bd.

**BST-05** When a create request has priority outside 0 through 4 or a negative estimate, the system shall reject the request before invoking bd.

**BST-06** When bd acknowledges a create, the system shall parse a non-empty bead ID from the create output before treating the write as acknowledged.

**BST-07** When bd acknowledges a create with a bead ID, the system shall read that bead back from the same configured store before returning success.

**BST-08** When read-after-write verification cannot find the acknowledged bead, the system shall return a hard error that identifies the bead ID and store path.

**BST-09** When bd show returns an error payload with a zero exit code, the system shall treat the bead as missing instead of accepting the command exit status as proof.

**BST-10** When the store lists beads, the system shall parse bd JSON output and shall fail on unparseable list output.

**BST-11** When legacy JSONL reconciliation reads malformed legacy lines, the system shall skip malformed lines and continue processing well-formed beads.

**BST-12** When reconciliation encounters a closed legacy bead, an already-backfilled legacy ID, or an existing bead with the same title, the system shall skip that legacy bead without creating a duplicate.

**BST-13** When reconciliation runs in dry-run mode, the system shall report which open legacy beads would be created without invoking the write path.

**BST-14** When reconciliation backfills a legacy bead, the system shall create it through `VerifiedCreate` and shall label it with `backfill-src:<legacy-id>`.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
- Package tests: `engram/internal/beadstore/store_test.go`
- Package tests: `engram/internal/beadstore/reconcile_test.go`

