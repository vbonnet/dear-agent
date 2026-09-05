# AGM Agentic API Reference

## Overview

AGM exposes three API surfaces, all backed by a shared operations layer (`internal/ops/`):

| Surface | Consumer | Entry Point |
|---------|----------|-------------|
| **CLI** | Humans, shell scripts | `agm session list --output json` |
| **MCP** | Claude, LLM tool-use | MCP server (JSON-RPC) |
| **Skills** | Claude Code agents | Skill files with frontmatter |

Every operation flows through `ops.OpContext`, ensuring identical behavior,
error handling, and output formatting regardless of surface.

Creation and message delivery use the same fail-closed readiness boundary on
every surface. Registration and startup prompts require a live configured
harness plus its tail-owning composer; an MCP-native onboarding wait is not a
substitute for that proof. Message sends resolve one active tmux pane and pin
process inspection, styled capture, and delivery to that pane ID, so another
pane or a later focus change cannot validate or redirect input.
`agm send msg` uses that shared operation for every CLI recipient in both
single-recipient and fan-out delivery; it does not retain a weaker CLI-only
tmux path or deliver to unregistered sessions whose harness identity cannot be
proven. Pure API recipients intentionally have no tmux pane: single-recipient
preflight resolves the registered delivery surface before any tmux probe, and
single-recipient plus fan-out sends reconstruct the adapter from the session's
persisted model, storage location, endpoint, and Azure settings. Credentials
remain runtime-only. The adapter's session status is the final common delivery
boundary, delivering only while it reports active or idle and failing closed
before any pending artifact when status is unavailable, suspended, or
terminated. Successful API delivery does not run tmux state resolution or
rewrite the manifest to `OFFLINE`. Under a provider-appropriate stable session-ID lock, API delivery
reloads lifecycle before adapter construction and shares that boundary with
archive: an in-flight completed turn commits before archive, or delivery sees
the reaping or archived lifecycle before paid provider work. The lock wait and provider
completion honor request cancellation, and the provider call has a finite
ceiling. Each sequential fan-out recipient receives a fresh outer deadline;
stable-lock acquisition, reconstruction, and readiness use an independently
bounded preflight context so the completed-turn phase starts with the adapter's
complete provider budget. Reconstruction loads only the requested session's
authoritative metadata and never scans unrelated session directories while the
lifecycle lock is held. Direct adapter callers use a context-aware store-scoped session lock
and the same provider ceiling. Provider failures, cancellation, and timeouts
leave no provisional user message in durable history.

Clearing OpenAI-compatible history atomically empties only the message stream
under the store lock. It reloads current on-disk metadata first, preserving the
model, title, working directory, and non-secret runtime configuration used by
later process reconstruction even when another process updated those fields.
Completed-turn commits reload the same metadata before updating history counts,
and every title, directory, or runtime-configuration writer participates in the
same lock and applies only its requested field.
Valid JSONL message records are reloaded without the standard scanner token
limit, so a long prompt or response cannot make the next append, read, or clear
transaction fail. Import converts a parsed conversation once and persists the
complete batch with one history transaction; an empty import performs none.

## Error Code Catalog

All errors use stable codes that agents can match on programmatically.

