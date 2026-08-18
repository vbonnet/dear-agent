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

This registers the marketplace and installs every plugin it declares (`agm`,
`wayfinder`, `youtube`). It is idempotent — re-running just refreshes the
marketplace and updates each plugin to the version declared in
`marketplace.json`. Restart Claude Code afterward to pick up the new commands.

Common flags:

```bash
./scripts/install-claude-plugins.sh --github     # install from github.com/vbonnet/dear-agent
./scripts/install-claude-plugins.sh --dry-run    # preview without changes
./scripts/install-claude-plugins.sh --uninstall  # remove every dear-agent plugin
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

# Or from GitHub:
claude plugin marketplace add vbonnet/dear-agent
claude plugin install agm@dear-agent wayfinder@dear-agent youtube@dear-agent
```

## Available plugins

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

## See also

- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Slash Commands](https://code.claude.com/docs/en/slash-commands)
- `tests/bats/install-claude-plugins.bats` — coverage for the install script.
