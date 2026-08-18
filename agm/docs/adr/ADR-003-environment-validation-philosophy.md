# ADR-003: Validate harness prerequisites without owning them

Status: Accepted (2026-01-19; verified 2026-07-17)

## Context

AGM needs actionable failures when a harness binary, authentication, config, or
runtime dependency is unavailable. Installing tools or storing provider secrets
would make session management responsible for host configuration and credential
lifecycle.

## Decision

AGM validates registered harness names, executable availability, authentication
signals, paths, tmux, and model choices before launch. Validation returns
specific remediation guidance but does not install harnesses, rewrite shell
configuration, or persist credentials.

Harness-specific checks live behind the agent registry. General input safety
lives in `agm/internal/validate`.

## Alternatives

Automatic installation is convenient but expands privilege and supply-chain
scope. Deferring every error to process launch produces late, opaque failures.

## Consequences

Operators own their host environment. AGM owns early, consistent diagnostics.
Tests under `agm/internal/agent` and `agm/internal/validate` verify the contract.
