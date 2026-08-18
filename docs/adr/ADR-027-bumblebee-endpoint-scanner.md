# ADR-027: Pinned endpoint inventory scanner

Status: Accepted (2026-05-26; verified 2026-07-17)

## Context

Source, SBOM, and disclosed-CVE scans do not inspect the tools actually
installed on a developer endpoint, including MCP and editor/browser extension
inventory. Installing an endpoint scanner without pinning would itself add a
supply-chain risk.

## Decision

`cmd/dear-agent-bumblebee` owns a local Bumblebee integration:

- install a pinned release only after checksum verification;
- run it from a per-user LaunchAgent;
- write local NDJSON findings with restricted permissions;
- scan only read-only inventory surfaces and fail closed on unsupported
  platforms or unverifiable artifacts.

The wrapper, not an agent prompt, owns installation and scheduling details.

## Alternatives

Relying on source scanners leaves endpoint state invisible. Installing latest
at runtime weakens provenance. A privileged system daemon would widen the blast
radius without improving the read-only inventory use case.

## Consequences

Pinned versions require deliberate upgrades, and scanner coverage is limited by
the upstream tool. Tests under `cmd/dear-agent-bumblebee` verify install,
LaunchAgent, and scan behavior.