| Code | Status | Type | Title | When | Suggestion |
|------|--------|------|-------|------|------------|
| AGM-001 | 404 | `session/not_found` | Session not found | Identifier matches no session | `agm session list` or `--all` |
| AGM-002 | 409 | `session/archived` | Session is archived | Mutating an archived session | Unarchive first, or create new |
| AGM-003 | 503 | `tmux/not_running` | Tmux not running | No tmux server detected | Start tmux, check socket |
| AGM-004 | 503 | `dolt/unavailable` | Dolt unavailable | Dolt server not reachable | `agm admin dolt-status` |
| AGM-005 | 400 | `input/invalid` | Invalid input | Bad field value or format | Check value, use `--schema` |
| AGM-006 | 403 | `permission/denied` | Permission denied | Insufficient permissions | Check file/socket permissions |
| AGM-007 | 409 | `session/exists` | Session exists | Creating duplicate session | Use different name or resume |
| AGM-008 | 503 | `harness/unavailable` | Harness unavailable | AI harness not responding | Check harness process |
| AGM-009 | 404 | `workspace/not_found` | Workspace not found | Workspace detection failed | Set `--workspace` explicitly |
| AGM-010 | 404 | `uuid/not_associated` | UUID not associated | UUID not linked to session | `agm admin fix-uuid` |
| AGM-011 | 500 | `storage/error` | Storage error | Storage read/write failed | `agm admin doctor` |
| AGM-012 | 403/409 | archive verification types | Verification failed | Archive safety or cleanup verification failed | Inspect the returned suggestions |
| AGM-013 | 409 | `session/kill_protected` | Session recently active | Killing a recently active session without confirmation | Retry with the suggested confirmation flag |
| AGM-014 | 409 | `session/active_kill` | Session is active | Killing a live harness without stuck confirmation | Retry with `--confirmed-stuck` only when verified stuck |
| AGM-015 | 409 | `session/lock_timeout` | Session lock timeout | Another lifecycle mutation owns the stable session lock | Wait for it to finish and retry |
| AGM-016 | 409 | `session/not_ready` | Session not ready | The exact target harness cannot safely receive input | Wait for the harness composer to become ready and retry |
| AGM-017 | 503 | `session/output_unavailable` | Session output unavailable | The tmux backend could not answer for the session — the socket was unreachable, or a live capture failed while the session is still running — so no durable capture can stand in for the current task | Transient — retry; if it persists, check the tmux socket and permissions |
| AGM-018 | 409 | `session/delivery_uncertain` | Delivery outcome is uncertain | The irreversible submission boundary was crossed but its final acknowledgement was lost | Inspect the exact pane and do not retry automatically |
| AGM-019 | 500 | `session/delivery_accounting_failed` | Delivery accounting is incomplete | Compaction delivery succeeded but the durable attempt record could not be finalized | Inspect the exact pane, repair the ledger, and do not retry |
| AGM-020 | 409 | `session/compaction_policy` | Compaction policy rejected the attempt | Durable anti-loop accounting rejected a new attempt before delivery | Wait for the policy window or use an audited force override |
| AGM-021 | 409 | `session/compaction_unverified` | Compaction completion is unverified | Delivery succeeded but the exact runtime did not produce positive completion proof | Inspect the exact pane and do not retry automatically |
| AGM-022 | 500 | `command/compaction_failed` | Compaction command failed | A compaction command failed without a typed operation outcome | Inspect the session and audit state before deciding whether a retry is safe |
| AGM-100 | 200 | `dry_run` | Dry run | `--dry-run` flag is set | Remove flag to execute |

## RFC 7807 Error Format

All errors are returned as RFC 7807 Problem Details objects:

```json
{
  "status": 404,
  "type": "session/not_found",
  "code": "AGM-001",
  "title": "Session not found",
  "detail": "No session matches identifier \"my-session\".",
  "instance": "session/get",
  "suggestions": [
    "Run `agm session list` to see available sessions.",
    "Check if the session was archived: `agm session list --all`.",
    "Use a session name, UUID, or UUID prefix as the identifier."
  ],
  "parameters": {
    "identifier": "my-session"
  }
}
```

**Key fields for agents:**
- `code` -- stable, never changes; safe for programmatic matching
- `suggestions` -- actionable next steps the agent should try
- `parameters` -- echoes back the input that caused the error

## Output Formats

### `--output text` (default)

Human-readable tables and prose. Best for interactive terminal use.

```
$ agm session list
NAME         STATUS    BACKEND   UPDATED
my-project   active    claude    2m ago
research     stopped   gemini    1h ago
```

### `--output json` / `-o json`

Machine-readable JSON. Use this from agents, scripts, and MCP.

```
$ agm session list -o json
[{"name":"my-project","status":"active","backend":"claude",...}]
```

When `--output json` is set, errors go to stderr as RFC 7807 JSON.
Non-interactive mode is automatically enabled.

### Registered compaction results

`agm send compact` and `agm session compact` emit one compaction result object
to stdout after confirmed delivery. This success envelope is not an RFC 7807
Problem Details object. Its `status` is exactly one of:

| Status | Meaning | Command path |
|--------|---------|--------------|
| `dry_run` | The command was validated and its stable-ID-keyed prompt audit was saved, but nothing was delivered | `agm send compact --dry-run` |
| `sent` | Delivery and durable attempt accounting were confirmed; completion monitoring was not requested | `agm send compact` without `--verify`, or `agm session compact --monitor=false` |
| `verified` | Delivery was confirmed and the exact bound runtime subsequently produced positive completion proof | `agm send compact --verify`, or the default monitored `agm session compact` |

A `dry_run` result has this exact shape (`delivery` and `verification` are
JSON `null`):

```json
{
  "operation": "deliver_session_compaction",
  "status": "dry_run",
  "delivery": null,
  "verification": null,
  "command": "/compact preserve context",
  "prompt_file": "/home/agent/.agm/compaction-prompts/stable-session-id-compact-1.md"
}
```

`command` is present only for `dry_run`. `prompt_file` is always present at
the top level and identifies the durable prompt audit. A confirmed `sent` or
`verified` result instead includes the delivery receipt and omits `command`:

```json
{
  "operation": "deliver_session_compaction",
  "status": "verified",
  "delivery": {
    "operation": "deliver_session_compaction",
    "session_id": "stable-session-id",
    "name": "worker",
    "tmux_name": "runtime",
    "harness": "codex-cli",
    "pane_id": "%7",
    "pane_pid": 700,
    "target_pid": 707,
    "harness_start_time": "Thu Aug 27 12:00:00 2026",
    "tmux_session_id": "$3",
    "prompt_file": "/home/agent/.agm/compaction-prompts/stable-session-id-compact-1.md",
    "attempt_id": "attempt-7",
    "attempt_outcome": "confirmed",
    "delivered": true,
    "may_have_started": true,
    "post_submit_processing_observed": true,
    "accounting_pending": false
  },
  "verification": {
    "proof": "busy_then_stable_ready",
    "elapsed_ms": 1250
  },
  "prompt_file": "/home/agent/.agm/compaction-prompts/stable-session-id-compact-1.md"
}
```

