# Wayfinder Corpus Callosum Schema

This directory contains the Corpus Callosum schema definition for Wayfinder, enabling cross-component discovery and data validation.

## Overview

The Wayfinder schema registers with Corpus Callosum to enable:

- **Component Discovery**: Other tools can discover Wayfinder projects
- **Data Validation**: Validate WAYFINDER-STATUS.md files against schema
- **Cross-Component Queries**: Query Wayfinder project data from other tools
- **Graceful Degradation**: Wayfinder works normally without Corpus Callosum

## Schema Files

- `wayfinder-v1.schema.json` - Corpus Callosum schema definition
  - Defines `project` and `phase` schemas
  - Includes discovery patterns for finding Wayfinder projects
  - Documents v1 and v2 phase types
  - Examples for both project and phase data

## Scripts

- `scripts/register-schema.sh` - Register schema with Corpus Callosum
- `scripts/unregister-schema.sh` - Unregister schema from Corpus Callosum
- `scripts/test-cc-integration.sh` - Comprehensive integration tests

## Usage

### Register Schema

```bash
# Run during Wayfinder installation
./scripts/register-schema.sh
```

The script will:
1. Check if Corpus Callosum is installed
2. Register the Wayfinder schema
3. Verify registration succeeded
4. Exit gracefully if CC not available

### Verify Registration

```bash
# List all registered components
cc discover --format text

# Get Wayfinder schema details
cc discover --component wayfinder

# Get full schema definition
cc schema --component wayfinder
```

### Unregister Schema

```bash
# Run during Wayfinder uninstallation
./scripts/unregister-schema.sh
```

### Run Tests

```bash
# Comprehensive integration tests
./scripts/test-cc-integration.sh
```

Tests verify:
- Corpus Callosum availability
- Schema file validation
- Registration success
- Discovery patterns
- Schema structure
- Graceful degradation

## Schema Structure

### Project Schema

Represents Wayfinder project metadata from WAYFINDER-STATUS.md:

```json
{
  "session_id": "uuid",
  "project_path": "/path/to/project",
  "schema_version": "1.0",
  "version": "v1 or v2",
  "started_at": "ISO 8601 timestamp",
  "status": "in_progress|completed|abandoned|obsolete|blocked",
  "lifecycle_state": "working|input-required|dependency-blocked|...",
  "current_phase": "S8 or discovery.problem"
}
```

### Phase Schema

Represents individual phase metadata:

```json
{
  "name": "S8 or discovery.problem",
  "status": "pending|in_progress|completed|skipped",
  "started_at": "ISO 8601 timestamp",
  "completed_at": "ISO 8601 timestamp",
  "outcome": "success|partial|skipped"
}
```

## Discovery Patterns

The schema defines how to discover Wayfinder projects:

- **Directories**: `wf/`
- **Patterns**: `**/WAYFINDER-STATUS.md`
- **Exclusions**:
  - `**/.wayfinder/archives/**` (archived sessions)
  - `**/.worktrees/**` (git worktrees)

## Compatibility

- **Mode**: backward
- **Versioning**: Semantic versioning (MAJOR.MINOR.PATCH)
- **Current Version**: 1.0.0

New versions can read old data (backward compatible).

## Integration Points

### Component Installation

The Wayfinder component installer should call:

```bash
./schema/scripts/register-schema.sh
```

### Component Uninstallation

The Wayfinder component uninstaller should call:

```bash
./schema/scripts/unregister-schema.sh
```

### Graceful Degradation

All scripts check for Corpus Callosum availability and exit cleanly if not found. Wayfinder continues to work normally without Corpus Callosum integration.

## Metadata

The schema includes metadata for documentation:

- **Repository**: https://github.com/vbonnet/engram
- **Documentation**: https://github.com/vbonnet/engram/tree/main/core/docs/wayfinder
- **Phase Types**: Lists all v1 and v2 phase identifiers

## Testing

Run the test suite to verify integration:

```bash
./scripts/test-cc-integration.sh
```

Expected output: All 10 tests pass

## Troubleshooting

### "Corpus Callosum not found"

This is expected if CC is not installed. Wayfinder works without it.

### "Schema registration failed"

Check that:
1. Corpus Callosum is installed correctly
2. The schema file is valid JSON
3. You have write permissions to `~/.config/corpus-callosum/`

### "Component not found in discovery"

Re-register the schema:

```bash
./scripts/register-schema.sh
```

Verify with:

```bash
cc discover --component wayfinder
```

## Version History

- **1.0.0** (2026-02-19): Initial schema release
  - Project and phase schemas
  - Discovery patterns for wf/ directory
  - Support for both v1 and v2 Wayfinder workflows
  - Backward compatibility mode

## See Also

- [Corpus Callosum Documentation](../../../../ai-tools/main/corpus-callosum/README.md)
- [Wayfinder Documentation](../../../docs/wayfinder/)
- [Component Model Architecture](../../../../../../swarm/projects/modular-architecture-system/)
