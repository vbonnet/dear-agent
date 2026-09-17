# Sandbox Onboarding Installation Specification

<!-- Last audited at: 2026-09-17 -->

## Overview

`agm/internal/sandboxonboarding` owns the one-shot rendering and authenticated
installation of Claude sandbox onboarding instructions beneath the retained
runtime HOME.

## Security Boundary

This contract rejects pre-existing hostile namespace entries and replacements
observed by its adjacent pre-commit checks. It does not isolate one process
from another process already running as the same effective UID: portable
Darwin/Linux rename and unlink operations provide no inode-conditional
compare-and-swap, and that peer can also modify the output after return.
"Atomic" below means readers observe either the old complete file or the new
complete file, not linearizable commit against a hostile same-UID writer.

## EARS Requirements

**AGM-SANDBOX-ONBOARDING-01** When onboarding is installed, the module shall derive repository display paths and the Claude project key from its request, render the selected built-in or configured template, and ignore later ambient HOME changes.

**AGM-SANDBOX-ONBOARDING-02** When resolving the output beneath retained HOME, the module shall traverse with held directory handles, reject links, special nodes, foreign ownership, or group/world-writable directories, and create a missing suffix as private real directories.

**AGM-SANDBOX-ONBOARDING-03** When an existing project `CLAUDE.md` is an owner-owned safe regular file with one link, the module shall preserve its content after the new onboarding instructions; every other existing final-node type or unsafe mode shall fail before replacement.

**AGM-SANDBOX-ONBOARDING-04** When installation commits, the module shall replace the exact final name with a synced same-directory temporary regular file through one atomic rename and leave the final mode exactly `0600`.

**AGM-SANDBOX-ONBOARDING-05** If installation fails before commit, the module shall remove only transaction-created temporary files and empty directories whose no-follow identities still match, return cleanup failures with the primary failure, and never return a later fallible error after commit.

**AGM-SANDBOX-ONBOARDING-06** When onboarding installation runs on a platform without the authenticated Darwin or Linux implementation, the module shall fail before filesystem mutation.

## Test Traceability

- Feature: `agm/test/bdd/features/sandbox_onboarding_guardrails.feature`
- Feature: `agm/test/bdd/features/sandbox_onboarding_cli_guardrails.feature`
- Unit package: `agm/internal/sandboxonboarding`
- Command integration: `agm/cmd/agm/new_sandbox_test.go`
