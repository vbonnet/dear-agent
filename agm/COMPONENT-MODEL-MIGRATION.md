# AGM Component Model Migration

**Date**: 2026-02-19
**Bead**: oss-pfzm
**Phase**: 3 - Component Migration (AGM)
**Project**: modular-architecture-system

---

## Overview

AGM (Autonomous Agent Manager) has been refactored to use **workspace contracts** instead of direct Engram Go library imports. This enables AGM to function as a standalone component with graceful degradation when optional dependencies (Engram, Corpus Callosum) are not installed.

---

## Changes Made

### 1. Removed Direct Engram Dependency

**Before**:
```go
import "github.com/vbonnet/engram/core/pkg/retrieval"
```

**After**:
```go
// No direct engram imports - uses CLI contract instead
```

**Impact**:
- Removed `go.mod` dependency on `github.com/vbonnet/engram/core`
- Removed local `replace` directive pointing to `../../../engram/core`
- AGM can now be installed without engram repository present

### 2. Refactored Engram Client to Use CLI Contract

**File**: `internal/engram/client.go`

**Before** (library integration):
- Direct import of `retrieval.Service`
- Tight coupling to Engram Go types
- No fallback if Engram unavailable

**After** (CLI contract):
- Uses `exec.Command("engram", "search", "--json")` for retrieval
- Uses `exec.Command("workspace", "detect", "--json")` for workspace detection
- Graceful degradation: returns empty results if CLI unavailable

**Contract Interface**:
```bash
# Workspace detection (priority: flag > env > auto-detect > config > interactive > error)
workspace detect --format=json [--pwd=/path]

# Workspace listing
workspace list --format=json

# Engram retrieval (future implementation)
engram search --query="..." --tags="..." --limit=N --json
```

### 3. Added Corpus Callosum Schema Registration

**Files**:
- `schemas/corpus-callosum-schema.json` - AGM schema definition
- `scripts/register-corpus-callosum.sh` - Install hook
- `scripts/unregister-corpus-callosum.sh` - Uninstall hook

**Schema Entities**:
- `agm.session` - Session metadata (id, name, model, workspace, status)
- `agm.message` - Message data (role, content, tokens, timestamp)

**Integration**:
- `install-commands.sh` - Calls registration script on install
- `uninstall-commands.sh` - Calls unregistration script on uninstall
- Graceful degradation: install succeeds even if `cc` CLI unavailable

### 4. Added Workspace Contract Integration

**File**: `internal/discovery/workspace_contract.go`

**Functions**:
- `DetectWorkspaceUsingContract(pwd)` - Workspace detection via CLI
- `ListWorkspacesUsingContract()` - List all workspaces via CLI
- `IsWorkspaceContractAvailable()` - Check if workspace CLI present

**Fallback Strategy**:
- If workspace CLI available → use contract (recommended)
- If workspace CLI unavailable → fallback to legacy `FindSessionsAcrossWorkspaces()` (filesystem scanning)

### 5. Comprehensive Testing

**New Tests**:
- `internal/engram/standalone_test.go` - Standalone integration tests
- `internal/discovery/workspace_contract_test.go` - Contract integration tests

**Test Coverage**:
- ✅ Client creation without engram library
- ✅ Graceful degradation when CLI unavailable
- ✅ Config loading from environment
- ✅ No direct engram imports (compile-time verification)
- ✅ Workspace contract detection and listing

---

## Contract Specifications

### Workspace Contract v1

**Reference**: `specs/component-contracts.md`

**Interface**:
```go
type WorkspaceDetector interface {
    Detect(ctx context.Context, opts DetectOptions) (*Workspace, error)
    List(ctx context.Context) ([]Workspace, error)
    Validate(ctx context.Context, name string) error
}
```

**CLI Equivalent**:
```bash
workspace detect --format=json       # Detect current workspace
workspace list --format=json         # List all workspaces
workspace validate <name>            # Validate workspace exists
```

