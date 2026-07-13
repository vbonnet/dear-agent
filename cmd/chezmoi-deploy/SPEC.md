# Chezmoi Deployment Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/chezmoi-deploy` applies selected dotfile source changes, commits the source
repository, and publishes it through a non-forced credential-safe push.

## EARS Requirements

**CHD-01** When force or force-with-lease flags are supplied, the command shall reject them before running chezmoi or Git.

**CHD-02** When dry-run mode is selected, the command shall show the chezmoi diff and source status without applying, committing, or pushing.

**CHD-03** When deployment runs, the command shall apply chezmoi targets before staging source changes.

**CHD-04** When apply or staging fails, the command shall stop and report which earlier steps succeeded without continuing to commit or push.

**CHD-05** When no staged source changes remain after apply, the command shall report convergence and skip commit and push.

**CHD-06** When no commit message is supplied, the command shall derive a bounded message from the staged file list.

**CHD-07** When source changes are committed, the command shall use argv-based Git invocation without shell interpretation.

**CHD-08** When publishing the commit, the command shall resolve GitHub credentials through the shared safe-git helper and shall never force push.

**CHD-09** When publishing fails after commit, the command shall preserve the local commit and print a `safe-push` recovery command.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `cmd/chezmoi-deploy/*_test.go`
