# A2A Bead Linkage Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/a2a/beads` validates file references embedded in bead
descriptions and reads or updates existing channel metadata so coordination
records stay grounded in repository facts.

## EARS Requirements

**A2A-BEAD-01** When a bead description references repository files, the system shall extract likely code and documentation paths while ignoring URLs and common domain suffixes.

**A2A-BEAD-02** When validation runs against a repository root, the system shall mark the description invalid for every extracted path that does not exist under that root.

**A2A-BEAD-03** When formatting a failed validation result, the system shall include each invalid file reference and a warning count.

**A2A-BEAD-04** When bead existence is checked, the system shall search project files, global files, and known bd-backed stores without invoking a command shell.

**A2A-BEAD-05** When channel metadata is extracted, the system shall return key-value fields from the fenced metadata header and ignore body content.

**A2A-BEAD-06** When existing channel metadata is updated, the system shall replace the fenced header while preserving the channel body.

**A2A-BEAD-07** When linked bead metadata is requested, the system shall return the existing `Bead-ID`, return an empty identifier when no link exists, and diagnose a missing channel.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/beads/validator_test.go`
- Package tests: `agm/internal/a2a/beads/linker_test.go`
