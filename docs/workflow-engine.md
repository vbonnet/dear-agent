# Workflow Engine — Operator Guide

**Status:** Active (Phase 3 shipped 2026-05-03)
**Audience:** developers running, authoring, or migrating workflows
**Source of truth for:** how to use the engine end-to-end. Architecture
lives in [ADR-010](adrs/ADR-010-workflow-engine-architecture.md); the
release plan lives in [ROADMAP.md](../ROADMAP.md).

This guide is the operator manual. If you've never used `dear-agent`
before, the **10-minute walkthrough** below takes you from a fresh
clone to a working research workflow with HITL approvals and
searchable outputs.

---

## 10-minute walkthrough

The thread below is one continuous shell session. Every command works
against a fresh `runs.db` — no global state, no setup beyond the build.

### 1. Build the toolchain (≈30s)

```bash
git clone https://github.com/vbonnet/dear-agent && cd dear-agent
GOWORK=off go install \
  ./cmd/workflow-run \
  ./cmd/workflow-status \
  ./cmd/workflow-list \
  ./cmd/workflow-logs \
  ./cmd/workflow-cancel \
  ./cmd/workflow-approve \
  ./cmd/workflow-lint \
  ./cmd/workflow-roles \
  ./cmd/workflow-migrate \
  ./cmd/dear-agent-mcp \
  ./cmd/dear-agent-search
```

Everything from here on assumes `$GOPATH/bin` is on your `PATH`.

### 2. Author a workflow (≈2 min)

A minimum viable workflow is one bash node:

```yaml
# hello.yaml
schema_version: "1"
name: hello
version: 0.1.0
description: "smoke test"
nodes:
  - id: greet
    kind: bash
    bash:
      cmd: 'echo "hello from $(date -u +%FT%TZ)"'
```

Lint it:

```bash
workflow-lint hello.yaml
# OK: hello.yaml
```

### 3. Run it (≈10s)

```bash
workflow-run --db ./runs.db hello.yaml
```

The runner prints the assigned `run_id`, then streams node transitions
as it executes. Every transition lands in `audit_events`; every
attempt lands in `node_attempts`.

### 4. Inspect the run (≈10s)

```bash
workflow-status --db ./runs.db <run_id>
workflow-status --db ./runs.db --json <run_id> | jq .
workflow-list --db ./runs.db
workflow-logs --db ./runs.db <run_id>
```

`workflow-status` answers "what happened?" in a single round-trip
because every state transition is a row, not a recomputed projection.

### 5. Add a role-based AI node (≈3 min)

Roles decouple "what tier of model is this node" from "what model is
deployed today". Define a registry once:

```yaml
# .dear-agent/roles.yaml
version: 1
roles:
  research:
    description: "long-context analysis"
    primary:
      model: gemini-3.1-pro
      effort: high
      max_context: 1000000
```

Reference it from a node:

```yaml
nodes:
  - id: research
    kind: ai
    role: research                      # not `model:`
    budget:
      max_tokens: 50000
      max_dollars: 1.00
      on_overrun: escalate
    ai:
      prompt:
        template: ./prompts/research.md.tmpl
```

Migrating to a new model is a one-line edit to `roles.yaml`. The
`workflow-lint --check-deprecated-models` command flags any node still
hardcoding a `model:`.

#### Multi-provider load-spreading

The shipped `config/roles.yaml` deliberately puts each role's **primary**
tier on a different provider so a typical workflow round-robins across
Anthropic, OpenAI, and Google. The benefits:

- A single-vendor outage degrades one role at a time, not the whole engine.
- Per-vendor rate limits are spread across the workload.
- Each role's secondary/tertiary tiers spill onto the other two providers,
  so any single role survives the loss of any single vendor.

| Role           | Primary (vendor)             | Secondary       | Tertiary        | Why this primary                        |
|----------------|------------------------------|-----------------|-----------------|------------------------------------------|
| `research`     | `gemini-3.1-pro` (Google)    | `claude-opus-4-7` | `gpt-5.5-pro` | 1M-context window, strong synthesis      |
| `implementer`  | `claude-opus-4-7` (Anthropic)| `gpt-5.5-pro`   | `gemini-3.1-pro`| Highest accepted-patch rate in evals     |
| `reviewer`     | `gpt-5.5-pro` (OpenAI)       | `claude-opus-4-7` | `gemini-2.5-pro` | Off-vendor critic — different blind spots |
| `orchestrator` | `claude-sonnet-4-6` (Anthropic)| `gemini-2.5-flash` | `gpt-5.4-mini` | Cheap/fast for high-volume routing       |

The `Resolver` walks tiers in order and skips any tier whose model fails the
node's capability or budget filters, or that the optional
`CapacityChecker` reports as rate-limited. To pin a role to a specific
model for one run, pass `Resolver.Overrides[role] = "<model-id>"` — useful
for A/B comparisons and incident workarounds.

### 6. Add a HITL gate (≈1 min)

```yaml
nodes:
  - id: review
    kind: ai
    role: reviewer
    depends_on: [research]
    hitl:
      block_policy: on_low_confidence
      confidence_threshold: 0.7
      approver_role: research-lead
      timeout: 24h
      on_timeout: escalate
```

When the gate fires, the node enters `awaiting_hitl` and the runner
returns. Approve it from any client:

```bash
workflow-approve --db ./runs.db --as research-lead --reason lgtm <approval_id>
# or via MCP
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workflow_approve","arguments":{"approval_id":"...","approver":"alice","role":"research-lead"}}}' | dear-agent-mcp
```

