# ADR-027: Bumblebee Endpoint Supply-Chain Scanner

**Status**: Accepted
**Date**: 2026-05-26
**Context**: Add a daily, host-side supply-chain inventory pass that
covers the layer DeepSec, Trivy, and the `weekly-security-audit` task all
miss — *what is actually installed on the developer endpoint*, including
MCP configs and IDE/browser extensions. Companion to the archived audit in
[`engram-research`](https://github.com/vbonnet/engram-research/blob/main/archive/dear-agent-temporal-artifacts-2026-06-30/docs/design/2026-05-23-security-pipeline-bumblebee-deepsec.md).

Builds on / aligns with:

- [ADR-011: Scheduled Repository Audit Subsystem](ADR-011-dear-audit-subsystem.md) — the
  catalog-curation follow-up (see "Follow-ups") is a candidate Audit-phase
  signal source.
- `docs/deepsec.md` — source-code scanner this complements but does not
  overlap with.

---

## Context

Dear-agent's security pipeline today covers three layers:

| Layer                      | Tool                       | Surface                |
| -------------------------- | -------------------------- | ---------------------- |
| Source code                | DeepSec                    | Every PR vs main       |
| Container/SBOM             | Trivy                      | `sbom-scan.yml` |
| Disclosed-CVE intel        | `weekly-security-audit`    | Daily Cowork scheduled task (sandboxed; no host access by design) |

What none of these touch is *the installed state of the developer's
laptop*. The dear-agent CLAUDE.md describes a setup running many MCP
servers (some carrying OAuth tokens for Slack/Gmail/Calendar/etc.),
several IDE extensions, and browser extensions across multiple browsers.
If one of those gets compromised — or simply ships a malicious version —
nothing in the current pipeline notices.

The archived 2026-05-23 audit doc evaluated [Perplexity's Bumblebee](https://github.com/perplexityai/bumblebee),
a single-binary read-only inventory scanner, and recommended adopting it
on a daily launchd schedule (§4.1). This ADR accepts that recommendation
with three modifications tightening the install path against the threat
model Bumblebee exists to address.

## Decision

Adopt Bumblebee on a per-user daily LaunchAgent, with a pinned and
checksum-verified install, writing local-only NDJSON output. Specifically:

### D1. Install — pinned release with SHA-256 verification

The audit doc proposed `go install github.com/perplexityai/bumblebee/cmd/bumblebee@latest`.
That's appropriate for a tool we trust to update transparently, but
Bumblebee is *the* tool we're installing because we don't trust silent
updates to other tools on this machine — so the install path itself must
not be a TOFU step.

`dear-agent-bumblebee install` (in `cmd/dear-agent-bumblebee/install.go`)
downloads a specific tagged tarball (`v0.1.1`) and verifies the SHA-256
against a digest embedded in the binary *before* the tarball is opened.
The digests were captured 2026-05-26 from two independent sources that
matched:

1. The GitHub release-asset digest API
2. The upstream `checksums.txt` artifact

A re-uploaded release, a compromised mirror, or an in-flight rewrite all
fail closed. Bumping the pin is a deliberate, reviewed change.

### D2. Scheduler — LaunchAgent (per-user), not LaunchDaemon (root)

The Cowork scheduled-task system has no host filesystem access by design
(memory: `cowork-scheduled-tasks-location.md`), so Bumblebee cannot live
there — the launchd path is forced.

LaunchAgent (per-user) is the right scope: Bumblebee inventories
user-scoped state (`~/Library/Application Support/`, `~/.npm/`, IDE/browser
user profiles, MCP configs). Running as root broadens the service
surface without buying additional coverage. The audit doc reached the
same conclusion in §4.1.2.

`RunAtLoad` is intentionally off — we do not want a scan on every login
competing for the user's first-minute resources. The calendar entry fires
once daily at 04:00 local; ad-hoc runs go through `make bumblebee-scan`
or `launchctl kickstart`.

### D3. Output — local-only NDJSON, atomic write

NDJSON output lands in the per-user data dir
(`~/Library/Application Support/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson`
on macOS, `$XDG_DATA_HOME/dear-agent/bumblebee/…` on Linux). The wrapper
writes via temp file + rename so a partial scan can't be mistaken for a
complete inventory by the next pass.

No network egress for the output. Bumblebee's own catalog fetch is the
only external call, and only when `--exposure-catalog` is supplied — which
isn't yet (see Follow-ups).

### D4. Catalog — defer to a follow-up

Bumblebee's value compounds when paired with an exposure catalog
identifying known-bad versions. Without one, it runs in pure-inventory
mode (still useful: day-over-day diffs reveal new MCP/IDE/browser
extensions appearing on the machine).

Shipping a catalog now is premature — the project is v0.1.x, days old,
and community catalogs haven't coalesced. The audit doc §2.3
recommended a narrow, in-repo catalog seeded from
`weekly-security-audit` findings, one PR per added entry. That work is
left for a follow-up ADR or Process DEAR Define artifact.

### D5. No CI integration

Running Bumblebee in CI would inventory the GitHub Actions runner, not a
developer endpoint. That's a category error. The audit doc reached the
same conclusion in §4.1 ("Out of scope for v1"). We do not add a
workflow.

## Consequences

**Positive**

- Coverage of an objectively unscanned layer (installed packages + MCP +
  extensions) that the existing pipeline doesn't reach.
- $0 ongoing cost — single Go binary, no LLM calls.
- Pinned + verified install means *this* tool is not a supply-chain entry
  point even though it exists to detect them elsewhere.
- Sets up a coherent intel-to-action loop: when the catalog follow-up
  lands, the daily scan becomes "this machine is exposed to disclosed
  incident X."

**Negative / costs**

- Disk: NDJSON files accumulate one per day, ~1–10 KB each in typical
  use. We don't ship a rotation. If this becomes material (it shouldn't
  for years), add a `find -mtime` reaper.
- A new LaunchAgent the user has to consent to and remember. The
  `--uninstall` path is supported and tested.
- Bumblebee v0.1.x — CLI flags and catalog format may shift. The pin
  protects us from breaking under us; the trade is that bumps are manual.

**Risks**

- **Catalog never lands.** The diff-vs-yesterday delta is still useful
  on its own, but most of the value comes from catalog matching. Flag
  this in the next scheduled repository audit pass.
- **Schedule conflict.** 04:00 local was chosen as low-activity; if the
  user is in a different timezone or works overnight, edit the plist's
  `StartCalendarInterval` and `launchctl kickstart`. Documented.

## Alternatives considered

1. **Cowork scheduled task** — rejected. No host filesystem access, by
   design (`cowork-scheduled-tasks-location.md`). Bumblebee needs to read
   the filesystem; this is a non-starter.
2. **Cron** — works on Linux but less idiomatic on macOS where launchd
   is the supported path. We can add a cron entry for Linux as a
   follow-up.
3. **`go install … @latest`** — simpler, but loses the pin. Rejected on
   threat-model grounds (D1).
4. **`brew install` (if/when a tap exists)** — Bumblebee doesn't have a
   Homebrew formula yet. Revisit when one ships from a maintainer with
   provenance.
5. **Snyk / Socket.dev / OSV-Scanner instead of Bumblebee** — these are
   adjacent but not quite: they focus on dependency *manifests* (which
   DeepSec and Trivy already touch), not on MCP configs and extension
   manifests. Bumblebee's distinguishing surface is the MCP + IDE +
   browser-extension scan.

## Follow-ups

- [ ] Curate `etc/bumblebee/catalog.json` seeded from
  `weekly-security-audit` findings (`pgserve`, `elementary-data`, …);
  one PR per entry. (Owner: TBD.)
- [ ] Linux scheduling: ship a `systemd --user` unit or `cron.d` entry.
- [ ] NDJSON retention sweeper if the per-day files become material.
- [ ] DeepSec docs-only short-circuit (audit doc §3 lever 1) — unrelated
  to Bumblebee but raised by the same audit; tracked separately.
- [ ] Revisit pin when v0.2.x ships; capture digests from two sources
  again at bump time.
