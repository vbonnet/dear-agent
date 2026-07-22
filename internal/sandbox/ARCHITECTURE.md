# Sandbox architecture

<!-- Last audited at: 2026-07-17 -->

The sandbox package defines a common provisioning interface and several
platform-specific workspace providers used by AGM session creation. Providers
materialize a session workspace and return its paths; command execution and
harness policy remain caller responsibilities.

## Creation flow

```text
agm new
   -> maybeProvisionSandbox
   -> choose configured provider or auto-detect
   -> Provider.Create(SandboxRequest)
   -> persist provider name, workspace root, and mapped working directory
   -> run the harness from the returned working directory
```

`agm/cmd/agm/new.go` blank-imports the platform provider packages so their
`init` functions populate the registry. Importing `internal/sandbox` alone does
not register those subpackages.

## Core contract

`Provider` has four operations:

- `Create` validates a request and provisions a workspace;
- `Destroy` removes resources tracked by that provider instance;
- `Validate` checks tracked workspace health;
- `Name` reports the implementation identity stored in manifests.

`SandboxRequest` supplies a session ID, lower/source directories, requested
working directory, workspace directory, optional secrets, network-sharing
request, and preferred target repository. `Sandbox` reports the workspace root,
provider-mapped harness working directory, upper path, and work path when
applicable. The provider owns this mapping seam because only its adapter knows
whether repositories are overlaid at the root, cloned under `repoN`, or
materialized as a selected worktree.

Provider instances keep in-memory ownership maps. Reconstructing a provider and
calling `Destroy` does not guarantee it can discover resources created by an
earlier instance; operational cleanup has separate package support and must use
explicit paths carefully.

## Automatic selection

`DetectPlatform` in `factory.go` is authoritative:

| Platform | Automatic recommendation |
|---|---|
| Linux with `bwrap` | `bubblewrap` |
| Linux without `bwrap`, kernel 5.11+ | `overlayfs` |
| Older Linux | `fuse-overlayfs` |
| macOS | `apfs` |
| Other | `fallback` |

`fuse-overlayfs` and `fallback` are not registered implementations. Automatic
selection therefore returns an unsupported-provider error on those branches.
Callers must surface that failure or select an available explicit provider; they
must not imply that a fallback sandbox was created.

## Providers

| Registry name | Platform | Current workspace strategy |
|---|---|---|
| `bubblewrap` | Linux host with `bwrap` | Create a git worktree when possible, validate namespace support, and use symlink population only when no repository is found. |
| `overlayfs`, `overlayfs-native` | Linux 5.11+ | Mount lower directories with an upper and work layer. |
| `gvisor` | Linux with `runsc` | Create a git worktree when possible and validate gVisor availability; callers own later command wrapping. |
| `apfs` | macOS | Clone source trees into an upper directory and expose it through a merged symlink. |
| `claudecode-worktree` | all | Create a workspace directory and map selected `SandboxSpec` values to Claude Code arguments; Claude Code owns actual worktree isolation. |
| `mock` | tests | In-memory lifecycle with configurable failures and delays. |

Bubblewrap and gVisor providers materialize a writable git worktree for the
target repository. Their fallback symlink layout is not equivalent to
copy-on-write isolation: writes through a symlink can reach the source. Callers
requiring a hard isolation guarantee must verify the provider and creation mode,
not infer it from the word “sandbox.”

## Provider-specific boundaries

### Bubblewrap

The provider checks `bwrap`, creates workspace directories, selects a target git
repository, and runs a self-test with network sharing controlled by the request.
Its returned metadata describes the materialized workspace. The provider does
not itself remain as a long-running command supervisor.

### OverlayFS

The Linux provider requires kernel 5.11 or newer, creates `upper`, `work`, and
`merged`, mounts the declared lower directories, and validates the mount through
`/proc/mounts`. Destroy unmounts before removing owned directories.

### gVisor

The provider checks `runsc` and materializes a workspace, but execution inside
an OCI bundle is a caller responsibility. Creation alone does not mean later
commands run under gVisor.

### APFS

The Darwin provider clones each lower directory under `upper` and makes
`merged` a symlink to that directory. It is not a union filesystem. Multiple
repositories remain separate children. The returned working directory points
through `merged/repoN` to the clone corresponding to the requested host path,
including any repository-relative subdirectory.

### Claude Code worktree

This provider creates directory metadata for a worktree-mode caller. It maps
allowed write directories and budget configuration to CLI arguments and exposes
tool preset data to callers. It does not directly enforce denied reads, domain
allowlists, timeouts, or tool restrictions.

## SandboxSpec

`SandboxSpec` is a declarative cross-provider shape with filesystem, network,
resource, and tool fields plus read-only, code-only, and full-access presets.
Support is not uniform. A field is effective only when the selected provider or
its caller explicitly consumes it. Adding a field to the struct is not an
isolation guarantee.

## Secrets

Filesystem providers may write request secrets to a provider-owned `.env` file.
Providers validate permissions and clean their owned workspace on a write
failure. Secret delivery to the harness and prevention of later disclosure are
separate caller and harness responsibilities.

## Source owners

| Concern | Owner |
|---|---|
| Registry and platform selection | `factory.go` |
| Provider interface | `provider.go` |
| Request and result shapes | `types.go` |
| Declarative settings and presets | `spec.go` |
| AGM wiring | `agm/cmd/agm/new_sandbox.go` |
| Implementations | `apfs`, `bubblewrap`, `gvisor`, `overlayfs`, `claudecode_provider.go` |

## Verification

Contract, platform, and provider tests live under `internal/sandbox`. Tests that
require mounts or external runtimes are platform- or integration-gated. The
strict guarantees are intentionally limited to [`SPEC.md`](SPEC.md); provider
comments and tests define narrower implementation behavior.
