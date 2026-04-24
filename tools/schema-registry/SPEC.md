# Corpus Callosum — Specification

## Overview

Corpus Callosum is a cross-component schema registry and data validation protocol.
It enables independent components (AGM, Wayfinder, Engram, etc.) to register schemas,
discover each other, and validate data against shared contracts.

## CLI Interface

Binary: `cc`

### Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `cc init` | Initialize registry for a workspace | `--workspace <name>` |
| `cc register` | Register a component schema | `--component`, `--schema <file>`, `--version`, `--compatibility` |
| `cc discover` | List registered components | `--component <name>` (optional, for details) |
| `cc schema` | Retrieve a schema definition | `--component`, `--schema-name` |
| `cc validate` | Validate data against a schema | `--component`, `--schema`, `--data <file>` |
| `cc query` | Query component metadata | `--component`, `--filter <json>`, `--sort`, `--limit` |
| `cc unregister` | Remove a component schema | `--component` |
| `cc status` | Show registry config and stats | — |

### Output Formats

- **JSON** (default): machine-readable structured output
- **Text** (`--format text`): human-friendly display

## MCP Server Interface

Binary: `cc-mcp-server`

Exposes 5 tools over JSON-RPC 2.0 for LLM integration:

| Tool | Description |
|------|-------------|
| `cc__discoverComponents` | List registered components |
| `cc__getComponentSchema` | Retrieve schema definitions |
| `cc__queryData` | Query component metadata with filters |
| `cc__registerSchema` | Register new schemas |
| `cc__validateData` | Validate data against registered schemas |

## Schema Format

Schemas use **JSON Schema Draft 2020-12**. Each registered schema has:

- **component**: owning component name
- **version**: semantic version string
- **compatibility mode**: one of `backward`, `forward`, `full`, `none`
- **schema body**: valid JSON Schema document

## Compatibility Modes

| Mode | Rule |
|------|------|
| `backward` | New schema can read data written by old schema |
| `forward` | Old schema can read data written by new schema |
| `full` | Both backward and forward compatible |
| `none` | No compatibility guarantee |

Compatibility is checked automatically on `cc register` when a prior version exists.

## Go Library API

```go
import "corpus-callosum/internal/registry"

reg, err := registry.New("oss")
defer reg.Close()

// Register
err = reg.RegisterSchema("agm", "1.0.0", "backward", schemaBytes)

// Query
schema, err := reg.GetSchema("agm", "1.0.0")
components, err := reg.ListComponents()

// Validate
result, err := reg.ValidateData("agm", "session", dataBytes)
```

## Configuration

Registry databases are stored per-workspace:

```
~/.config/corpus-callosum/
├── oss/registry.db
├── acme/registry.db
└── global/registry.db
```

Workspace is auto-detected from `~/src/ws/{workspace}/*` path patterns.

## Error Handling

- Missing component → `component not found` error
- Invalid schema → JSON Schema validation errors with paths
- Incompatible version → compatibility check failure with diff details
- Missing database → prompt to run `cc init`

## Dependencies

- Go 1.25.0+
- `github.com/mattn/go-sqlite3` — database driver
- `github.com/xeipuuv/gojsonschema` — JSON Schema validation
- `gopkg.in/yaml.v3` — YAML parsing
