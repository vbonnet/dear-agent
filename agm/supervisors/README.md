# VROOM Process-Isolated Supervisors

Each VROOM supervisor (Meta-Orchestrator, Orchestrator, Overseer) runs as a
separate Claude Code session instead of goroutines in one process.

## SKILL Files

The operational instructions for each supervisor live in
`cmd/vroom-dispatch/skills/` (embedded in the `vroom-dispatch` binary):

| File | Supervisor | Tick interval |
|------|-----------|---------------|
| `meta-orchestrator.md` | Meta-O (CTO) — roadmap, prioritization | 3 min |
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

All supervisors read/write from `~/.agm/vroom/`:

```
~/.agm/vroom/
├── trail.jsonl          # Append-only decision log (all 3 write)
├── roadmap.jsonl        # Accepted proposals (Meta-O writes)
├── dispatched.jsonl     # Dispatch records (Orch writes)
├── skills/              # Installed SKILL files
└── heartbeat/           # Per-supervisor heartbeat files
```

## Relationship to pkg/vroom/supervisor/

The `pkg/vroom/supervisor/` package defines the Go interfaces and
in-process mesh (used by `cmd/vroom-mesh`). The process-isolated model
replaces the in-process goroutines with separate Claude Code sessions
that implement the same logic via their SKILL prompts.

The Go interfaces remain valuable as the canonical specification — the
SKILL files are the operational implementation of those interfaces.
