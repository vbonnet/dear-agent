# VROOM Process-Isolated Supervisors

Each VROOM supervisor (Meta-Orchestrator, Orchestrator, Overseer) runs as a
separate AGM-managed harness session instead of a goroutine in one process.
`pkg/vroom/supervisor` is the canonical source for their session identities,
compact heartbeat aliases, roles, and Primary/Tertiary peer relationships.
`vroom-dispatch` adds harness, model, skill, and tick policy to those members.

## SKILL Files

The operational instructions for each supervisor live in
`cmd/vroom-dispatch/skills/` (embedded in the `vroom-dispatch` binary):

| File | Supervisor | Tick interval |
|------|-----------|---------------|
| `meta-orchestrator.md` | Meta-O (CTO) — Beads backlog quality, prioritization | 3 min |
| `orchestrator.md` | Orch (COO) — dispatch, worker monitoring | 90s |
| `overseer.md` | Overseer (CRO) — resources, health, cleanup | 60s |
| `protocol.md` | Shared state protocol (all three read) | — |

## Launcher

```bash
make install-vroom-dispatch

vroom-dispatch                  # full launch: install skills, create sessions, start loops
vroom-dispatch --skills-only    # just install SKILL files to ~/.agm/vroom/skills/
vroom-dispatch --boot-only      # install + create sessions, don't start loops
vroom-dispatch --loop-only      # start loops on existing sessions
vroom-dispatch --status         # show mesh health
```

## Shared State

The process-isolated mesh uses this local state:

```
~/.agm/vroom/
├── trail.jsonl          # Typed AGM components append decision evidence
├── skills/              # Installed SKILL files
└── heartbeat/           # Per-supervisor heartbeat files
```

Beads, live AGM sessions, and open GitHub PRs are the operational sources of
truth. The retired roadmap, dispatch-ledger, deploy-ledger, and prompt-file
projections are not part of the process-isolated architecture. Supervisor
prompts do not hand-build JSONL; typed Go components own encoding and writes.

`agm supervisor heartbeat` stores the authoritative record under
`~/.agm/supervisors/<canonical-id>/heartbeat.json` and mirrors it to the VROOM
directory using the canonical compact names `meta-o.json`, `orch.json`, and
`overseer.json`.

## Relationship to pkg/vroom/supervisor/

The `pkg/vroom/supervisor/` package defines the topology, Go interfaces, and
in-process mesh (used by `cmd/vroom-mesh`). The process-isolated model replaces
the in-process goroutines with separate AGM-managed harness sessions that
implement the same logic via their SKILL prompts.

The Go interfaces remain valuable as the canonical specification — the
SKILL files are the operational implementation of those interfaces.
