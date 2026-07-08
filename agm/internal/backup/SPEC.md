# AGM Backup Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/backup` protects AGM and Claude session state through numbered
file backups and compressed session archives. It favors recoverability: restore
operations snapshot the current file before overwriting it, and session
archives are constrained to local session and backup directories.

## Requirements

**AGM-BACKUP-01** When a numbered backup is created, the system shall choose the next numeric suffix after existing backups for the source file.

**AGM-BACKUP-02** When a numbered backup is written, the system shall copy the source file content with owner-only file permissions.

**AGM-BACKUP-03** When numbered backups exceed the retention limit, the system shall delete the oldest backups until at most `MaxBackups` remain.

**AGM-BACKUP-04** When a numbered backup is restored over an existing source file, the system shall first create a backup of the current source state.

**AGM-BACKUP-05** When a session backup is created, the system shall archive the requested session directory into `~/.agm/backups/sessions`.

**AGM-BACKUP-06** When a session manifest name is available, the system shall use a sanitized form of that name in the backup filename.

**AGM-BACKUP-07** When a session manifest is unavailable or empty, the system shall fall back to the session ID for the backup filename.

**AGM-BACKUP-08** When session backups are listed, the system shall return only `.tar.gz` backup files with parseable timestamps and include path, size, timestamp, and session name.

**AGM-BACKUP-09** When a session archive is restored, the system shall reject archive entries that attempt directory traversal.

**AGM-BACKUP-10** When extracting session archives, the system shall create directories and regular files, skip unsupported entry types, and limit regular file extraction to the configured per-file size bound.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
