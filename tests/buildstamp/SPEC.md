# Governed Build Stamp Integration Requirements Specification (EARS)

<!-- Last audited at: 2026-08-03 -->

**Version**: 1.0
**Status**: Active
**Scope**: Executable verification of the root Makefile build-provenance seam.

## EARS Requirements

**BUILDSTAMP-01** When the governed build-stamp contract is tested, the system shall build and execute a real probe that reports `pkg/version` provenance as structured data.

**BUILDSTAMP-02** When ordinary effective `GOFLAGS` are supplied through Make, the process, or persisted GOENV configuration, the system shall preserve their behavior while retaining all protected provenance values.

**BUILDSTAMP-03** When effective `GOFLAGS` contains `-ldflags` or `--ldflags`, the system shall reject the competing ingress before creating the requested artifact.

**BUILDSTAMP-04** When unpatterned optional linker flags conflict with Version, GitCommit, BuildDate, or BuiltBy, the system shall retain every Make-owned protected value.

**BUILDSTAMP-05** When caller build values contain Make or shell syntax that is valid within the linker grammar, the system shall preserve that syntax as opaque data without evaluating it.

**BUILDSTAMP-06** When protected provenance contains a Go linker whitespace separator or quote delimiter, the system shall reject it before creating the requested artifact.

**BUILDSTAMP-07** When optional linker flags use Go package-pattern form, the system shall reject them before the pattern can scope protected stamps away from the target package.

**BUILDSTAMP-08** When callers attempt to override private Make variables, the system shall retain the protected package identity, flag policy, and runtime values.

**BUILDSTAMP-09** When the root Makefile is inspected, the system shall require every explicit `go build -o` recipe to use the shared `BUILD_STAMP_FLAGS` seam.

**BUILDSTAMP-10** When the canonical AGM installation target is executed with ordinary caller `GOFLAGS`, the system shall install both companion binaries, expose all protected stamps from their build metadata, and pass the installed AGM version and reaper revision runtime checks.

## Test Traceability

- Package tests: `tests/buildstamp/*_test.go`
- Runtime value contract: `pkg/version/SPEC.md`
- BDD: `agm/test/bdd/features/hook_parity.feature`
