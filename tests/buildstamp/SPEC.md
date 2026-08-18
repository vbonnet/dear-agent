# Governed Build Stamp Integration Requirements Specification (EARS)

<!-- Last audited at: 2026-08-08 -->

**Version**: 1.0
**Status**: Active
**Scope**: Executable verification of the root Makefile build-provenance seam.

## EARS Requirements

**BUILDSTAMP-01** When the governed build-stamp contract is tested, the system shall build and execute a real probe that reports `pkg/version` provenance as structured data.

**BUILDSTAMP-02** When ordinary effective `GOFLAGS` are supplied through Make, the process, or persisted GOENV configuration, the system shall preserve their behavior while retaining all protected provenance values.

**BUILDSTAMP-03** When a top-level effective `GOFLAGS` field has the exact flag name `-ldflags` or `--ldflags`, the system shall reject the competing ingress before creating the requested artifact.

**BUILDSTAMP-04** When unpatterned optional linker flags conflict with Version, GitCommit, BuildDate, or BuiltBy, the system shall retain every Make-owned protected value.

**BUILDSTAMP-05** When caller build values contain Make or shell syntax that is valid within the linker grammar, the system shall preserve that syntax as opaque data without evaluating it.

**BUILDSTAMP-06** When protected provenance contains a Go linker whitespace separator or quote delimiter, the system shall reject it before creating the requested artifact.

**BUILDSTAMP-07** When optional linker flags use Go package-pattern form, the system shall reject them before the pattern can scope protected stamps away from the target package.

**BUILDSTAMP-08** When callers attempt to override private Make variables, the system shall retain the protected package identity, flag policy, and runtime values.

**BUILDSTAMP-09** When the root Makefile is inspected, the system shall require every explicit `go build -o` recipe to use the shared `BUILD_STAMP_FLAGS` seam and require its owning target to register the GOFLAGS guard prerequisite.

**BUILDSTAMP-10** When the canonical AGM installation target is executed with ordinary caller `GOFLAGS`, the system shall install both companion binaries, expose all protected stamps from their build metadata, and pass the installed AGM version and reaper revision runtime checks.

**BUILDSTAMP-11** When a quoted `-toolexec` GOFLAGS field contains an argument beginning with `-ldflags-helper`, the system shall build and execute the probe and prove the requested tool wrapper ran.

**BUILDSTAMP-12** When a leading quoted GOFLAGS field is malformed, the system shall reject it before creating the requested artifact without misclassifying an inner wrapper argument as linker ingress.

**BUILDSTAMP-13** When whitespace-only direct GOFLAGS accompanies persisted GOENV linker flags, the system shall treat the direct value as effective and retain protected provenance.

**BUILDSTAMP-14** When BDD verifies the canonical AGM installation topology, the system shall render a non-executing plan that includes the build-stamp guard, both stamped companion builds, and both companion installations.

**BUILDSTAMP-15** When default provenance is tested, the system shall distinguish clean and detached HEADs from tracked or untracked dirty worktrees and shall represent Git or status failure as `unknown`.

**BUILDSTAMP-16** When a caller supplies `GIT_COMMIT`, the governed build shall preserve the supplied value even when default Git discovery is unavailable.

**BUILDSTAMP-17** When AGM-family CI, cross-platform CI, or local preflight build entrypoints are inspected, the system shall require all four canonical `pkg/version` linker assignments and reject every obsolete `main` package assignment.

## BDD Traceability

- Package tests: `tests/buildstamp/*_test.go`
- BUILDSTAMP-17: `TestAGMFamilyBuildEntrypointsUseSharedVersionPackage`
- Runtime value contract: `pkg/version/SPEC.md`
- BDD: `agm/test/bdd/features/hook_parity.feature`
- Test consequence: Deterministic unit test `TestAGMFamilyBuildEntrypointsUseSharedVersionPackage` scans the exact CI, cross-platform CI, and local preflight entrypoints and fails when any canonical linker field is absent or any obsolete `main` package assignment remains; no new BDD feature is required for this repository-text contract.
