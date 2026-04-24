# Corpus Callosum Examples

Real-world usage examples for both CLI and MCP interfaces.

## CLI Examples

### Example 1: Component Installation Script

```bash
#!/bin/bash
# install-agm.sh - Install AGM and register with Corpus Callosum

set -e

echo "Installing AGM..."
# ... installation steps ...

# Check if Corpus Callosum is available
if command -v cc &> /dev/null; then
    echo "Registering AGM schema with Corpus Callosum..."
    cc register --component agm --schema ./agm-schema.json

    # Verify registration
    if cc discover --component agm | jq -e '.component' > /dev/null 2>&1; then
        echo "✓ AGM registered successfully"
    else
        echo "⚠ AGM registration may have failed"
    fi
else
    echo "Corpus Callosum not installed - skipping schema registration"
fi
```

### Example 2: Multi-Workspace Setup

```bash
#!/bin/bash
# setup-workspaces.sh - Initialize Corpus Callosum for multiple workspaces

WORKSPACES=("oss" "acme" "personal")

for workspace in "${WORKSPACES[@]}"; do
    echo "Initializing workspace: $workspace"
    cc init --workspace "$workspace"

    # Verify initialization
    status=$(cc status --workspace "$workspace" --format json)
    if echo "$status" | jq -e '.installed' > /dev/null; then
        echo "✓ $workspace initialized"
    else
        echo "✗ $workspace initialization failed"
    fi
done
```

### Example 3: Schema Version Upgrade

```bash
#!/bin/bash
# upgrade-schema.sh - Upgrade component schema with compatibility checking

COMPONENT="agm"
NEW_VERSION="1.1.0"
SCHEMA_FILE="./agm-schema-v1.1.0.json"

echo "Upgrading $COMPONENT to $NEW_VERSION..."

# Check current version
current=$(cc discover --component "$COMPONENT" | jq -r '.latest_version')
echo "Current version: $current"

# Attempt registration (will fail if incompatible)
if cc register --component "$COMPONENT" --schema "$SCHEMA_FILE"; then
    echo "✓ Successfully upgraded to $NEW_VERSION"

    # Show compatibility check result
    result=$(cc discover --component "$COMPONENT" | jq '.versions[0].compatibility')
    echo "Compatibility mode: $result"
else
    echo "✗ Upgrade failed - compatibility violation"
    echo "Use --force to skip compatibility check (not recommended)"
    exit 1
fi
```

### Example 4: Data Validation Pipeline

```bash
#!/bin/bash
# validate-sessions.sh - Validate all AGM session files

COMPONENT="agm"
SCHEMA="session"
SESSION_DIR="./sessions"

echo "Validating sessions in $SESSION_DIR..."

valid=0
invalid=0

for file in "$SESSION_DIR"/*.json; do
    if [ -f "$file" ]; then
        if cc validate --component "$COMPONENT" --schema "$SCHEMA" --data "$file" 2>/dev/null; then
            echo "✓ $(basename "$file")"
            ((valid++))
        else
            echo "✗ $(basename "$file")"
            ((invalid++))
        fi
    fi
done

echo ""
echo "Results: $valid valid, $invalid invalid"

if [ $invalid -gt 0 ]; then
    exit 1
fi
```

### Example 5: Query Component Schema Versions

```bash
#!/bin/bash
# query-versions.sh - Query all versions of a component schema

COMPONENT="agm"
SCHEMA="session"

echo "Querying $COMPONENT schema versions..."
echo ""

# Query all versions
echo "All versions:"
cc query --component "$COMPONENT" --format text

echo ""
echo "Latest version details (JSON):"
cc query --component "$COMPONENT" --limit 1 --sort created_at:desc

echo ""
echo "Specific schema details:"
cc query --component "$COMPONENT" --schema "$SCHEMA" --limit 1

echo ""
echo "Filter by version:"
cc query --component "$COMPONENT" --filter '{"version":"1.0.0"}'

echo ""
echo "Query with timestamp filter (created after timestamp):"
cc query --component "$COMPONENT" --filter '{"created_at_gt":1708300000000}'
```

### Example 6: Query Available Components

```bash
#!/bin/bash
# list-components.sh - List all components with their schemas

echo "Registered Components:"
echo "===================="

# Get component list as JSON
components=$(cc discover --format json | jq -r '.components')

# Iterate through components
echo "$components" | jq -r '.[] | "\(.component) v\(.latest_version)"' | while read -r line; do
    component=$(echo "$line" | cut -d' ' -f1)
    version=$(echo "$line" | cut -d' ' -f2)

    echo ""
    echo "$line"
    echo "$(printf '%.0s-' {1..40})"

    # Get schema names
    schemas=$(cc discover --component "$component" | jq -r '.schemas[]')
    echo "Schemas:"
    echo "$schemas" | while read -r schema; do
        echo "  - $schema"
    done
done
```

### Example 7: Export/Import Schemas

