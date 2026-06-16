# VROOM Supervisor Shared Protocol

This document defines the shared state, file formats, and communication
conventions used by all three VROOM supervisors (Meta-Orchestrator,
Orchestrator, Overseer). Each supervisor reads this at boot.

## State Directory

All shared state lives under `~/.agm/vroom/`. Create it if missing:
```bash
mkdir -p ~/.agm/vroom/heartbeat
```

| File | Format | Writer | Readers |
|------|--------|--------|---------|
| `trail.jsonl` | JSONL append-only | All 3 | All 3 + humans |
| `roadmap.jsonl` | JSONL append-only | Meta-O only | Orch + all |
| `dispatched.jsonl` | JSONL append-only | Orch only | All |
| `heartbeat/meta-o.json` | JSON atomic-write | Meta-O | Orch, Overseer |
| `heartbeat/orch.json` | JSON atomic-write | Orch | Meta-O, Overseer |
| `heartbeat/overseer.json` | JSON atomic-write | Overseer | Meta-O, Orch |

## Record Formats

### Trail Record (all supervisors write)
One JSON object per line, appended via `>>`:
```json
{"ts":"2026-06-15T22:00:00Z","role":"meta-orchestrator","kind":"supervisor.metao.roadmap.evaluated","payload":{"bead_id":"ce-abc1","title":"Fix X","accepted":true}}
```
Fields: `ts` (RFC3339 UTC), `role` (your role name), `kind` (event topic), `payload` (free-form object).

Write via:
```bash
printf '{"ts":"%s","role":"%s","kind":"%s","payload":%s}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "<role>" "<kind>" '<json>' \
  >> ~/.agm/vroom/trail.jsonl
```

### Roadmap Record (Meta-O writes)
```json
{"bead_id":"ce-abc1","title":"Fix auth timeout","priority":"P1","state":"accepted","reason":"Blocking 3 other beads","decided_at":"2026-06-15T22:00:00Z"}
```
States: `accepted`, `rejected`.

### Dispatch Record (Orch writes)
```json
{"bead_id":"ce-abc1","session":"worker-ce-abc1","model":"opus","dispatched_at":"2026-06-15T22:05:00Z"}
```

## Heartbeat

Each supervisor writes its heartbeat using the existing AGM command:
```bash
agm supervisor heartbeat --id <supervisor-id> --primary-for <peer> --tertiary-for <peer>
```

This writes `~/.agm/supervisors/<id>/heartbeat.json`. Additionally, write a
simpler file for quick peer checks:
```bash
date -u +%Y-%m-%dT%H:%M:%SZ > ~/.agm/vroom/heartbeat/<name>.json
```

## Peer Liveness Check

Read the peer's heartbeat file. If missing or timestamp is >5 minutes old,
the peer is **stale**. Actions:
1. Record to trail: `kind: "supervisor.<role>.peer_stale"`
2. Send message: `agm send msg <peer> --priority urgent --sender <self> --prompt "status? Heartbeat stale."`
3. If >10 minutes: send critical priority

## Inter-Supervisor Messaging

For direct messages between supervisors:
```bash
agm send msg <target-session> --sender <self> --priority <level> --prompt "<message>"
```

Priority levels: `critical`, `urgent`, `normal`, `background`, `fyi`.

## Beads Access

Always use the canonical form with explicit database path:
```bash
bd --db ~/beads/context-engine/.beads <subcommand>
```
Never use bare `bd` — it silently resolves to the wrong database.

## Constraints (ALL supervisors)

- **NEVER** write to `~/src/**` (read-only golden checkouts)
- **NEVER** use `--no-verify` or `--force` flags
- **NEVER** use bare `bd` without `--db`
- **ALWAYS** use `GIT_TERMINAL_PROMPT=0 gtimeout 30` for git push operations
- **ALWAYS** write one JSON line at a time to JSONL files (atomic append)
- **ALWAYS** write heartbeat at the END of each tick (confirms tick completed)
