# Claude Credential Monitor Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/credential-monitor` is a read-only monitor for Claude Code's shared OAuth
credential file. Other harness and provider credentials use their native
status or shared provider-security surfaces rather than this adapter.

## EARS Requirements

**CRM-01** When credential freshness is checked, the command shall perform no network request and shall not mutate the credential file.

**CRM-02** When no credential path is supplied, the Claude adapter shall inspect `~/.claude/.credentials.json`.

**CRM-03** When a token is fresh, expiring, expired, or missing, the command shall return the distinct documented state and exit code.

**CRM-04** When a stale window is configured, the command shall classify tokens expiring within that duration as stale.

**CRM-05** When JSON output is selected, the command shall emit state, stale flag, exit code, refresh-token presence, stale window, note, and optional expiry.

**CRM-06** When a decision-trail path is configured and credentials are stale, the command shall append a stale event without including token values.

**CRM-07** When decision-trail persistence fails, the command shall warn without changing the credential-state exit code.

**CRM-08** When flags are invalid, the command shall return the usage code rather than a credential-state code.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Package tests: `cmd/credential-monitor/*_test.go`
