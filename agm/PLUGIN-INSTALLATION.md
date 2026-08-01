# Plugin Installation

This repository ships its slash commands and skills through the Claude Code
plugin marketplace system. The harness-neutral
[`.dear-agent/marketplace.json`](../.dear-agent/marketplace.json) is the canonical
plugin inventory. Claude installs from the parity-checked
[native mirror](../.claude-plugin/marketplace.json), whose marketplace name is
**`dear-agent`**.

## Recommended: the install script

From a local clone of this repo:

```bash
./scripts/install-claude-plugins.sh
```

This registers the marketplace and installs every plugin declared by its
manifest. It is idempotent — re-running just refreshes the marketplace and
updates each plugin to the declared version. Any declared-plugin update or
install failure stops the script non-zero without a success message, so rerun it
after fixing the Claude CLI failure rather than treating a partial or stale
plugin set as current. Restart Claude Code afterward to pick up the new commands
and skills.

Common flags:

```bash
./scripts/install-claude-plugins.sh --github     # install from github.com/vbonnet/dear-agent
./scripts/install-claude-plugins.sh --dry-run    # preview without changes
./scripts/install-claude-plugins.sh --uninstall  # remove every dear-agent plugin
./scripts/install-claude-plugins.sh --scope user # forward --scope to claude plugin install
./scripts/install-claude-plugins.sh --help       # full help
```

## Manual install of one plugin

The script is the supported bulk-install path. To use the underlying `claude`
CLI for one plugin, register either source and choose a name from the canonical
neutral catalog:

```bash
# From a local clone:
claude plugin marketplace add ~/src/dear-agent

# Or from GitHub:
claude plugin marketplace add vbonnet/dear-agent

# Then install any declared plugin, for example:
claude plugin install spec-governance@dear-agent
```

## Available plugins

The [neutral catalog](../.dear-agent/marketplace.json) is the canonical
inventory; the [Claude marketplace manifest](../.claude-plugin/marketplace.json)
is its parity-checked native mirror and describes each plugin's Claude commands
or skills. For example, the AGM plugin exposes `/agm:agm-assoc`. Use
`claude plugin list` and `claude plugin details <name>@dear-agent` to inspect the
installed snapshot.

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
work until you also install their execution prerequisites:

- **`agm` plugin** — `agm` binary (`go install
  github.com/vbonnet/dear-agent/agm/cmd/agm@latest`) and `tmux`.
- **`spec-governance` plugin** — the `audit-specs` collector requires an
  absolute Go 1.26.5 or newer executable selected from the caller's `PATH`.
  Its isolated module is standard-library-only and runs with module lookup
  disabled, including with fresh empty Go caches; `write-spec` does not invoke
  the collector.
- **`youtube` plugin** — `yt-dlp` (`brew install yt-dlp` /
  `pipx install yt-dlp`).

## See also

- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Slash Commands](https://code.claude.com/docs/en/slash-commands)
- `tests/bats/install-claude-plugins.bats` — coverage for the install script.
