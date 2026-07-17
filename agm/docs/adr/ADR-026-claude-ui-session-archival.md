# ADR-026: Programmatic Archival of Claude Code UI Sessions

**Status:** Accepted — implemented (Phases 1–3)
**Date:** 2026-05-17 (Phase 3 wiring landed 2026-06-22)
**Deciders:** vbonnet (owner), AGM Foundation Engineering
**Context:** There is no first-party CLI/API to archive the sessions that
accumulate in the claude.ai/code and Claude desktop "Code" session list. The
user archives ~400 of them by hand. This ADR proposes where that capability
lives and how it works.

> **Implementation status (2026-06-22).** Phases 1–2 landed in PR #126
> (`agm session archive-ui`, the `claudeui` package, and
> `ops.ArchiveUISessions`). Phase 3 — the missing automatic invocation that
> let the store re-accumulate to 278 unarchived files — is now wired as a
> daily `launchd` job: `agm admin install-archive-ui-schedule` installs
> `com.dear-agent.archive-ui.plist`, which runs
> `agm session archive-ui --older-than 7d --status idle --apply` every 24h
> (and once at load). It joins the same launchd family as the orphan reaper
> (`install-reap-schedule`) and worktree sweep (`install-sweep-schedule`).
> The experimental `--via=web-api` escape hatch (Phase 3, below) remains
> unbuilt and is not needed: V3 validation showed the local flip propagates.
>
> **Namespace boundary reaffirmed (2026-07-17).** Consolidating AGM-internal
> archival under `ops.ArchiveSession` does not merge this capability into that
> lifecycle. `ArchiveUISessions` still owns only Claude desktop/UI records;
> `ArchiveSession` owns AGM manifests, reaper completion, and associated
> provider-extension outcomes. Neither operation calls the other.

Related: [ADR-016 Shared Ops Layer](ADR-016-shared-ops-layer.md),
[ADR-001 CLI Command Structure](ADR-001-cli-command-structure.md),
[ADR-012 Provider Transport Layer](../../../docs/adrs/ADR-012-provider-transport-layer.md),
the existing `agm session gc` / `agm session archive` (AGM-internal sessions only).

---

## Problem

`remoteControlAtStartup` is now enabled globally, so every CLI session shows up
in the claude.ai/code web UI and the Claude desktop "Code" tab. The list grows
without bound. The only way to clear it is the manual UI: select a session,
archive, repeat. The user does this ~400 sessions at a time. It is the single
most painful recurring chore in the workflow.

`agm session gc` does **not** solve this. It only touches AGM's own
Dolt-backed session manifests (`lifecycle='archived'`); it explicitly does not
read or write `~/.claude/` or `~/Library/Application Support/Claude/`, and it
has no concept of the claude.ai/code UI list (confirmed in
`agm/internal/ops/session_gc.go` and prior memory). Third-party tools
(`cc-sessions`, `claude-bulk-delete`) are either read-only resumers or
unvetted browser hacks.

**Goal:** one command —
`agm session archive-ui --older-than 7d --status idle` — that declutters the
UI session list safely, reversibly, and without harvesting credentials.

---

## Research Findings (evidence base)

> Routing note: per `.dear-agent.yml`, standalone research artifacts belong in
> `~/src/engram-research`, not this code repo. These findings are kept inline
> because they are the *decision-constraining evidence* for an AGM code change
> (ADR Context), not a free-standing literature review. Raw external source
> notes, if expanded, belong in `engram-research`.

### F1. There are two disjoint "session" namespaces

This is the load-bearing fact. Almost every third-party tool conflates them.

| | **Local CLI sessions** | **Desktop "Code" / claude.ai/code list** | **claude.ai chat conversations** |
|---|---|---|---|
| Store | `~/.claude/projects/<slug-cwd>/<uuid>.jsonl` | `~/Library/Application Support/Claude/claude-code-sessions/<deviceId>/<accountId>/local_<id>.json` (+ server mirror) | Anthropic server, org-scoped |
| ID | `sessionId` UUID (== filename) | `local_<uuid>` + `cliSessionId` + server `cse_<id>` | conversation `uuid` |
| Archive bit | none (filesystem only) | **`isArchived` boolean in the JSON** | none — only delete |
| List API | none | desktop app internal (`/v1/sessions`, `code/sessions` — private) | `claude.ai/api/organizations/{org}/chat_conversations` (undocumented) / Compliance API (Enterprise) |

