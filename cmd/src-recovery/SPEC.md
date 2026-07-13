# Golden Source Recovery Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/src-recovery` is the narrowly scoped, audited exception for restoring a
golden `~/src` checkout or removing a provably stale Git index lock.

## EARS Requirements

**SRC-01** When a repository path is supplied, the command shall require it to be strictly contained beneath the current user's `~/src` directory.

**SRC-02** When recovery is requested, the command shall allow only stash-if-dirty, default-branch checkout, and fast-forward pull operations.

**SRC-03** When a repository has unresolved conflicts, the command shall refuse recovery rather than hiding or discarding them.

**SRC-04** When dry-run recovery is selected, the command shall report the planned branch and stash behavior without running mutating Git operations.

**SRC-05** When recovery runs, the command shall apply the configured timeout to remote synchronization and shall not permit pass-through Git arguments.

**SRC-06** When unlock is requested, the command shall derive `.git/index.lock` itself and shall remove it only after the minimum age and open-holder checks pass.

**SRC-07** When unlock dry-run is selected, the command shall report the stale lock without deleting it.

**SRC-08** When recovery or unlock makes a decision, the command shall append private timestamped evidence to the source-recovery audit log.

**SRC-09** When a mutating step fails, the command shall stop the sequence and return recovery guidance without using destructive reset or force operations.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `cmd/src-recovery/*_test.go`
