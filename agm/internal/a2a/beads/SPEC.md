# A2A Bead Linkage Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/beads` links A2A channels to Beads work items and validates
file references embedded in bead descriptions so coordination metadata stays
grounded in repository facts.

## EARS Requirements

**A2A-BEAD-01** When a bead description references repository files, the system shall extract likely code and documentation paths while ignoring URLs and common domain suffixes.

**A2A-BEAD-02** When validation runs against a repository root, the system shall mark the description invalid for every extracted path that does not exist under that root.

**A2A-BEAD-03** When formatting a failed validation result, the system shall include each invalid file reference and a warning count.

**A2A-BEAD-04** When linking a channel to a bead, the system shall require the active channel file and a bead that exists in project, global, or bd-backed storage.

**A2A-BEAD-05** When a channel is linked to a bead, the system shall write `Bead-ID` and `Bead-Link` metadata into the channel header.

**A2A-BEAD-06** When a channel is unlinked from a bead, the system shall remove bead metadata while preserving the rest of the channel header and body.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
- Package tests: `agm/internal/a2a/beads/validator_test.go`
- Package tests: `agm/internal/a2a/beads/linker_test.go`
