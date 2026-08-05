# Wayfinder Archive Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`wayfinder/cmd/wayfinder-session/internal/archive` snapshots Wayfinder project
state before rewind or other destructive lifecycle moves. Archives live under
`.wayfinder/archives`, keep status and optional history files together, and are
listed without requiring callers to know the on-disk archive naming convention.

## EARS Requirements

**WAYFINDER-ARCHIVE-01** When an archive manager is created, the system shall bind all archive operations to the supplied project directory.

**WAYFINDER-ARCHIVE-02** When a phase is archived, the system shall create a timestamped, collision-resistant directory under `.wayfinder/archives` with the canonical phase name as the prefix.

**WAYFINDER-ARCHIVE-03** When a phase is archived, the system shall copy `WAYFINDER-STATUS.md` into the archive and shall fail if the status file cannot be read or written.

**WAYFINDER-ARCHIVE-04** When `WAYFINDER-HISTORY.jsonl` exists, the system shall copy it into the same archive directory as the status snapshot.

**WAYFINDER-ARCHIVE-05** When `WAYFINDER-HISTORY.jsonl` does not exist, the system shall still complete the archive if the status snapshot succeeds.

**WAYFINDER-ARCHIVE-06** When archives are listed and the archive root does not exist, the system shall return an empty archive list rather than an error.

**WAYFINDER-ARCHIVE-07** When archives are listed, the system shall return directory entries only and shall skip entries that cannot be statted.

**WAYFINDER-ARCHIVE-08** When archive files are written, the system shall use owner-only file permissions for copied snapshots.

**WAYFINDER-ARCHIVE-09** When an archive snapshot completes, the system shall publish its final directory by rename and return a typed project-relative archive reference; when construction fails, the system shall remove its temporary snapshot when possible.

**WAYFINDER-ARCHIVE-10** When `RETRO-retrospective.md` exists, the system shall require it to be a regular file and shall copy its pre-rewind contents into the archive before publishing the snapshot.

**WAYFINDER-ARCHIVE-11** When the phase name is non-canonical, a canonical snapshot path is non-regular, or `.wayfinder` or its archive root is a symbolic link, the system shall reject archive creation or listing rather than reading or publishing through the unowned path.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_lifecycle_guardrails.feature`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/archive/archive_test.go`
