# External PR Reviewer Command Specification

<!-- Last audited at: 2026-08-17 -->

## Overview

`cmd/external-pr-reviewer` is the command-line surface for the external pull
request review poller. It parses operator flags, builds one
`internal/prreviewer` configuration, and drives either a single review pass or a
supervised polling loop. All review detection, provider dispatch, and posting
behaviour lives in `internal/prreviewer`; this command owns only argument
handling, loop control, and process exit codes.

## EARS Requirements

**XPR-01** When command-line flags cannot be parsed, the command shall exit with status 2 without contacting GitHub or any review provider.

**XPR-02** When a positional argument is supplied, the command shall report the unexpected argument and exit with status 2.

**XPR-03** When the `--repo` flag is repeated, the command shall accumulate every target repository in the order supplied, and shall reject an empty repository value.

**XPR-04** When a provider command is supplied as a single string, the command shall split it into an argument vector that preserves single- and double-quoted arguments, and shall reject an unbalanced quote with status 2.

**XPR-05** When a review event is supplied, the command shall normalize it to trimmed upper case before configuring the review pass.

**XPR-06** When `--watch` is not set, the command shall perform exactly one review pass and exit with status 0 on success.

**XPR-07** When `--watch` is set, the command shall repeat review passes and wait the configured interval between passes.

**XPR-08** When `--watch` is set with a non-positive interval, the command shall reject the configuration with status 2 rather than polling without pause.

**XPR-09** When SIGINT or SIGTERM is received, the command shall cancel the review pass, stop the polling loop, and exit with status 0.

**XPR-10** When a review pass returns an error that is not the shutdown cancellation, the command shall report the error on standard error and exit with status 1.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/external-pr-reviewer/*_test.go`
