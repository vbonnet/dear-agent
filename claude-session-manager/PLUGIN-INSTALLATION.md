# AGM - Plugin Installation

This repository provides Claude Code slash commands for Agent Gateway Manager (AGM) through the plugin marketplace system.

## Installation

### Option 1: Add Marketplace via Command (Recommended)

From within any Claude Code session, run:

```
/plugin marketplace add ~/src/repos/ai-tools/main/claude-session-manager
```

Then install the plugin:

```
/plugin install agm@ai-tools
```

### Option 2: Configure in Settings

Add to your `.claude/settings.json`:

```json
{
  "extraKnownMarketplaces": {
    "ai-tools": {
      "source": "~/src/repos/ai-tools/main/claude-session-manager"
    }
  }
}
```

Then restart Claude Code and run:

```
/plugin install agm
```

### Option 3: GitHub-based (For Shared Teams)

If this repo is on GitHub:

```
/plugin marketplace add your-org/ai-tools
/plugin install agm@ai-tools
```

## Available Commands

After installation, the following slash commands will be available:

- `/agm:assoc <session-name>` - Associate current Claude session with an AGM session

## Verification

List installed plugins:

```
/plugin list
```

View available commands:

```
/help
```

The AGM commands should appear with "(agm)" suffix.

## Updating

When commands are updated in the repository:

```
/plugin update agm
```

Or reinstall:

```
/plugin uninstall agm
/plugin install agm@ai-tools
```

## Requirements

- Claude Code CLI installed
- AGM binary installed (`make install` for the binary)
- This repository cloned locally

## See Also

- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Slash Commands Documentation](https://code.claude.com/docs/en/slash-commands)
