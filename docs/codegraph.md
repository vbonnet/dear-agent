# codegraph — knowledge graphs for our codebases

We run two complementary tools to give Claude (and humans) up-to-date
context about our Go codebases:

| Tool       | Kind                      | What it gives you                                          |
|------------|---------------------------|------------------------------------------------------------|
| graphify   | static knowledge graph    | structural call/import graph, communities, "god nodes"     |
| gopls MCP  | live, type-aware queries  | symbol search, go-to-references, package APIs, diagnostics |

Use graphify to ask *"what is in this codebase, what's connected to what,
what are the central abstractions"*. Use gopls MCP to ask *"who calls
`Server.Run`", "what does the storage package export"*. They overlap a
little but mostly answer different questions.

## graphify (whole-repo knowledge graph)

[graphify](https://github.com/safishamsi/graphify) extracts a tree-sitter
AST from every code file, builds a NetworkX graph, runs Leiden
community detection, and writes:

- `graph.json` — the queryable graph (nodes + edges + communities)
- `GRAPH_REPORT.md` — top "god nodes", surprising connections, community list
- `graph.html` — interactive viz (only when the graph is under 5000 nodes)

Code-only extraction is **deterministic and free** (no LLM calls). Larger
docs/papers/images would call the agent's API; we don't use that.

### Where the outputs live

`~/.local/share/codegraph/<repo>/`. The repo name is derived from
`git remote origin` so worktrees and clones produce the same key.

```
~/.local/share/codegraph/
├── dear-agent/                  # this repo
│   ├── GRAPH_REPORT.md          # tens of thousands of nodes / edges
│   └── graph.json
└── brain-v2/                    # ~/src/brain-v2
    ├── GRAPH_REPORT.md          # 4k nodes / 5.5k edges / 346 communities
    ├── graph.html               # interactive viz (open in browser)
    └── graph.json
```

### Generating / refreshing

```sh
make codegraph-install     # one-time: installs graphify into ~/.local/venvs/graphify
make codegraph             # rebuild graph for this repo
make codegraph-all         # rebuild graphs for dear-agent and brain-v2

# Direct invocations:
scripts/codegraph                            # this repo
scripts/codegraph ~/src/brain-v2             # any path
scripts/codegraph install                    # bootstrap venv
```

The `update` mode is incremental — re-running on an unchanged tree only
rewalks the AST cache. A full dear-agent rebuild is ~30s on a warm cache.

### Querying without Claude

```sh
scripts/codegraph query   "where does workflow scheduling live?"
scripts/codegraph explain "NewError"
scripts/codegraph path    "Scheduler" "Runner"
```

`query` runs a BFS over the graph from likely-related symbols and prints
nodes with file/line/community. `explain` summarizes a node and its
neighbors. `path` finds the shortest connection between two named nodes.

### Reading the report

The first thing to read in `GRAPH_REPORT.md`:

- **God nodes** — the most-connected symbols. These are the central
  abstractions; if you don't know what they do, you don't know the repo.
- **Surprising connections** — INFERRED edges between symbols that aren't
  obviously related. Often points at hidden coupling or implicit contracts.
- **Communities** — Leiden clusters. Each is a roughly self-contained
  area of the codebase. The community labels are placeholder ("Community
  17"); read the member list and rename mentally.

### Gotchas

- `graphify update` looks at *code files*. Non-Go files in our tree
  (Markdown, JSON, YAML) aren't extracted. That's fine — we want the
  *code* graph.
- The HTML viz is skipped above 5000 nodes (configurable via
  `GRAPHIFY_VIZ_NODE_LIMIT`). For dear-agent, query the JSON instead.
- Node IDs are `{filename_stem}_{symbol}` lower-cased. When passing
  symbols to `path`/`explain`, use the human label or the ID.
- The graph is per-repo. Cross-repo queries need
  `graphify merge-graphs`; we haven't wired that up yet.

## gopls MCP server

[gopls v0.20+](https://go.dev/gopls/features/mcp) ships an experimental
MCP server. We register it in this repo's `.mcp.json` so Claude Code
sessions in this directory pick it up automatically.

### Setup

```sh
go install golang.org/x/tools/gopls@latest    # need v0.20+; we tested v0.21.1
```

Then in a fresh Claude Code session in this repo, the `gopls` MCP server
appears in `claude mcp list`. Tools it exposes:

| Tool                  | Use when                                                  |
|-----------------------|-----------------------------------------------------------|
| `go_workspace`        | first call in a Go session — tells gopls what's loaded    |
| `go_search`           | fuzzy-find a type/function/var by name                    |
| `go_file_context`     | summary of a file's intra-package dependencies            |
| `go_package_api`      | exported API of a package (great for third-party deps)    |
| `go_symbol_references`| find all callers of a symbol before refactoring           |
| `go_diagnostics`      | build/analysis errors after editing                       |
| `go_rename_symbol`    | type-safe rename across the workspace                     |
| `go_vulncheck`        | govulncheck across the module                             |

### When to use which

- *"What does this codebase look like?"* → graphify `GRAPH_REPORT.md`.
- *"What other functions in this package does `runner.go` use?"* → gopls
  `go_file_context`.
- *"What public API does `pkg/cliframe` expose?"* → gopls `go_package_api`.
- *"Who calls `Scheduler.Run`?"* → gopls `go_symbol_references`.
- *"Are there hidden cross-package couplings?"* → graphify "surprising
  connections" + community detection.

## Re-running

graphify outputs are cached at `~/.local/share/codegraph/<repo>/cache/`,
so re-running on an unchanged tree is cheap. Run `make codegraph-all` after
significant code churn — particularly after adding/removing packages or
moving large amounts of code between modules — so the graph stays a
useful answer to "what's in here right now."