The thing the user manually archives is the **middle column**: the desktop /
claude.ai/code session list. It is backed by a local per-session JSON store
**that already has an `isArchived` flag**.

### F2. The local desktop session store — verified on this machine

Path: `~/Library/Application Support/Claude/claude-code-sessions/<deviceId>/<accountId>/local_<sessionId>.json`

Measured state (2026-05-17): **593 files — 523 `isArchived:true`, 70
`isArchived:false`**. The 70 unarchived span 2026-05-04 → 2026-05-17. This
matches the reported pain exactly: the store accumulates hundreds; archiving is
a per-row UI action.

Per-file schema (observed fields):

```json
{
  "sessionId": "local_0595b63b-...",         // desktop session id
  "cliSessionId": "041ee5fa-...",            // == ~/.claude/projects/<slug>/<this>.jsonl
  "cwd": "/Users/vbonnet/worktrees/dear-agent/objective-diffie-c0b2f6",
  "originCwd": "...",
  "createdAt": 1778964435744,                // epoch ms
  "lastActivityAt": 1778964565517,           // epoch ms — the age signal
  "model": "claude-opus-4-7[1m]",
  "isArchived": false,                       // <-- the lever
  "title": "Wire cleanup stop-hook",
  "permissionMode": "auto",
  "completedTurns": 1,
  "dispatchParentId": "local_ditto_...",
  "enabledMcpTools": { ... }
}
```

Adjacent state used for "is this session live?" determination:

- `~/.claude/sessions/<pid>.json` — live process registry:
  `{pid, sessionId, cwd, startedAt, version, kind, entrypoint:"claude-desktop"}`.
  Presence ⇒ a running process owns that CLI session.
- `~/Library/Application Support/Claude/bridge-state.json` — maps
  `localSessionId ↔ remoteSessionId (cse_…) ↔ environmentId (env_…)`, keyed by
  `<accountId>:<deviceId>`. Confirms each local session has a server-side id.
- Desktop binary strings include `/api/organizations/`, `/v1/sessions`,
  `code/sessions`, `claude.ai/code` — the app *does* call a server session API,
  but it is the desktop app's **private** contract.

AGM already has precedent for reading this filesystem read-only:
`agm/internal/session/session.go` `checkClaudeBloat()` parses
`~/.claude/projects/{hash}/{uuid}.jsonl` for bloat detection.

### F3. The premise about `/v1/sessions` archive endpoint — corrected

The task framed claude.ai as exposing `GET /v1/sessions` +
`POST /v1/sessions/{id}/archive`. Reality:

- The Claude **desktop binary** literally contains `/v1/sessions` and
  `code/sessions` strings, so an internal endpoint of that shape plausibly
  exists — but as the **private desktop↔server contract**, undocumented and
  unstable. Not a public, stable surface to build on.
- The reverse-engineered **public** web surface is
  `GET/DELETE/POST claude.ai/api/organizations/{org}/chat_conversations`
  (incl. `delete_many`, ~500/req). It has **no archive verb at all — only
  delete** (soft-delete is the de-facto archive). Highest-trust reference impl:
  `KoushikNavuluri/Claude-API` (898★, explicitly unofficial). `delete_many`
  body: `{"conversation_uuids":[...]}`. Auth: first-party
  `sessionKey=sk-ant-sid01-…` cookie, in-origin.
- The official, contractually-stable path is the **Anthropic Compliance API**
  (`api.anthropic.com/v1/compliance/apps/chats`): **Enterprise-plan only**,
  scoped `sk-ant-api01` key, **hard-delete only (no archive, no recovery)**.

### F4. Third-party tools — trust assessment

- **`cc-sessions` (chronologos, 23★) / `cc-deck/cc-session` (12★):** Rust,
  read-only resumers over `~/.claude/projects/*.jsonl`. No network to
  Anthropic, no auth, no delete/archive. Safe to read as a *discovery*
  reference; cannot exfiltrate. Confirm the on-disk model.
