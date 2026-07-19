# Sandbox provider specification

<!-- Last audited at: 2026-07-17 -->

## Executable EARS requirements

**SNDBR-01** When a provider creates a sandbox, the system shall return the actual provider identity and workspace path used by the caller.

**SNDBR-02** When automatic Linux selection finds `bwrap`, the system shall recommend the `bubblewrap` provider before OverlayFS.

**SNDBR-03** When automatic selection recommends an unregistered provider, the system shall return an unsupported-provider error rather than claim isolation succeeded.

**SNDBR-04** When provider creation fails, the system shall return the creation error; cleanup of provider-internal allocations that were not returned as a tracked sandbox is best-effort rather than guaranteed.

**SNDBR-05** When a tracked sandbox is destroyed, the system shall remove only resources owned by that sandbox instance.

**SNDBR-06** When a provider or caller does not consume a `SandboxSpec` field, the system shall not document that field as an enforced isolation control.

**SNDBR-07** The system shall not describe symlink-populated source directories as copy-on-write isolation.

## BDD traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

## Executable owners

- `internal/sandbox/factory.go`
- `internal/sandbox/provider.go`
- `internal/sandbox/types.go`
- `internal/sandbox/spec.go`
- `agm/cmd/agm/new_sandbox.go`
