# Native OverlayFS Sandbox Provider Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The native OverlayFS provider is a deprecated Linux compatibility provider kept
for existing callers that use the historical `internal/sandbox/overlayfs`
subpackage. It creates a kernel OverlayFS mount with read-only lower
directories, writable upper and work directories, and a merged path where agents
operate.

## Requirements

**OVERLAYFS-01** When the provider is queried for its name, the system shall return `overlayfs-native`.

**OVERLAYFS-02** When creating a sandbox, the system shall reject requests with an empty session ID, no lower directories, missing lower directories, or an empty workspace directory using structured sandbox errors.

**OVERLAYFS-03** When the Linux kernel version cannot be read or is below 5.11, the system shall fail creation with a kernel-too-old sandbox error before mounting.

**OVERLAYFS-04** When a valid sandbox request is created, the system shall create `upper`, `work`, and `merged` directories under the requested workspace directory.

**OVERLAYFS-05** When mounting the overlay filesystem, the system shall use the requested lower directories, the sandbox upper and work directories, the sandbox merged directory, and the `xino=auto` option.

**OVERLAYFS-06** When mount execution reports permission denial, the system shall surface a mount-permission sandbox error.

**OVERLAYFS-07** When secrets are provided, the system shall write them only to `upper/.env` with owner-only permissions and shell environment expansion.

**OVERLAYFS-08** When a sandbox is destroyed, the system shall unmount the merged directory, verify unmount best-effort, remove sandbox directories, and remove the sandbox from the active provider registry.

**OVERLAYFS-09** When a sandbox is validated, the system shall require an active registry entry, an existing merged path, and a corresponding mount entry in `/proc/mounts`.

**OVERLAYFS-10** When a request names a working directory inside a lower directory, the provider shall return `merged/<relative-directory>` as the harness working directory.

**OVERLAYFS-11** When multiple lower directories contain colliding paths and a sandbox request names one repository as its working directory, the provider shall give that matched repository overlay precedence while preserving the order of all remaining lower directories.

## BDD Traceability

- `agm/test/bdd/features/sandbox_provider_guardrails.feature` enforces that this
  package keeps co-located SPEC coverage and that the SPEC points back to the
  executable guardrail.
