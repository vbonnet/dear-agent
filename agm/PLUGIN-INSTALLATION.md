# Plugin Installation

This repository provides Claude Code slash commands through the plugin marketplace system.

## Installation

### Option 1: Add Marketplace via Command (Recommended)

From within any Claude Code session, run:

```
/plugin marketplace add ~/src/repos/ai-tools/main/agm
```

Then install plugins:

```
/plugin install agm@ai-tools
/plugin install youtube@ai-tools
```

### Option 2: Configure in Settings

Add to your `.claude/settings.json`:

```json
{
  "extraKnownMarketplaces": {
    "ai-tools": {
      "source": "~/src/repos/ai-tools/main/agm"
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

## Available Plugins

### AGM (`/plugin install agm@ai-tools`)

- `/agm:assoc <session-name>` - Associate current Claude session with an AGM session
- `/agm:exit` - Exit Claude and archive AGM session automatically

### YouTube (`/plugin install youtube@ai-tools`)

- `/youtube <url-or-video-id>` - Extract transcript from a YouTube video

Requires: `yt-dlp` (`brew install yt-dlp`)

## Verification

List installed plugins:

```
/plugin list
```

View available commands:

```
/help
```

The commands should appear with their plugin suffix (e.g., "(agm)", "(youtube)").

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
- This repository cloned locally
- **AGM plugin**: AGM binary installed (`make install`)
- **YouTube plugin**: yt-dlp installed (`brew install yt-dlp`)

## See Also

- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Slash Commands Documentation](https://code.claude.com/docs/en/slash-commands)