- **`claude-bulk-delete` (scootsmagoo, 0★):** browser bookmarklet; DOM-scrapes
  the sidebar and `DELETE`s via the in-origin cookie. Unvetted, *deletes* (no
  undo), endpoints can change. Not a dependency; pattern only.
- **Compliance API:** correct provenance, wrong semantics (hard-delete,
  Enterprise-only).

### F5. The credential-harvest path is actively blocked — and that's correct

During research, a query of the Claude `Cookies` SQLite store (to confirm the
`sessionKey` row) was **denied by the harness auto-mode classifier** as
credential-store scanning. This is a *design signal*: any AGM feature that
routinely extracts the first-party session cookie to drive the undocumented
web API would be engineering around a safety rail the platform deliberately
enforces. We should not build the primary path on cookie harvesting.

### F6. Prior art in engram-research

Existing AGM session-archive work (reaper, `gc`, disposable TTL, Dolt
migration) is all **AGM-internal** (`lifecycle='archived'` in Dolt). Nothing
in `engram-research`/`ai-conversation-logs` addresses the claude.ai/code UI
list, `isArchived`, `claude-code-sessions`, or `chat_conversations`. This is
greenfield; no design to supersede.

---

## Decision

**Build the capability into AGM's shared `ops` layer as a *local desktop
session-store reconciler*, exposed as `agm session archive-ui`. It flips the
`isArchived` flag in the local `claude-code-sessions` JSON store. It does not
delete anything, does not call undocumented web APIs by default, and does not
harvest credentials.**

Concretely:

1. **New ops capability** `ops.ArchiveUISessions(ctx, req)` in
   `agm/internal/ops/session_archive_ui.go`, backed by a new read/write package
   `agm/internal/claudeui/` that owns the store layout, schema-version guard,
   atomic write, and backup. Per [ADR-016](ADR-016-shared-ops-layer.md), logic
   lives in `ops` so CLI, a future MCP tool, a dear-agent skill, and cron all
   share one tested implementation.

2. **New CLI subcommand** `agm session archive-ui` in
   `agm/cmd/agm/session_archive_ui.go`, registered to `sessionCmd` in `init()`
   per [ADR-001](ADR-001-cli-command-structure.md). It is a distinct verb from
   the existing `agm session archive` (which archives AGM's *own* Dolt
   sessions) — the `-ui` suffix makes the namespace boundary explicit and
   prevents the F1 conflation in our own UX.

3. **Mechanism = local `isArchived` flip**, never delete, never touch
   `.jsonl` transcripts. The desktop app reconciles its local store with the
   server on launch/sync; we rely on that existing sync rather than calling the
   server ourselves. (Server-sync direction is a validated risk — see Risks.)

4. **Default is dry-run.** `--apply` is required to mutate. Every mutated file
   is backed up to `~/.agm/backups/claude-ui-sessions/<ts>/` first; `--unarchive`
   reverses. Operation is idempotent.

5. **"idle" / "status" is derived, conservative.** A session is `idle` iff:
   its `sessionId`/`cliSessionId` is **not** in the live `~/.claude/sessions/*.json`
   PID set, **and** there is no live tmux session for it, **and**
   `now - lastActivityAt > --older-than`. Anything live or ambiguous is skipped
   and reported, never archived.

   > **Correction (2026-06-22).** Liveness is **identity-based only**
   > (`cliSessionId`/`sessionId`), exactly as specified above. The initial
   > implementation also matched on `cwd` as a "conservative" extra signal, but
   > `cwd` is not per-session: dozens of sessions share one working directory
   > (every CLI run rooted at `~/src/dear-agent`, or at `$HOME`), so a single
   > live process there marked **every** past session in that directory as live
   > forever. Measured on this machine, that buried **248** long-idle sessions
   > as false-"live" against only **13** genuinely live by id — the dominant
   > reason the desktop list never drained even when the tool ran. The `cwd`
   > clause was removed (`isLive` in `session_archive_ui.go`); idleness remains
   > protected by the age gate and the safety-warning gate (uncommitted/unmerged
   > work, open PR, awaiting-input), so dropping it never archives a session
   > that still has live work.

