# Claude Session Manager - Plugin Installation

This repository provides Claude Code slash commands through the plugin marketplace system.

## Installation

### Option 1: Add Marketplace via Command (Recommended)

From within any Claude Code session, run:

```
/plugin marketplace add ~/src/repos/ai-tools/main/claude-session-manager
```

Then install the plugin:

```
/plugin install csm-tools@ai-tools
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
/plugin install csm-tools
```

### Option 3: GitHub-based (For Shared Teams)

If this repo is on GitHub:

```
/plugin marketplace add your-org/ai-tools
/plugin install csm-tools@ai-tools
```

## Available Commands

After installation, the following slash commands will be available:

- `/csm-assoc <session-name>` - Associate current Claude session with a CSM session

## Verification

List installed plugins:

```
/plugin list
```

View available commands:

```
/help
```

The CSM commands should appear with "(csm-tools)" suffix.

## Updating

When commands are updated in the repository:

```
/plugin update csm-tools
```

Or reinstall:

```
/plugin uninstall csm-tools
/plugin install csm-tools@ai-tools
```

## Requirements

- Claude Code CLI installed
- CSM binary installed (`make install` for the binary)
- This repository cloned locally

## See Also

- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Slash Commands Documentation](https://code.claude.com/docs/en/slash-commands)
