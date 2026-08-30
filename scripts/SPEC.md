# Repository Script Specification

<!-- Last audited at: 2026-07-20 -->

## EARS Requirements

**REPO-SCRIPT-01** When a repository maintenance script runs, the system shall validate its prerequisites and operate only on the declared repository scope.

**REPO-SCRIPT-02** If a build, synchronization, installation, or verification step fails, the system shall preserve the failure and avoid a false success message.

**REPO-SCRIPT-03** When local preflight runs repository tests, the system shall use the same explicit test timeout as required CI.

**REPO-SCRIPT-05** When the GOBIN guard or its independent freshness auditor observes a degraded state without a non-linked regular alarm marker, the system shall attempt its durable-trail and active-notification channels; when either channel succeeds and the configured marker path has owner-repairable, non-linked directory components and an absent leaf, the system shall create an owner-private marker beneath owner-traversable directories and suppress later serialized delivery attempts while that non-linked regular marker remains. Configured alarm, heartbeat, and trail paths beginning with `-` shall be treated as relative path operands rather than utility options. Every degraded invocation shall exit non-zero, and any pre-existing non-regular, linked, unsafe, or otherwise unwriteable marker path shall fail closed without opening or mutating that leaf or its linked target.

**REPO-SCRIPT-06** When full local preflight skips wall-clock performance assertions under race instrumentation, the system shall re-run every affected package with inherited Go test modes neutralized and explicit ordinary non-race, non-short settings before publication succeeds.

**REPO-SCRIPT-07** When full local preflight cannot resolve a required Go-installed security tool from `PATH`, the system shall inspect a configured `GOBIN` first and shall use the first `GOPATH` entry's `bin` directory only when `GOBIN` is empty, so the documented `go install` remediation produces a discoverable executable.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`