```bash
#!/bin/bash
# export-schemas.sh - Export all schemas to files

OUTPUT_DIR="./schema-export"
mkdir -p "$OUTPUT_DIR"

echo "Exporting schemas to $OUTPUT_DIR..."

# Get all components
components=$(cc discover | jq -r '.components[].component')

for component in $components; do
    echo "Exporting $component..."

    # Export full schema
    cc schema --component "$component" > "$OUTPUT_DIR/${component}.json"

    # Export versions
    versions=$(cc discover --component "$component" | jq -r '.versions[].version')
    mkdir -p "$OUTPUT_DIR/${component}-versions"

    for version in $versions; do
        cc schema --component "$component" --version "$version" \
            > "$OUTPUT_DIR/${component}-versions/${version}.json"
    done
done

echo "✓ Export complete"
```

## MCP Examples (LLM Interactions)

### Example 1: Component Discovery

**User**: "What components are registered in Corpus Callosum?"

**LLM Action**:
```
[Calls cc__discoverComponents]
```

**LLM Response**:
```
Found 3 components in the Corpus Callosum registry:

1. **agm** (v1.0.0)
   - Description: Agent Management System - session and message storage
   - Schemas: session, message

2. **wayfinder** (v2.1.0)
   - Description: Structured SDLC workflow manager
   - Schemas: task, milestone, dependency

3. **engram** (v1.5.0)
   - Description: Memory encoding system
   - Schemas: memory, context

Would you like to see the schema details for any component?
```

### Example 2: Schema Exploration

**User**: "Show me the AGM session schema"

**LLM Action**:
```
[Calls cc__getComponentSchema with component="agm", schemaName="session"]
```

**LLM Response**:
```
The AGM session schema defines conversation session metadata:

**Required Fields**:
- `id` (string): Unique session identifier (UUID format)
- `timestamp` (integer): Session creation time in Unix milliseconds

**Optional Fields**:
- `user_id` (string): User who created the session (default: "unknown")
- `model` (string): LLM model used (enum: claude-opus-4.6, gemini-2.0-flash-thinking)
- `status` (string): Session status (active, completed, or error)
- `tags` (array): Session tags for organization

**Example**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": 1708300800000,
  "user_id": "alice",
  "model": "claude-opus-4.6",
  "status": "active",
  "tags": ["research", "architecture"]
}
```

