# workspace-manager

Generic workspace structure manager for multi-session AI development.

## Overview

workspace-manager provides a CLI tool (`session`) for managing development sessions across multiple projects. It handles:
- Session lifecycle (create, resume, list, cleanup)
- Claude Code integration (session discovery, UUID mapping)
- Tmux integration (automatic session creation/attachment)
- YAML-based manifest management

**Key feature**: Works with any AI coding agent - not specific to Engram or Claude.

## Installation

```bash
# Clone ai-tools repository
git clone https://github.com/vbonnet/ai-tools
cd ai-tools/workspace-manager

# Symlink to your PATH
mkdir -p ~/.local/bin
ln -s "$(pwd)/bin/session" ~/.local/bin/session

# Verify installation
session --help
```

## Quick Start

```bash
# List all sessions
session list

# Discover Claude sessions from history.jsonl
session sync

# Resume a session
session resume claude-1
```

## Documentation

- **User Guide**: [docs/SESSION-CLI-README.md](docs/SESSION-CLI-README.md)
- **Debugging Notes**: [docs/DEBUGGING-SESSION.md](docs/DEBUGGING-SESSION.md)
- **Design Documentation**: See `docs/` directory for full Wayfinder process artifacts

## Requirements

- Bash 4.0+
- Python 3.x (for JSON parsing)
- tmux (optional, for tmux integration)
- Claude Code (optional, for Claude integration)

## Features

- **Session Discovery**: Auto-discover Claude sessions from `~/.claude/history.jsonl`
- **Three-way Mapping**: tmux name ↔ workspace ID ↔ Claude UUID
- **Health Checks**: CWD deleted bug detection, corruption recovery
- **Active Filtering**: Smart detection of active vs test sessions
- **Manifest Management**: YAML-based session metadata

## Status

**Alpha quality** - Bash prototype, works but Go rewrite planned.

## License

MIT License - see [../LICENSE](../LICENSE)

## Relationship to Other Tools

- **Independent**: Works without Engram or specific AI agents
- **Claude Code**: Optional integration for session management
- **Future**: claude-session-manager will be Go rewrite of Claude-specific features
