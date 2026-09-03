# Claude OAuth Token Refresher Specification

## Overview

`cmd/token-refresher` is the single-owner refresh adapter for Claude Code OAuth
credentials. It does not stand in for OpenAI, Gemini, OpenRouter, or other
provider credential handling.

## EARS Requirements

**CTR-01** When a token freshness operation runs, the command shall hold the cross-process credential lock across read, exchange, and atomic persistence.

**CTR-02** When check mode is selected, the command shall report status without network access or mutation.

**CTR-03** When a token remains fresh, the command shall return it without spending the refresh token.

**CTR-04** When force mode is selected, the command shall attempt refresh even when the access token is still fresh.

**CTR-05** When refresh succeeds, stdout shall contain only the access token and logs shall remain on stderr.

**CTR-06** When the OAuth server reports `invalid_grant`, the command shall report a dead token family and return exit code 2.

**CTR-07** When server refresh succeeds but rotated credentials cannot be persisted, the command shall report a critical non-persistence failure and return exit code 3.

**CTR-08** When audit evidence is written, the command shall record mode, outcome, freshness, expiry metadata, and errors without recording token values.

**CTR-10** When a refresh request is transmitted but no usable response is received, the command shall treat the refresh token as possibly spent, quarantine it, and return exit code 4.

**CTR-11** When the refresh token on disk matches an active quarantine, the command shall decline to present it to the OAuth server and return exit code 4.

**CTR-12** When the refresh token on disk differs from the quarantined fingerprint, or a refresh succeeds, the command shall clear the quarantine automatically.

**CTR-13** When cadence mode encounters a quarantine, the command shall alert the operator once per episode and still exit 0 so launchd retains the schedule.

**CTR-14** When a quarantine marker exists but cannot be read or parsed, the command shall decline to present the refresh token and return exit code 4.

**CTR-15** When a possibly-spent refresh token cannot be recorded in the quarantine marker, the command shall report a critical non-persistence failure and return exit code 3.

**CTR-16** When reporting status in check mode, the command shall treat a quarantine marker naming a token other than the one on disk as inactive, and shall not mutate it.

**CTR-17** When a server-successful refresh cannot persist rotated credentials, the system shall quarantine the on-disk refresh token before returning the non-persistence error, and every shared resolver entry point shall honor that quarantine.

**CTR-18** While a credential-scoped refresh-stop marker exists, the system shall prevent every shared resolver entry point from presenting that credential set's refresh token until an operator explicitly clears the stop.

**CTR-19** When cadence mode receives a non-persistence error, the system shall report that refreshing stopped only when the credential-scoped refresh-stop marker exists.

**CTR-20** When resolving credentials, the system shall apply the same store precedence as the Claude Code CLI, reading the macOS keychain item before the plaintext credentials file.

**CTR-21** When deriving the keychain item identity, the system shall use the CLI's own rules: the service name scoped by an explicitly configured config directory, and the account taken from `USER` falling back to the current OS user.

**CTR-22** When a refresh persists rotated credentials, the system shall mirror them into every other store the CLI may resolve, so the credential presented by the CLI is the credential that was refreshed.

**CTR-23** When a mirror to a secondary store fails after the file write succeeded, the system shall log the failure and preserve the successful refresh rather than quarantining a credential that persisted correctly.

**CTR-24** When the store the CLI reads holds no refresh token while the fallback store holds one, the system shall report the credential set as SHADOWED and name the operator re-authentication step, rather than reporting a successful refresh.

**CTR-25** When mirroring credentials, the system shall refuse to write a credential set that carries no refresh token, so a store holding a refreshable credential is never overwritten with an unrefreshable one.

**CTR-09** When provider credentials are not Claude Code OAuth credentials, the system shall use the corresponding provider or harness credential surface instead of this adapter.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Package tests: `cmd/token-refresher/*_test.go`