This schema uses backward compatibility mode, meaning new versions can read old data.
```

### Example 3: Data Validation

**User**: "Validate this session data: `{\"id\": \"test-123\", \"timestamp\": 1708300800000}`"

**LLM Action**:
```
[Calls cc__validateData with component="agm", schema="session", data={...}]
```

**LLM Response**:
```
✓ The data is valid according to the agm.session schema.

All required fields are present:
- `id`: "test-123" ✓
- `timestamp`: 1708300800000 ✓

The data can be safely stored as an AGM session.
```

### Example 4: Schema Registration

**User**: "Register a new component called 'task-tracker' with a simple task schema"

**LLM Action**:
```
[Calls cc__registerSchema with:
  component="task-tracker",
  schema={
    "$schema": "https://corpus-callosum.dev/schema/v1",
    "component": "task-tracker",
    "version": "1.0.0",
    "compatibility": "backward",
    "schemas": {
      "task": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "title": {"type": "string"},
          "completed": {"type": "boolean", "default": false}
        },
        "required": ["id", "title"]
      }
    }
  }
]
```

**LLM Response**:
```
✓ Successfully registered task-tracker v1.0.0

The schema has been added to the Corpus Callosum registry with:
- Component: task-tracker
- Version: 1.0.0
- Compatibility: backward mode
- Schemas: task

The task schema requires `id` and `title` fields, with an optional `completed` boolean (defaults to false).

Other components can now discover task-tracker and validate task data against this schema.
```

### Example 5: Version History Analysis

**User**: "Show me the version history for the AGM component"

**LLM Action**:
```
[Calls cc__discoverComponents with component="agm"]
```

**LLM Response**:
```
AGM version history:

**Current Version**: 1.0.0

**All Versions**:
1. v1.0.0 (2026-02-18 20:00:00)
   - Compatibility: backward
   - Initial release with session and message schemas

2. v0.9.5 (2026-02-11 15:30:00)
   - Compatibility: backward
   - Beta version

The component has been upgraded 1 time since initial development. All upgrades maintained backward compatibility, so old session data can be read by new versions.
```

## Advanced Examples

### Example 6: Component Health Check Script

```bash
#!/bin/bash
# health-check.sh - Verify all registered components are properly configured

echo "Corpus Callosum Health Check"
echo "============================"
echo ""

# Check CC installation
if ! command -v cc &> /dev/null; then
    echo "✗ Corpus Callosum not installed"
    exit 1
fi

echo "✓ Corpus Callosum installed ($(cc version --format json | jq -r '.cli'))"

# Check registry status
status=$(cc status --format json)
workspace=$(echo "$status" | jq -r '.workspace')
components=$(echo "$status" | jq -r '.components_registered')

echo "✓ Registry active (workspace: $workspace, components: $components)"
echo ""

# Validate each component
echo "Component Validation:"
echo "--------------------"

cc discover --format json | jq -r '.components[].component' | while read -r component; do
    echo -n "Checking $component... "

    # Get schema
    if ! schema=$(cc schema --component "$component" 2>&1); then
        echo "✗ Failed to retrieve schema"
        continue
    fi

    # Check if schema has examples
    has_examples=$(echo "$schema" | jq 'has("examples")')

    if [ "$has_examples" == "true" ]; then
        # Validate examples
        examples=$(echo "$schema" | jq -r '.examples | keys[]')

        for example_name in $examples; do
            example_data=$(echo "$schema" | jq ".examples[\"$example_name\"]")

            if echo "$example_data" | cc validate --component "$component" --schema "$example_name" --data - 2>/dev/null; then
                echo "✓"
            else
                echo "✗ Example validation failed"
            fi
        done
    else
        echo "⚠ No examples"
    fi
done

echo ""
echo "Health check complete"
```

### Example 7: Schema Diff Tool

```bash
#!/bin/bash
# schema-diff.sh - Compare two schema versions

COMPONENT="$1"
OLD_VERSION="$2"
NEW_VERSION="$3"

if [ -z "$COMPONENT" ] || [ -z "$OLD_VERSION" ] || [ -z "$NEW_VERSION" ]; then
    echo "Usage: $0 <component> <old-version> <new-version>"
    exit 1
fi

echo "Comparing $COMPONENT: $OLD_VERSION → $NEW_VERSION"
echo ""

# Get schemas
old_schema=$(cc schema --component "$COMPONENT" --version "$OLD_VERSION")
new_schema=$(cc schema --component "$COMPONENT" --version "$NEW_VERSION")

# Extract schema definitions
old_schemas=$(echo "$old_schema" | jq -r '.schemas | keys[]')
new_schemas=$(echo "$new_schema" | jq -r '.schemas | keys[]')

# Find added schemas
added=$(comm -13 <(echo "$old_schemas" | sort) <(echo "$new_schemas" | sort))
if [ -n "$added" ]; then
    echo "Added schemas:"
    echo "$added" | sed 's/^/  + /'
    echo ""
fi

# Find removed schemas
removed=$(comm -23 <(echo "$old_schemas" | sort) <(echo "$new_schemas" | sort))
if [ -n "$removed" ]; then
    echo "Removed schemas:"
    echo "$removed" | sed 's/^/  - /'
    echo ""
fi

# Compare common schemas
common=$(comm -12 <(echo "$old_schemas" | sort) <(echo "$new_schemas" | sort))
if [ -n "$common" ]; then
    echo "Modified schemas:"
    echo "$common" | while read -r schema_name; do
        old_def=$(echo "$old_schema" | jq ".schemas[\"$schema_name\"]")
        new_def=$(echo "$new_schema" | jq ".schemas[\"$schema_name\"]")

        if [ "$old_def" != "$new_def" ]; then
            echo "  ~ $schema_name"
        fi
    done
fi
```

## Testing Examples

### Example 8: Integration Test

```go
package main_test

import (
    "encoding/json"
    "os"
    "os/exec"
    "testing"
)

func TestCLIIntegration(t *testing.T) {
    // Create test schema
    schema := map[string]interface{}{
        "$schema":   "https://corpus-callosum.dev/schema/v1",
        "component": "test-integration",
        "version":   "1.0.0",
        "schemas": map[string]interface{}{
            "item": map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "id": map[string]interface{}{"type": "string"},
                },
                "required": []interface{}{"id"},
            },
        },
    }

    // Write to temp file
    tmpfile, err := os.CreateTemp("", "schema-*.json")
    if err != nil {
        t.Fatal(err)
    }
    defer os.Remove(tmpfile.Name())

    if err := json.NewEncoder(tmpfile).Encode(schema); err != nil {
        t.Fatal(err)
    }
    tmpfile.Close()

    // Register schema
    cmd := exec.Command("cc", "register",
        "--component", "test-integration",
        "--schema", tmpfile.Name(),
        "--workspace", "test-integration",
    )
    if err := cmd.Run(); err != nil {
        t.Fatalf("Failed to register: %v", err)
    }

    // Discover component
    cmd = exec.Command("cc", "discover",
        "--component", "test-integration",
        "--workspace", "test-integration",
    )
    output, err := cmd.Output()
    if err != nil {
        t.Fatalf("Failed to discover: %v", err)
    }

    var result map[string]interface{}
    if err := json.Unmarshal(output, &result); err != nil {
        t.Fatalf("Failed to parse output: %v", err)
    }

    if result["component"] != "test-integration" {
        t.Errorf("Expected component test-integration, got %v", result["component"])
    }

    // Cleanup
    exec.Command("cc", "unregister",
        "--component", "test-integration",
        "--confirm",
        "--workspace", "test-integration",
    ).Run()
}
```

These examples demonstrate the full range of Corpus Callosum capabilities for both CLI automation and LLM-driven interactions.
