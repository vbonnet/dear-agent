# Wayfinder Resume Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`wayfinder/cmd/wayfinder-session/internal/resume` detects whether a target
directory should start a fresh Wayfinder project, resume existing status, create
a replacement project elsewhere, or abort without writes. It protects resume
flows from ambiguous files, permission failures, path traversal, and symlink
overwrite attacks.

## EARS Requirements

**WAYFINDER-RESUME-01** When a directory is scanned, the system shall ignore hidden entries and directories and shall classify only visible files.

**WAYFINDER-RESUME-02** When directory scanning fails because of permissions, the system shall return a permission-denied resume error that names the directory.

**WAYFINDER-RESUME-03** When visible files are empty, the system shall classify the directory as empty and allow the caller to continue without prompting.

**WAYFINDER-RESUME-04** When visible files contain only W0 charter files, only `WAYFINDER-STATUS.md`, or both, the system shall classify the directory as resumable.

**WAYFINDER-RESUME-05** When visible files include anything beyond W0 charter and status files, the system shall classify the directory as non-resumable and shall require force or another directory.

**WAYFINDER-RESUME-06** When resuming from W0-only state, the system shall create status, mark W0 complete, and advance the current phase to D1.

**WAYFINDER-RESUME-07** When resuming from status-backed state, the system shall load existing status and set the current phase to the next incomplete phase when one can be determined.

**WAYFINDER-RESUME-08** When the user chooses to create a project in a different location, the system shall reject empty names, path traversal, absolute-path separators, and backslash separators in the project name.

**WAYFINDER-RESUME-09** When the user aborts, the system shall return the user-aborted error and shall not create or update status.

**WAYFINDER-RESUME-10** When a file path is checked before writing, the system shall reject symlinks and allow absent files.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_lifecycle_guardrails.feature`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/resume/actions_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/resume/detector_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/resume/prompt_test.go`

