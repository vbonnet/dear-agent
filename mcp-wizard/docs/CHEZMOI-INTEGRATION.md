# Chezmoi Integration

This document explains how mcp-wizard integrates with chezmoi for automated dotfile management.

## Overview

The mcp-wizard setup command detects if your MCP configuration is managed by chezmoi and offers to automate the template creation and application process. This eliminates manual copy-paste steps and reduces setup friction.

## Architecture

### Modules

**`src/lib/chezmoi-manager.ts`**: Core module for chezmoi automation
- `detectChezmoi()`: Detects chezmoi installation and source directory
- `writeChezmoiTemplate()`: Creates template file with MCP config
- `showChezmoiDiff()`: Previews changes before applying
- `applyChezmoiConfig()`: Applies specific file via chezmoi
- `automateChezmoiSetup()`: High-level orchestrator

**`src/commands/setup.ts`**: Integration point
- Detects chezmoi during environment check
- Prompts user: "Apply via chezmoi? (Y/n)"
- Optionally shows diff preview
- Falls back to manual instructions on errors

### User Flow

```
setup.ts detects chezmoi
  ├─ Not detected → writeMcpConfig() (direct file write)
  └─ Detected → Prompt user

User chooses automated apply
  ├─ Show diff? (y/N)
  │   └─ Yes → Run chezmoi diff
  │
  ├─ writeChezmoiTemplate()
  │   └─ Creates: ~/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl
  │
  ├─ applyChezmoiConfig()
  │   └─ Runs: chezmoi apply ~/.config/claude-code/mcp.json
  │
  ├─ Success → "✓ Applied MCP config via chezmoi"
  └─ Failure → Fall back to showChezmoiSnippet()

User declines automation
  └─ showChezmoiSnippet() (manual instructions)
```

## Function Reference

### detectChezmoi()

Detects if chezmoi is installed and initialized.

**Algorithm**:
1. Check if `chezmoi` command exists (`which chezmoi`)
2. Get source directory (`chezmoi source-path`)
3. Verify directory exists on filesystem

**Returns**: `ChezmoiDetection`
```typescript
{
  detected: true,
  sourcePath: '/home/user/.local/share/chezmoi'
}
```

Or on failure:
```typescript
{
  detected: false,
  reason: 'not installed' | 'not initialized' | 'permission denied'
}
```

### writeChezmoiTemplate()

Writes MCP config to chezmoi template file.

**Template syntax**:
```
{{- if hasSuffix "-w" .chezmoi.hostname }}
{
  "mcpServers": { ... }
}
{{- else }}
{ "mcpServers": {} }
{{- end }}
```

This ensures:
- Work machines (hostname ending in `-w`): Get full MCP config
- Other machines: Get empty config

**File location**: `<source>/dot_config/claude-code/private_mcp.json.tmpl`

### applyChezmoiConfig()

Applies specific file via chezmoi.

**Uses targeted apply**: `chezmoi apply <file>` (not blanket `chezmoi apply`)

**Why targeted?**
- Safety: Only applies MCP config, not all pending changes
- User control: Doesn't surprise users by applying unrelated changes
- Minimal blast radius: If something fails, only MCP config affected

### automateChezmoiSetup()

High-level orchestrator that:
1. Detects chezmoi
2. Writes template
3. Shows diff (optional)
4. Applies config
5. Falls back to manual on any error

**Error handling**: All errors trigger fallback to `showChezmoiSnippet()`. No crashes, always provides user path forward.

## Error Handling

### Error Types

| Error | Detection | User Impact | Mitigation |
|-------|-----------|-------------|------------|
| Chezmoi not installed | `which chezmoi` fails (127) | Fall back to manual | Show manual instructions |
| Not initialized | `chezmoi source-path` fails | Fall back to manual | Show manual instructions |
| Permission denied | EACCES on write/apply | Fall back to manual | Show error + manual path |
| Apply failure | `chezmoi apply` fails | Fall back to manual | Show stderr + manual path |

### Error Messages

**Format**:
```
✗ Chezmoi apply failed: <specific error>
  Falling back to manual instructions...

ℹ️ Manual Steps:
  1. Add this to: ~/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl
  2. Run: chezmoi apply
```

## Testing

### Unit Tests

