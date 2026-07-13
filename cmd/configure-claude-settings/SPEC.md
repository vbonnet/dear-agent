# Claude Settings Adapter Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/configure-claude-settings` is the Claude Code native-settings adapter. It
manages Claude's JSON hook and plugin surfaces; shared policy remains in
canonical `AGENTS.md`, neutral hook manifests, and marketplace catalogs.

## EARS Requirements

**CCS-01** When no alternate file is supplied, the command shall operate on the current user's `~/.claude/settings.json`.

**CCS-02** When settings are read from a missing file, the command shall initialize an empty object rather than failing validation.

**CCS-03** When nested values are set or removed, the command shall traverse dot-separated object paths without corrupting sibling settings.

**CCS-04** When an array-append path is used, the command shall append the decoded JSON value to the selected array.

**CCS-05** When a hook or plugin is added repeatedly, the command shall remain idempotent and shall not create duplicate entries.

**CCS-06** When a hook is removed by command, the command shall remove only matching command entries from the selected event.

**CCS-07** When settings are written, the command shall create parent directories, format valid JSON, and persist through a temporary file replacement.

**CCS-08** When dry-run mode is selected, the command shall report intended changes without writing the settings file.

**CCS-09** When additional directories no longer exist, the command shall remove only stale entries and preserve existing paths.

**CCS-10** When shared instructions or policies are needed, the adapter shall not duplicate them in Claude settings in place of canonical cross-harness sources.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Package tests: `cmd/configure-claude-settings/*_test.go`