`workflow-approve` and the MCP `workflow_approve` tool both write to
the `approvals` table; the runner picks up the decision on its next
poll and resumes the run.

### 7. Index outputs as searchable knowledge (≈2 min)

Declare an output with `durability: engram_indexed`:

```yaml
nodes:
  - id: research
    # ...
    outputs:
      report:
        path: "notes/{{ .RunID }}/report.md"
        content_type: text/markdown
        durability: engram_indexed     # ← Phase 3
```

When the node finishes, the engine writes the file to disk **and**
forwards it to `pkg/source.AddSource`. The default backend is
`pkg/source/sqlite` (FTS5 over the same file as `runs.db`).

Search across all runs:

```bash
dear-agent-search --db ./runs.db "routing strategies"
dear-agent-search --db ./runs.db --since 30d --cue research "topic"
dear-agent-search --db ./runs.db --json --work-item run-abc "topic"
```

Each result is annotated with the run/node that produced it, so you
can drill back into the work-item:

```text
[1] report.md  (run abc12345/research succeeded)
    workflow://abc12345/research/report
    Routing across primary/secondary/tertiary tiers...
```

### 8. Migrate legacy `FileState` snapshots (≈30s)

Pre-Phase-0 runs persisted as JSON via `FileState`. The migrate tool
folds them into `runs.db` without rewriting the runner:

```bash
workflow-migrate --db ./runs.db --workflow legacy-wf path/to/snap.json
workflow-migrate --db ./runs.db --workflow legacy-wf --dry-run path/to/snap.json
```

The migration is idempotent at the run-id level: re-running against
the same snapshot is a no-op. Snapshots without a `run_id` (truly
legacy) get a deterministic id derived from `sha256(workflow_name +
started)`.

---

## CLI reference

| Command | Purpose |
|---|---|
| `workflow-run` | Start a new run from a YAML file |
| `workflow-status` | One-shot run state, with `--json` and `--watch` |
| `workflow-list` | All runs, filterable by `--state` |
| `workflow-logs` | Per-attempt log for a run |
| `workflow-cancel` | Cancel an in-flight run |
| `workflow-approve` | Approve a HITL request, optionally with `--as <role>` |
| `workflow-lint` | Validate a YAML; `--check-deprecated-models` for role audits |
| `workflow-roles` | `list / describe / validate` against the registry |
| `workflow-migrate` | Port a `FileState` snapshot into `runs.db` |
| `dear-agent-mcp` | JSON-RPC server: `workflow_*` + `FetchSource` + `AddSource` |
| `dear-agent-search` | Search the knowledge corpus, joined to `runs` |
| `workflow-codemod` | v0.1 → v0.2 upgrade; `from-wayfinder` synthesis (Phase 4.1/4.2) |
| `workflow-dev` | Interactive dev shell with mocked-by-default fixtures (Phase 4.4) |
| `workflow-inspector` | Read-only HTML view of `runs.db` (Phase 5.5) |

---

## MCP tools

A vanilla MCP client (Claude Code, Cursor, custom agents) gets seven
tools out of the box:

| Tool | Purpose |
|---|---|
| `workflow_run` | Queue a new run from a YAML file (returns `run_id`) |
| `workflow_status` | Look up the current state of a `run_id` |
| `workflow_approve` | Resolve a HITL request as approved |
| `workflow_reject` | Resolve a HITL request as rejected |
| `workflow_cancel` | Cancel an in-flight run |
| `FetchSource` | Search the knowledge corpus (Phase 3) |
| `AddSource` | Add or update a knowledge entry (Phase 3) |

Backend mismatches on `FetchSource` / `AddSource` return JSON-RPC
error code `-32004` with `expected` / `actual` fields, so a
misconfigured client gets a clear signal rather than silent results.

---

## Where rows live

| Table | Written by | Read by |
|---|---|---|
| `workflows` | runner (per workflow definition hash) | every CLI and MCP tool |
| `runs` | runner; `workflow-migrate`; `workflow_run` MCP | `workflow-status / list`, `dear-agent-search` join |
| `nodes` | runner; `workflow-migrate` | `workflow-status` |
| `node_attempts` | runner | `workflow-logs` |
| `node_outputs` | `OutputWriter` (Phase 1.6) | future inspector |
| `audit_events` | every state transition | `workflow-logs`, audit sinks (JSONL, OTel, Engram) |
| `approvals` | HITL backend (CLI / Discord / MCP) | runner poll loop, `workflow-approve` |
| `sources` (+`sources_fts`) | `pkg/source/sqlite` adapter | `dear-agent-search`, `FetchSource` MCP |

The default deployment puts every table in **one** SQLite file. JOINs
across runs ↔ sources are local; nothing has to be fanned out across
processes. Override with `--sources` on the MCP / search CLI if you
want the knowledge corpus in a separate file.

---

## Phase status (as of 2026-05-03)

| Phase | Scope | Status |
|---|---|---|
| 0 | SQLite + audit_events | done (#38) |
| 1 | Roles + budget | done (#39) |
| 2 | DEAR hooks + HITL + audit sinks + MCP + Discord | done (#40) |
| 3 | FetchSource / AddSource (this guide is current as of here) | done |
| 4 | Migration + `workflow dev` | done — `workflow-migrate`, `workflow-codemod`, `workflow-dev` |
| 5 | Adapters + visual inspector + `kind: spawn` | done — Obsidian/llm-wiki/registry adapters, `workflow-inspector`, `kind: spawn`; OpenViking ships as a stub |

For per-ticket detail see [BACKLOG.md](workflow-engine/BACKLOG.md).