**File**: `tests/unit/lib/chezmoi-manager.test.ts`

**Coverage**: 97.5% (23 tests)

**Test categories**:
- Detection (4 tests): installed, not installed, not initialized, permission denied
- Path generation (6 tests): all agents, invalid agent, path traversal
- Template writing (2 tests): correct syntax, directory creation
- Diff preview (4 tests): changes exist, no changes, error cases
- Apply execution (3 tests): success, failure, output capture
- End-to-end (4 tests): full flow, detection failure, apply failure, diff option

**Mocking strategy**:
- Mock `child_process.exec` for CLI commands
- Mock `fs.promises` for file operations
- Use Jest's mock implementation callbacks

### Integration Tests

**File**: `tests/integration/setup-chezmoi.test.ts` (to be created)

**Scenarios**:
1. Full automated flow (chezmoi detected, user accepts)
2. User declines automation (shows manual instructions)
3. Chezmoi not detected (falls back to writeMcpConfig)
4. Apply failure (falls back to manual)

### Manual Testing

**Prerequisites**:
- System with chezmoi installed and initialized
- System without chezmoi (for fallback testing)

**Test cases**:
1. Run wizard with automated apply enabled
2. Verify template file created correctly
3. Verify MCP config applied to target file
4. Test diff preview
5. Test error scenarios (permission denied, etc.)

## Troubleshooting

### Chezmoi apply fails with "permission denied"

**Cause**: User doesn't have write permission to target file.

**Solution**:
1. Check file permissions: `ls -la ~/.config/claude-code/mcp.json`
2. Fix permissions: `chmod 644 ~/.config/claude-code/mcp.json`
3. Re-run wizard or apply manually: `chezmoi apply`

### Template file not found

**Cause**: Chezmoi source directory doesn't exist or is in non-standard location.

**Solution**:
1. Check source path: `chezmoi source-path`
2. Initialize if needed: `chezmoi init`
3. Verify directory exists: `ls -la $(chezmoi source-path)`

### Diff shows unexpected changes

**Cause**: Template file differs from what wizard generated.

**Solution**:
1. Check template: `cat $(chezmoi source-path)/dot_config/claude-code/private_mcp.json.tmpl`
2. If incorrect, manually edit or re-run wizard
3. Apply: `chezmoi apply`

### Wizard doesn't detect chezmoi

**Cause**: Chezmoi not in PATH or not initialized.

**Solution**:
1. Check installation: `which chezmoi`
2. Install if missing: `brew install chezmoi` (macOS) or `apt install chezmoi` (Linux)
3. Initialize: `chezmoi init`
4. Re-run wizard

## Development

### Adding Support for New Agents

To add support for a new AI agent:

1. **Update `getTemplateFilePath()`**:
   ```typescript
   const agentConfigMap: Record<string, string> = {
     'claude-code': '.config/claude-code',
     'new-agent': '.new-agent',  // Add here
   };
   ```

2. **Update `getTargetFilePath()`**:
   ```typescript
   const agentConfigMap: Record<string, string> = {
     'claude-code': '.config/claude-code/mcp.json',
     'new-agent': '.new-agent/mcp.json',  // Add here
   };
   ```

3. **Add tests** in `chezmoi-manager.test.ts`:
   ```typescript
   test('generates correct path for new-agent', () => {
     const path = getTemplateFilePath(sourcePath, 'new-agent');
     expect(path).toBe('...expected path...');
   });
   ```

### Code Style

- TypeScript strict mode enabled
- JSDoc comments on all public functions
- Error handling: Catch exceptions, return structured results
- Security: Validate all paths, quote file paths in commands

## Security Considerations

### Path Traversal Prevention

All template paths are validated via `validatePath()`:
- Rejects paths containing `..`
- Rejects paths outside home directory
- Canonicalizes and normalizes paths

### Command Injection Prevention

File paths in commands are always double-quoted:
```typescript
await execAsync(`chezmoi apply "${targetFile}"`);
```

User input is never directly interpolated into shell commands.

## References

- Chezmoi documentation: https://www.chezmoi.io/
- Chezmoi template syntax: https://www.chezmoi.io/reference/templates/
- Chezmoi apply command: https://www.chezmoi.io/reference/commands/apply/
