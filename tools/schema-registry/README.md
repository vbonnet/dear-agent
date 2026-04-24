# Corpus Callosum

**Cross-component knowledge sharing and data aggregation protocol for modular AI tools**

Corpus Callosum provides schema registration, component discovery, and query capabilities through a centralized registry with dual CLI and MCP interfaces.

## Overview

The Corpus Callosum Protocol enables independent components (AGM, Wayfinder, Engram, etc.) to:

- **Register schemas** for their data structures
- **Discover** other components and their capabilities
- **Query** data across components (when integrated)
- **Validate** data against registered schemas
- **Version** schemas with compatibility checking

**Name Origin**: In neuroscience, the corpus callosum connects the brain's two hemispheres enabling communication between specialized regions. Similarly, this protocol connects independent components while maintaining component autonomy.

## Features

- **Optional Plugin**: Components work without Corpus Callosum installed (graceful degradation)
- **Workspace Isolation**: Separate registries for OSS vs Acme Corp workspaces
- **Dual Interface**: Both CLI (humans/scripts) and MCP (LLMs) access
- **Compatibility Checking**: 4 modes - backward, forward, full, none
- **JSON Schema**: Industry-standard schema format (Draft 2020-12)

## Installation

### From Source

```bash
cd corpus-callosum
make build
make install
```

Binaries are installed to `$GOPATH/bin/`:
- `cc` - CLI tool
- `cc-mcp-server` - MCP server for Claude Desktop

### Verify Installation

```bash
cc version
```

## Quick Start

### Initialize Workspace

```bash
# Auto-detect workspace from current directory
cd ~/projects/myworkspace
cc init

# Or specify workspace explicitly
cc init --workspace acme
```

### Register a Schema

Create a schema file (`agm-schema.json`):

```json
{
  "$schema": "https://corpus-callosum.dev/schema/v1",
  "component": "agm",
  "version": "1.0.0",
  "compatibility": "backward",
  "description": "Agent Management System - session and message storage",
  "schemas": {
    "session": {
      "type": "object",
      "description": "AGM conversation session",
      "properties": {
        "id": {
          "type": "string",
          "description": "Unique session identifier (UUID)"
        },
        "timestamp": {
          "type": "integer",
          "description": "Unix timestamp (milliseconds)"
        },
        "user_id": {
          "type": "string",
          "description": "User who created session",
          "default": "unknown"
        },
        "model": {
          "type": "string",
          "description": "LLM model used",
          "enum": ["claude-opus-4.6", "gemini-2.0-flash-thinking"]
        }
      },
      "required": ["id", "timestamp"]
    }
  },
  "examples": {
    "session": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "timestamp": 1708300800000,
      "user_id": "alice",
      "model": "claude-opus-4.6"
    }
  }
}
```

Register it:

```bash
cc register --component agm --schema agm-schema.json
```

### Discover Components

```bash
# List all components
cc discover

# Get details for specific component
cc discover --component agm

# Output in text format
cc discover --format text
```

### Get Schema

```bash
# Get full component schema
cc schema --component agm

# Get specific schema definition
cc schema --component agm --schema-name session

# Get specific version
cc schema --component agm --version 1.0.0
```

### Validate Data

```bash
# Validate data from file
cc validate --component agm --schema session --data session-data.json

# Validate from stdin
echo '{"id": "test-123", "timestamp": 1708300800000}' | \
  cc validate --component agm --schema session --data -
```

### Check Status

```bash
cc status
```

## CLI Commands

All commands support JSON output (default) or text output (`--format text`):

| Command | Description |
|---------|-------------|
| `cc version` | Display CLI and protocol versions |
| `cc init` | Initialize registry in workspace |
| `cc status` | Show status and configuration |
| `cc register` | Register component schema |
| `cc unregister` | Unregister schema (with confirmation) |
| `cc discover` | List registered components |
| `cc schema` | Get schema definition |
| `cc query` | Query component data (requires integration) |
| `cc validate` | Validate data against schema |

## MCP Server (Claude Desktop Integration)

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "corpus-callosum": {
      "command": "cc-mcp-server"
    }
  }
}
```

With environment variables:

```json
{
  "mcpServers": {
    "corpus-callosum": {
      "command": "cc-mcp-server",
      "env": {
        "CC_WORKSPACE": "oss",
        "CC_VERBOSE": "1"
      }
    }
  }
}
```

### MCP Tools

The MCP server provides 5 tools for LLMs:

1. **cc__discoverComponents** - List registered components
2. **cc__getComponentSchema** - Get schema definitions
3. **cc__queryData** - Query component data
4. **cc__registerSchema** - Register new schemas
5. **cc__validateData** - Validate data against schemas

## Schema Format

Schemas follow JSON Schema Draft 2020-12 with Corpus Callosum extensions:

### Required Fields

- `$schema`: Protocol version URI
- `component`: Component identifier (lowercase, kebab-case)
- `version`: Semantic version (MAJOR.MINOR.PATCH)
- `schemas`: Map of schema names to JSON Schema definitions

### Optional Fields

- `compatibility`: "backward" (default), "forward", "full", "none"
- `description`: Human-readable component description
- `examples`: Example data for each schema
- `reserved`: Reserved field names (prevent reuse)
- `metadata`: Custom component metadata

### Compatibility Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `backward` | New schema reads old data | Default - consumers upgrade gradually |
| `forward` | Old schema reads new data | Producers upgrade first |
| `full` | Both directions | Maximum safety |
| `none` | No checking | Breaking changes allowed |

## Workspace Isolation

Each workspace has its own registry:

```
~/.config/corpus-callosum/
  oss/
    registry.db
  acme/
    registry.db
  global/
    registry.db
```

Workspace auto-detection pattern: `~/src/ws/{workspace}/*`

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Lint

```bash
make lint
```

### Clean

```bash
make clean
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Client Interfaces                        │
├──────────────────────┬──────────────────────────────────────┤
│   CLI Interface      │   MCP Server Interface               │
│   cc register        │   cc__registerSchema                 │
│   cc discover        │   cc__discoverComponents             │
│   cc query           │   cc__queryData                      │
└──────────┬───────────┴───────────────┬──────────────────────┘
           │                           │
           ▼                           ▼
┌─────────────────────────────────────────────────────────────┐
│              Corpus Callosum Core Library                    │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Schema Registry (SQLite)                             │  │
│  │  - Component schemas                                  │  │
│  │  - Version history                                    │  │
│  │  - Compatibility metadata                             │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Compatibility Checker                                │  │
│  │  - Backward/forward/full modes                        │  │
│  │  - Breaking change detection                          │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Related Documentation

- [API Documentation](./API.md) - Go library API reference
- [Examples](./EXAMPLES.md) - Real-world usage examples
- [Protocol Specification](../../swarm/projects/modular-architecture-system/specs/corpus-callosum-protocol.md)
- [CLI Specification](../../swarm/projects/modular-architecture-system/specs/corpus-callosum-cli.md)
- [MCP Specification](../../swarm/projects/modular-architecture-system/specs/corpus-callosum-mcp.md)

## Version

- **CLI Version**: 1.0.0
- **Protocol Version**: 1.0.0
- **MCP Protocol**: 2024-11-05

## License

See LICENSE file.

## Contributing

This is part of the modular-architecture-system project. See the project roadmap for contribution guidelines.
