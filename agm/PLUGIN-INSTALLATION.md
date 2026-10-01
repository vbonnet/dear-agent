# Plugin Installation

This repository ships its slash commands and skills through the Claude Code
plugin marketplace system. The marketplace is named **`dear-agent`** and is
defined by [`.claude-plugin/marketplace.json`](../.claude-plugin/marketplace.json)
at the repo root.

## Recommended: the install script

From a local clone of this repo:

```bash
./scripts/install-claude-plugins.sh
```

This registers the marketplace and bulk-manages the historical four-plugin set:
`agm`, `wayfinder`, `youtube`, and `research-pipeline`. It is idempotent —
re-running just refreshes the marketplace and updates those four plugins to the
versions declared in `marketplace.json`. Restart Claude Code afterward to pick
up the new commands.

The Claude source catalog also declares a `spec-governance` projection. It is
deliberately excluded from this script's install, update, and uninstall actions.
Catalog and source validation do not prove that projection is registered,
installed, enabled, discovered, invoked, or loaded at runtime.

Common flags:

```bash
./scripts/install-claude-plugins.sh --github     # install from github.com/vbonnet/dear-agent
./scripts/install-claude-plugins.sh --dry-run    # preview without changes
./scripts/install-claude-plugins.sh --uninstall  # remove the four bulk-managed plugins
./scripts/install-claude-plugins.sh --scope user # forward --scope to claude plugin install
./scripts/install-claude-plugins.sh --help       # full help
```

## Manual install (equivalent commands)

If you prefer to run the underlying `claude` CLI yourself:

```bash
# From a local clone:
claude plugin marketplace add ~/src/dear-agent
claude plugin install agm@dear-agent
claude plugin install wayfinder@dear-agent
claude plugin install youtube@dear-agent
claude plugin install research-pipeline@dear-agent

# Or from GitHub:
claude plugin marketplace add vbonnet/dear-agent
claude plugin install agm@dear-agent wayfinder@dear-agent youtube@dear-agent research-pipeline@dear-agent
```

## Bulk-installed plugins

After install, the following are exposed:

- **`agm@dear-agent`** — session and orchestration commands: `/agm:agm-assoc`,
  `/agm:agm-exit`, `/agm:agm-list`, `/agm:agm-new`, `/agm:agm-resume`,
  `/agm:agm-search`, `/agm:agm-send`, `/agm:agm-status`,
  `/agm:audit-completion`, `/agm:wiki-ingest`, `/agm:wiki-lint`,
  `/agm:wiki-query-save`, and the `scan-health` skill.
- **`wayfinder@dear-agent`** — the top-level `wayfinder` skill (9-phase SDLC
  workflow); it does not install slash commands.
- **`youtube@dear-agent`** — `/youtube:youtube` for transcript extraction
  (needs `yt-dlp`).
- **`research-pipeline@dear-agent`** — the portable `research-pipeline` skill
  for source collection, evidence synthesis, and a human-approved Beads
  handoff; Wayfinder may govern later delivery when the target requires it.

## Verification

```bash
claude plugin list                    # confirms each plugin is enabled
claude plugin details agm@dear-agent  # lists exposed commands/skills
```

Slash commands also appear in `/help` once Claude Code is restarted.

## Updating

```bash
./scripts/install-claude-plugins.sh           # re-run; idempotent
# or
claude plugin update agm@dear-agent
```

## External prerequisites

The plugins themselves install cleanly without these, but some commands won't
work until you also install:

- **`agm` plugin** — `agm` binary (`go install
  github.com/vbonnet/dear-agent/agm/cmd/agm@latest`) and `tmux`.
- **`youtube` plugin** — `yt-dlp` (`brew install yt-dlp` /
  `pipx install yt-dlp`).
- **`research-pipeline` plugin** — access to a second model for Stage 3
  verification/planning and a different third model for Stage 4 decomposition,
  plus the Beads `bd` CLI on `PATH` for filing that decomposition. If either
  independent model is unavailable, the plugin stops at the last completed
  artifact, reports the next-stage external handoff, and never substitutes the
  authoring model or claims the blocked stage ran. Stage 5 additionally
  requires a reachable Codex execution surface: either a configured `codex`
  CLI/account or a repository-approved dispatcher that can launch Codex
  `/goal` runs. Without that surface, the plugin stops after the reviewed bead
  graph, reports the external handoff explicitly, and does not claim that
  implementation ran.

## See also

- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Slash Commands](https://code.claude.com/docs/en/slash-commands)
- `tests/bats/install-claude-plugins.bats` — coverage for the install script.
