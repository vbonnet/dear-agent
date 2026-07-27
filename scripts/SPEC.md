# Repository Script Specification

<!-- Last audited at: 2026-07-20 -->

## EARS Requirements

**REPO-SCRIPT-01** When a repository maintenance script runs, the system shall validate its prerequisites and operate only on the declared repository scope.

**REPO-SCRIPT-02** If a build, synchronization, installation, or verification step fails, the system shall preserve the failure and avoid a false success message.

**REPO-SCRIPT-03** When local preflight runs repository tests, the system shall use the same explicit test timeout as required CI.

**REPO-SCRIPT-05** When the GOBIN guard runs and the Go toolchain bin directory or its sentinel binary is absent, the system shall append an escalation record to the decision trail and exit non-zero.

**REPO-SCRIPT-06** When full local preflight skips wall-clock performance assertions under race instrumentation, the system shall re-run every affected package with inherited Go test modes neutralized and explicit ordinary non-race, non-short settings before publication succeeds.

**REPO-SCRIPT-07** When full local preflight cannot resolve a required Go-installed security tool from `PATH`, the system shall inspect a configured `GOBIN` first and shall use the first `GOPATH` entry's `bin` directory only when `GOBIN` is empty, so the documented `go install` remediation produces a discoverable executable.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`
