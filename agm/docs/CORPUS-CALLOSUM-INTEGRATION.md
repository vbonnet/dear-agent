# AGM Corpus Callosum Integration

This document describes AGM's integration with the Corpus Callosum protocol for cross-component knowledge sharing and schema discovery.

## Overview

AGM registers its data schemas with Corpus Callosum to enable:
- **Component Discovery**: Other components can discover AGM and its capabilities
- **Schema Validation**: Validate session and message data against registered schemas
- **Cross-Component Queries**: Query AGM data from other components (when integrated)
- **Schema Versioning**: Track schema evolution with compatibility guarantees

## Registered Schemas

AGM registers two primary schemas:

### 1. Session Schema (`agm.session`)

Defines AGM conversation session metadata including:
- `id` - Unique session identifier (UUID)
- `name` - Human-readable session name
- `timestamp` - Creation timestamp (Unix milliseconds)
- `agent_type` - LLM agent type (claude, gemini, gpt, o1)
- `model` - Specific model version
- `mode` - Agent operating mode (implementer, researcher, reviewer, planner)
- `status` - Session lifecycle status (active, archived, terminated)
- `workspace` - **Workspace name** (oss, acme, etc.) - determines session storage location
- `tmux_session` - Associated tmux session name
- `manifest_path` - Absolute path to session manifest.yaml

**Required fields**: `id`, `name`, `timestamp`, `agent_type`

**Workspace-Aware Paths**: Sessions are stored in workspace-specific directories:
- Workspace sessions: `~/src/ws/{workspace}/.agm/sessions/{session-name}/manifest.yaml`
- Fallback path (no workspace): `~/.claude/sessions/{session-name}/manifest.yaml`

### 2. Message Schema (`agm.message`)

Defines individual messages within a session:
- `id` - Unique message identifier (UUID)
- `session_id` - Parent session UUID
- `role` - Message role (user, assistant, system)
- `content` - Message content (text or JSON)
- `timestamp` - Message timestamp (Unix milliseconds)
- `tokens` - Token usage statistics (input, output, cache_write, cache_read)

**Required fields**: `id`, `session_id`, `role`, `content`, `timestamp`

## Schema Registration

### Automatic Registration (Post-Install)

AGM schemas are automatically registered during component installation via the post-install hook:

```bash
./scripts/post-install.sh <workspace-root>
```

This calls `./scripts/register-corpus-callosum.sh` which registers the schema if Corpus Callosum is available.

### Manual Registration

To manually register AGM schemas:

```bash
cd agm
./scripts/register-corpus-callosum.sh
```

Or using the `cc` CLI directly:

```bash
cc register --component agm \
  --schema schemas/corpus-callosum-schema.json \
  --workspace oss
```

## Usage Examples

### Discover AGM Component

```bash
# List all registered components
cc discover --workspace oss

# Get AGM component details
cc discover --component agm --workspace oss
```

### Retrieve AGM Schemas

```bash
# Get full AGM schema
cc schema --component agm --workspace oss

# Get specific schema definition
cc schema --component agm --schema-name session --workspace oss
cc schema --component agm --schema-name message --workspace oss
```

### Validate Data

Validate session data:

```bash
# Create test session data
cat > /tmp/session.json << 'EOF'
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "my-test-session",
  "timestamp": 1708300800000,
  "agent_type": "claude",
  "model": "claude-sonnet-4.5",
  "status": "active"
}
EOF

# Validate against schema
cc validate --component agm \
  --schema session \
  --data /tmp/session.json \
  --workspace oss
```

Validate message data:

```bash
# Create test message data
cat > /tmp/message.json << 'EOF'
{
  "id": "660f8511-f39c-51e5-b827-557766551111",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "role": "user",
  "content": "Hello, AGM!",
  "timestamp": 1708300800000,
  "tokens": {
    "input": 10,
    "output": 0,
    "cache_write": 5,
    "cache_read": 0
  }
}
EOF

# Validate against schema
cc validate --component agm \
  --schema message \
  --data /tmp/message.json \
  --workspace oss
```

## Testing Integration

Run the integration test suite:

```bash
cd agm
CC_BIN=/path/to/cc ./scripts/test-corpus-callosum-integration.sh
```

The test suite verifies:
1. Corpus Callosum CLI is available
2. AGM component is registered
3. AGM schema can be retrieved
4. Session data validates correctly
5. Message data validates correctly
6. Schema metadata is correct (version, compatibility)

## Schema Compatibility

AGM uses **backward compatibility** mode (`"compatibility": "backward"`):
- New schema versions can read old data
- Consumers can upgrade gradually
- Breaking changes require major version bump

### Version History

- **v1.1.0** (2026-03-13): Workspace-aware path integration
  - Updated schema examples to reflect workspace-specific paths
  - Documented workspace field as key session attribute
  - Added workspace isolation guidance for component integration
  - Sessions stored in `~/src/ws/{workspace}/.agm/sessions/` or `~/.claude/sessions/` (fallback)

- **v1.0.0** (2026-02-21): Initial schema registration
  - Session schema with core metadata fields
  - Message schema with token tracking
  - Backward compatibility mode

## Integration with Other Components

Components that integrate with AGM via Corpus Callosum:

- **Wayfinder**: May query AGM sessions for context-aware navigation
- **Engram**: Provides workspace detection to AGM (dependency)
- **Future components**: Can discover and query AGM session data

**Workspace Isolation**: Sessions are isolated by workspace. Components querying AGM data should specify the target workspace to retrieve workspace-specific sessions. Sessions with empty `workspace` field are stored in the fallback path (`~/.claude/sessions/`).

## Graceful Degradation

AGM works without Corpus Callosum installed:
- Schema registration is optional (non-fatal if cc CLI not found)
- AGM core functionality remains intact
- Discovery and cross-component queries are unavailable

## Files

- `schemas/corpus-callosum-schema.json` - AGM schema definition
- `scripts/register-corpus-callosum.sh` - Registration script
- `scripts/test-corpus-callosum-integration.sh` - Integration test suite
- `scripts/post-install.sh` - Post-install hook (includes registration)

## Future Enhancements

Planned improvements for Corpus Callosum integration:

1. **Query Integration**: Support `cc query` for session data retrieval
2. **Embedding Schema**: Register schema for semantic search embeddings
3. **Tool Call Schema**: Track tool usage across sessions
4. **Tag Schema**: Session categorization and filtering
5. **Real-time Updates**: Notify other components of session changes

## Troubleshooting

### Schema not found

```bash
# Re-register schema
cd agm
./scripts/register-corpus-callosum.sh
```

### Validation errors

Common validation errors:
- **UUID format**: IDs must be valid UUIDs (e.g., `550e8400-e29b-41d4-a716-446655440000`)
- **Required fields**: Ensure `id`, `name`, `timestamp`, `agent_type` are present for sessions
- **Enum values**: `agent_type` must be one of: claude, gemini, gpt, o1
- **Timestamp type**: Must be integer (Unix milliseconds), not string

### Corpus Callosum not found

If `cc` CLI is not found:
1. Build corpus-callosum: `cd ../corpus-callosum && make build`
2. Install to PATH: `make install`
3. Verify: `cc version`

## References

- [Corpus Callosum Protocol](../../corpus-callosum/README.md)
- [Corpus Callosum API](../../corpus-callosum/API.md)
- [AGM Component Manifest](../component.yaml)
- [AGM Schema Definition](../schemas/corpus-callosum-schema.json)