The delivery `session_id` is the stable AGM session identity;
`tmux_session_id`, `pane_id`, `pane_pid`, `target_pid`, and
`harness_start_time` bind the receipt to the exact tmux incarnation, pane root,
and foreground harness process birth identity that
accepted Enter. The nested and
top-level `prompt_file` values are identical. The optional
`post_submit_processing_observed` field is emitted only when the atomic send
observed native processing immediately after submission; when false, the field
is absent. A `sent` result has the same delivery receipt, `status: "sent"`, and
`verification: null`. A `verified` result includes `verification.proof` and
elapsed milliseconds; the currently defined proof is
`busy_then_stable_ready`, where `busy` means positively classified native
`PROCESSING`, never an arbitrary occupied or generic-busy composer.

Strict terminal compaction refuses paste and Enter while any tmux client is
attached, and the error instructs the operator to detach. This is defense in
depth rather than an exclusive composer lease: a detached external
`tmux send-keys` writer can still alter terminal input between readiness and
mutation without changing the receipt identities, and tmux's queued condition
cannot bind the foreground child PID/birth if the harness exits or restarts in
that interval. Until a harness-native input transaction is available, callers
must coordinate external terminal writers and must not treat this transport as
proof that composer or foreground-harness identity was immutable across that
interval. On macOS, `ps lstart` is second-resolution; the receipt therefore
cannot distinguish the pathological case of the same PID being recycled within
the same second.

Failures, including a requested verification that cannot establish positive
completion evidence, do not use this success envelope. They emit one RFC 7807
object to stderr with a stable AGM error code and do not append success prose
or a success object to stdout.

## Field Masks

Use `--fields` to request only specific fields, reducing token consumption:

```
$ agm session list -o json --fields name,status
[{"name":"my-project","status":"active"},{"name":"research","status":"stopped"}]
```

```
$ agm session get my-project -o json --fields name,uuid,status
{"name":"my-project","status":"active","uuid":"abc-123"}
```

**How it works:** `ops.ApplyFieldMask()` marshals the result to a map, then
filters to only the requested keys. Unknown fields are silently ignored.

**When to use:** Always use `--fields` from agents. A full session object
contains 20+ fields; most operations need only 2-3.

## Dry Run

Mutation commands support `--dry-run` to preview changes without executing:

```
$ agm session archive my-project --dry-run -o json
{
  "status": 200,
  "type": "dry_run",
  "code": "AGM-100",
  "title": "Dry run",
  "detail": "Would archive session \"my-project\".",
  "instance": "session/archive",
  "suggestions": ["Remove `--dry-run` to execute."],
  "parameters": {
    "session_id": "01234567-89ab-cdef-0123-456789abcdef",
    "session_name": "my-project"
  }
}
```

Except for the registered compaction result documented above, dry-run returns
an AGM-100 `OpError` with status 200. Agents can parse
the `detail` and `parameters` fields to confirm the exact resolved target before
re-running without the flag. Single-session archive previews execute the shared
archive guards but return before any AGM, provider, process, worktree, branch,
sandbox, settings, telemetry, or reaper mutation. Execution reuses the returned
stable AGM session ID; active asynchronous execution carries its resolved tmux
identity separately to the detached reaper.

## Progressive Disclosure (Skills)

Skills use a 3-layer documentation model to minimize token overhead:

### Layer 1: Frontmatter (always loaded)
```yaml
---
name: agm-session-list
description: List AGM sessions with optional filters
arguments:
  - name: status
    description: Filter by status (active, stopped, archived)
    required: false
---
```

### Layer 2: Skill body (loaded on invocation)
Concise usage instructions and examples. Kept under 50 lines.

### Layer 3: `--help` output (on demand)
Full CLI help text, only fetched when the agent needs detailed flag info.

This layered approach means an agent scanning available tools pays ~5 tokens
per skill (frontmatter only), not 500+ tokens for full documentation.

## OpContext

Every operation receives an `OpContext` with shared configuration:

```go
type OpContext struct {
    Storage    dolt.Storage
    Tmux       session.TmuxInterface
    Config     *config.Config
    DryRun     bool       // --dry-run flag
    Fields     []string   // field mask
    OutputMode string     // "json" or "text"
}
```

CLI, MCP, and Skills each construct an `OpContext` and pass it to the
same `ops.*` functions. This guarantees that `agm session list`,
the MCP `session/list` tool, and the `agm-session-list` skill all
return identical results with identical error handling.
