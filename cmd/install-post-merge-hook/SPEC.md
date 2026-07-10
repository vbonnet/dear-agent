# Post-merge Hook Installer Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/install-post-merge-hook` installs the reviewed repository post-merge hook
into Git's effective hook directory without overwriting foreign or
configuration-managed artifacts.

## EARS Requirements

**PMI-01** When Git defines `core.hooksPath`, the installer shall resolve absolute, home-relative, and repository-relative values according to Git semantics.

**PMI-02** When no custom hook path is configured, the installer shall resolve Git's repository hook path.

**PMI-03** When chezmoi manages the target hook directory or file, the installer shall refuse a direct write and print configuration-managed installation guidance.

**PMI-04** When an identical post-merge hook is already installed, the installer shall report that it is current and perform no write.

**PMI-05** When a different post-merge hook exists, the installer shall refuse to overwrite it and shall provide composition guidance.

**PMI-06** When a new hook is installed, the installer shall create the target directory and write the reviewed source with executable permissions.

**PMI-07** When repository or source-hook discovery fails, the installer shall return a failure without writing an alternate location.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `cmd/install-post-merge-hook/*_test.go`