**JSON Output** (`workspace detect`):
```json
{
  "name": "oss",
  "root": "~/projects/myworkspace",
  "enabled": true,
  "output_dir": "./output",
  "detection_method": "auto_detect",
  "confidence": 1.0,
  "settings": {
    "log_level": "debug"
  }
}
```

### Corpus Callosum Contract v1

**Reference**: `specs/corpus-callosum-protocol.md`

**CLI Commands**:
```bash
cc register --schema=schema.json --component=agm --version=1.0.0
cc unregister --component=agm --version=1.0.0
cc discover --component=agm
cc query --component=agm --entity=session --filter='{"status":"active"}'
```

**AGM Schema**:
- Component: `agm`
- Version: `1.0.0`
- Entities: `session`, `message`
- Compatibility: `backward`

---

## Migration Benefits

### 1. **True Modularity**
- AGM works standalone without Engram installed
- Optional features degrade gracefully
- No tight Go library coupling

### 2. **Language Agnostic**
- CLI contracts work from any language (Go, TypeScript, Python)
- JSON output enables easy integration
- No language-specific bindings required

### 3. **Workspace Isolation**
- Proper workspace detection via standardized protocol
- Supports multi-workspace environments (OSS, Acme, etc.)
- No hard-coded paths or assumptions

### 4. **Cross-Component Discovery**
- Corpus Callosum enables component discovery
- Other tools can query AGM session data
- Extensible schema system for future evolution

### 5. **Backward Compatible**
- Existing AGM sessions continue to work
- Legacy workspace scanning available as fallback
- Graceful migration path

---

## Testing Validation

### Unit Tests
```bash
cd main/agm
go test ./internal/engram/... -v
go test ./internal/discovery/... -v
```

### Integration Tests
```bash
# Verify no direct engram imports
grep -r "github.com/vbonnet/engram" --include="*.go" .
# Expected: No matches

# Verify go.mod clean
grep "engram" go.mod
# Expected: No matches

# Test workspace contract (requires workspace CLI)
workspace detect --format=json
workspace list --format=json

# Test Corpus Callosum (requires cc CLI)
bash scripts/register-corpus-callosum.sh
cc discover --component=agm
bash scripts/unregister-corpus-callosum.sh
```

### Graceful Degradation Tests
```bash
# Test AGM works without workspace CLI
PATH=/tmp:$PATH go test ./internal/engram/... -v
# Expected: Tests pass with graceful degradation

# Test install without Corpus Callosum
PATH=/tmp:$PATH bash install-commands.sh
# Expected: Install succeeds with INFO message
```

---

## Future Enhancements

### Phase 4: Full Contract Migration
1. Implement `engram search --json` CLI for retrieval
2. Migrate AGM to use retrieval contract fully
3. Add MCP server interfaces for LLM access

### Phase 5: Dolt Storage
1. Migrate session storage from SQLite+JSONL to Dolt
2. Workspace-isolated databases (`~/.agm/oss.db`, `~/.agm/acme.db`)
3. Component-specific table prefixes (`agm_sessions`, `agm_messages`)

### Phase 6: Component Installer
1. Automated dependency detection
2. Schema registration on install/upgrade
3. Migration orchestration for breaking changes

---

## References

### Project Documentation
- **Roadmap**: `ROADMAP.md`
- **Contracts**: `specs/component-contracts.md`
- **Workspace Protocol**: `specs/workspace-protocol-cli.md`
- **Corpus Callosum**: `specs/corpus-callosum-protocol.md`

### Implementation
- **Workspace Library**: `pkg/workspace/`
- **AGM Engram Client**: `internal/engram/client.go`
- **Workspace Contract**: `internal/discovery/workspace_contract.go`
- **Schema Definition**: `schemas/corpus-callosum-schema.json`

---

**Status**: ✅ COMPLETE
**Next Phase**: Phase 3.3 - Wayfinder Component Migration
