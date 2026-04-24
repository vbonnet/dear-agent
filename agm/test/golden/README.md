# Golden Test Files

This directory contains golden (snapshot) test files for the Claude Session Manager project.

## Directory Structure

```
test/golden/
├── README.md                           # This file
├── .gitkeep                           # Ensures directory is tracked
├── manifest-*.json                    # Manifest generation snapshots
├── config-*.json                      # Configuration parsing snapshots
└── agent-interactions/                # Agent interaction golden datasets
    ├── .gitkeep
    ├── claude-*.json                  # Claude agent interactions
    ├── gemini-*.json                  # Gemini agent interactions
    ├── gpt-*.json                     # GPT agent interactions
    └── edge-case-*.json              # Edge cases and error scenarios
```

## Golden Test Types

### 1. Manifest Golden Tests (`manifest-*.json`)

Location: `internal/session/manifest_golden_test.go`

These files capture the expected JSON structure of session manifests for different scenarios:

- `manifest-new-session.json` - New session with all standard fields
- `manifest-resumed-session.json` - Previously created session being resumed
- `manifest-archived-session.json` - Archived/completed session
- `manifest-engram-session.json` - Session with Engram metadata
- `manifest-gemini-agent.json` - Session using Gemini agent
- `manifest-minimal-fields.json` - Minimal required fields only

**Purpose**: Detect breaking changes in manifest structure and ensure consistent JSON serialization.

### 2. Configuration Parsing Golden Tests (`config-*.json`)

Location: `internal/config/parser_golden_test.go`

These files capture the expected parsed configuration structures:

- `config-default.json` - Default configuration
- `config-minimal-yaml.json` - Minimal YAML configuration
- `config-full-yaml.json` - Complete YAML with all options
- `config-timeout-disabled.json` - Timeout disabled configuration
- `config-lock-disabled.json` - Lock disabled configuration
- `config-healthcheck-customized.json` - Custom health check settings
- `config-workspace.json` - Workspace configuration
- `config-structure.json` - Full structure for breaking change detection
- `config-yaml-roundtrip.json` - YAML round-trip verification

**Purpose**: Detect breaking vs non-breaking configuration format changes.

### 3. Agent Interaction Golden Dataset (`agent-interactions/`)

Location: `test/golden/agent-interactions/`

These files document known-good request/response pairs for different AI agents:

#### Claude Agent
- `claude-create-session-success.json` - Successful session creation
- `claude-send-message-success.json` - Successful message send
- `claude-get-history-success.json` - Retrieve conversation history
- `claude-session-not-found-error.json` - Session not found error

#### Gemini Agent
- `gemini-create-session-success.json` - Successful session creation
- `gemini-send-message-success.json` - Successful message with API call
- `gemini-api-error.json` - API quota exceeded error

#### GPT Agent
- `gpt-create-session-success.json` - Successful session creation
- `gpt-send-message-with-tools.json` - Message with tool/function call
- `gpt-rate-limit-error.json` - Rate limit error

#### Edge Cases
- `edge-case-empty-message.json` - Empty message validation
- `edge-case-invalid-session-id.json` - Invalid session ID format
- `edge-case-context-window-exceeded.json` - Context window limit exceeded

**Purpose**:
1. Document expected behavior for each agent operation
2. Regression testing for API integration
3. Error handling verification
4. Contract testing between AGM and AI providers

## File Format

All golden files use JSON format with consistent structure:

### Agent Interaction Format
```json
{
  "description": "Human-readable description of the interaction",
  "agent": "claude|gemini|gpt",
  "operation": "CreateSession|SendMessage|GetHistory|etc",
  "request": {
    // Request parameters
  },
  "response": {
    "success": true|false,
    "error": null | {
      "code": "ERROR_CODE",
      "message": "Error message",
      "details": {}
    }
    // Additional response data
  },
  "metadata": {
    "timestamp": "ISO 8601 timestamp",
    // Additional metadata
  }
}
```

## Updating Golden Files

### Intentional Changes (New Features/Fixes)

When you intentionally change code that affects golden files:

```bash
# Update manifest golden files
go test ./internal/session -run TestManifestGeneration -update

# Update config golden files
go test ./internal/config -run TestConfigParsing -update
go test ./internal/config -run TestConfigStructure -update

# Verify tests pass
go test ./internal/session -run TestManifestGeneration
go test ./internal/config -run "TestConfig"
```

### Unintentional Changes (Breaking Changes Detected)

If golden tests fail unexpectedly, you've introduced a breaking change:

1. Review the diff to understand what changed
2. Determine if the change is:
   - **Breaking**: Requires migration or version bump
   - **Non-breaking**: Safe to update golden files
3. Update golden files if non-breaking, or fix the code if breaking

## Testing Strategy

### Regression Detection

Golden tests detect:
- Manifest structure changes
- Configuration parsing changes
- Agent interaction contract changes
- JSON serialization format changes

### Test Execution

```bash
# Run all golden tests
go test ./internal/session -run Golden
go test ./internal/config -run TestConfig

# Run specific test
go test ./internal/session -run TestManifestGeneration_NewSession -v
```

## Maintenance

- **Add new golden files** when adding new agent operations or edge cases
- **Update golden files** when intentionally changing output formats
- **Review diffs carefully** when golden tests fail - they indicate breaking changes
- **Document changes** in commit messages when updating golden files

## Related Documentation

- Agent Interface: `internal/agent/interface.go`
- Manifest Schema: `internal/manifest/manifest.go`
- Config Schema: `internal/config/config.go`

## Testing Coverage

Current coverage:
- ✅ Manifest generation (6 test cases)
- ✅ Configuration parsing (9 test cases)
- ✅ Agent interactions (13 golden datasets)
  - Claude: 4 interactions
  - Gemini: 3 interactions
  - GPT: 3 interactions
  - Edge cases: 3 scenarios

Total: 28 golden test artifacts