6. **The undocumented web API is an explicitly experimental, off-by-default
   escape hatch only** (`--via=web-api`, hidden, requires an explicit
   `--i-understand-unsupported`), used *only if* validation shows the local
   flip does not propagate to claude.ai/code. Even then it `delete_many`s
   nothing — it would only call a soft/archive route if one is confirmed. It
   never reads the cookie store; it can only consume a user-provided token via
   env. Document, but do not implement in phase 1.

7. **Replacement criterion.** If Anthropic publishes a supported per-session
   archive API with archive (not delete) semantics, AGM must replace this local
   store extension with that API and retain the exact-UUID and reversible
   outcome contract at the shared archive boundary.

### Interface

```
agm session archive-ui [flags]

  --older-than DUR     Archive sessions idle longer than DUR (e.g. 7d, 24h). Default 7d.
  --status STR         Which to consider: idle (default) | all
  --apply              Perform the flip. Omit = dry-run (default).
  --unarchive          Reverse: flip isArchived true -> false (same filters).
  --keep N             Always keep the N most-recently-active unarchived (default 20).
  --account ID         Limit to one accountId dir (default: all under the device).
  --device  ID         Limit to one deviceId dir (default: autodetect single).
  --backup-dir PATH    Override backup location.
  --json               Machine-readable output (for skill/cron/MCP).
  --protect-substr S   Never archive sessions whose title/cwd contains S (repeatable).

Default (no --apply) prints a table:
  TITLE  CWD  AGE  LIVE?  ARCHIVED?  ACTION(would-archive/skip:reason)
Exit 0 always in dry-run; exit non-zero on write failure under --apply.
```

`agm session archive-ui --older-than 7d --status idle` (dry-run) →
review → `… --older-than 7d --status idle --apply`.

---

## Alternatives Considered

**A. Local `isArchived` reconciler (CHOSEN).** No auth, no network, no cookie
harvest (respects F5), reversible (backups + `--unarchive`), fully unit-testable
with fixtures, matches AGM precedent (`checkClaudeBloat` already reads this
tree), archive-not-delete (matches intent, no data loss). Cost: depends on an
undocumented file shape ⇒ must be defensive (schema-version pin, dry-run
default, refuse unknown shapes); server-sync propagation must be validated.

**B. Undocumented `claude.ai/api/.../chat_conversations` via first-party
cookie.** Rejected as primary: it is *delete*, not archive (irreversible data
loss of conversation history); endpoints undocumented/unstable; requires the
`sk-ant-sid01` cookie, whose extraction the harness correctly blocks (F5).
Building AGM to routinely defeat that rail is the wrong default. Retained only
as a documented, hidden, opt-in fallback.

**C. Anthropic Compliance API.** Rejected: Enterprise-plan only, hard-delete
only (no archive, no recovery window), needs a scoped admin-ish key, requires
user-ID enumeration. Wrong semantics and unavailable for a single-user
declutter tool.

**D. dear-agent skill or brain-v2 integration as the home.** Rejected as the
*home* (kept as optional thin wrappers). A skill that shells out has no tests
and duplicates logic; brain-v2 is memory, not session lifecycle. Per ADR-016
the logic belongs in `ops`; a dear-agent skill + `agm session archive-ui` cron
can wrap it later (phase 3).

**E. Extend `agm session gc`.** Rejected: `gc` operates on AGM's Dolt
manifests; overloading it with a different namespace (F1) reintroduces exactly
the conflation we are trying to avoid. Distinct verb, shared ops patterns.

---

## Consequences

**Positive**
- One command replaces a ~400-row manual chore; reversible and dry-run-first.
- No credentials, no network, no third-party code in the default path.
- Tested `ops` code reusable by CLI/MCP/skill/cron (ADR-016).
- Documents the session-store format so it is not re-reverse-engineered.

**Negative / risks (see Risks)**
- Couples AGM to an undocumented Anthropic on-disk format (mitigated by
  schema-version guard + dry-run default + refuse-on-unknown).
- Local flip may not immediately reflect server-side until desktop sync;
  must be validated; web fallback is unsupported.

**Neutral**
- Adds one subcommand and two small packages; no change to existing `gc`/
  `archive`/Dolt behavior.

---

## Implementation Plan (no code yet — phased)

**Where things go**

| Concern | Location |
|---|---|
| Store layout, schema guard, atomic write, backup | `agm/internal/claudeui/` (new) |
| Business logic (filter, idle-derivation, plan/apply) | `agm/internal/ops/session_archive_ui.go` (new) |
| Request/Result types | same file: `ArchiveUISessionsRequest/Result` |
| CLI subcommand + flags | `agm/cmd/agm/session_archive_ui.go` (new), `init()` → `sessionCmd` |
| Live-process / tmux cross-check | reuse `agm/internal/session` + existing tmux helpers |
| Fixtures | `agm/internal/claudeui/testdata/` (sample store dirs) |
| Skill/cron wrapper (optional) | dear-agent skill + `agm session archive-ui --json` in cron |

**Phase 1 — read + dry-run (no mutation).** `claudeui` reader; store
discovery (`<deviceId>/<accountId>`); idle derivation against
`~/.claude/sessions/*.json` + tmux; `--older-than`/`--status`/`--keep`/
`--protect-substr` filtering; table + `--json`. Unit tests on fixtures.
Acceptance: dry-run on the real store reproduces the 523/70 split and a
correct would-archive set, mutating nothing.

**Phase 2 — apply + reverse.** Atomic write (`tmp`+`rename`), per-file backup
to `~/.agm/backups/claude-ui-sessions/<ts>/`, `--unarchive`, idempotency,
schema-version guard (refuse on unknown shape, configurable pin), refuse if a
target session is live in the PID registry. Tests for backup/restore round-trip
and idempotency.

**Phase 3 — wrappers + validated escape hatch.** dear-agent skill + cron using
`--json`; document the hidden experimental `--via=web-api` only after Phase 2
validation determines whether the local flip propagates server-side.

**Dogfooding / DEAR.** Implementation should be run through AGM
(`agm new` / `agm send`), with `agm acceptance show` at start; the
`.dear-agent.yml` acceptance criteria (`go test ./...`, `golangci-lint`,
no-regressions) gate the Audit phase. If the undocumented-format coupling
causes churn, write an `engram-research/retrospectives/` entry (Retro phase).

## Validation

- **V1 (Phase 1):** `agm session archive-ui --older-than 7d --status idle`
  dry-run lists only sessions with no live PID/tmux and
  `lastActivityAt` older than 7d; live sessions appear as `skip:live`.
- **V2 (Phase 2):** `--apply` flips exactly the dry-run set; a matching backup
  exists per file; `--unarchive` restores byte-equivalent originals; second
  `--apply` is a no-op (idempotent); a corrupted/unknown-schema file is refused,
  not rewritten.
- **V3 (server propagation):** after `--apply`, restart the Claude desktop app
  and confirm the archived sessions leave the claude.ai/code list. Records
  whether local→server sync is automatic. **This single result decides whether
  the Phase 3 web fallback is ever needed.**
- **V4 (safety):** never modifies any `~/.claude/projects/*.jsonl`; never
  deletes a file; no network egress in the default path (verify with no
  outbound during `--apply`).

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Undocumented JSON shape changes across Claude versions | `claudeui` schema-version guard; refuse-on-unknown; dry-run default; version pin configurable |
| Local flip doesn't sync to server | V3 validates explicitly; documented experimental web fallback; never silently "succeed" |
| Desktop app overwrites our write / race with a live session | Refuse when session is in the live PID registry; atomic write; advise running with desktop app closed; backups |
| Mistaking a live/important session as idle | Conservative AND-of-three idle test; `--keep N`; `--protect-substr`; dry-run default; `--unarchive` |
| Scope creep into delete semantics | Hard rule: this verb never deletes and never touches `.jsonl`; delete is out of scope for this ADR |
| Multi-account/device store dirs | `--account`/`--device`; autodetect single; explicit error if ambiguous |

---

## Routing & Authority Note

This ADR is a code-constraining design doc for AGM ⇒ it belongs in this repo
(`agm/docs/adr/`, per `.dear-agent.yml` decision procedure step 1 and the
explicit request). The embedded **Research Findings** are the decision's
evidence base, kept inline as ADR Context rather than as a standalone
`research/*.md` (which `.dear-agent.yml > forbidden-paths.research` prohibits
here; standalone research expansion belongs in `~/src/engram-research`). No
implementation is included, per the request.
