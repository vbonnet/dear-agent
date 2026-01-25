# SessionStart Hook - MCP Health Check at Shell Startup

## Overview

The `session-start` hook provides proactive MCP authentication status feedback when you start a new terminal session. It checks your Okta token health and notifies you if tokens need refreshing, helping you avoid authentication failures mid-workflow.

**Purpose**: Proactive auth status feedback at shell startup
**Performance**: <500ms for cached checks
**Exit codes**: 0 (healthy/warning), 1 (unhealthy)

## Quick Start

### Check MCP Health

```bash
$ mcp-wizard session-start
✓ MCP Health: 4 MCPs authenticated
```

### Shell Integration

Add to `~/.bashrc` or `~/.zshrc`:

```bash
# Check MCP health at session startup
mcp-wizard session-start
```

**Silent mode** (suppress warnings):

```bash
# Only show errors, suppress warnings
mcp-wizard session-start 2>/dev/null || true
```

## Command Reference

### Usage

```bash
mcp-wizard session-start [options]
```

### Options

- `--verbose` / `-v`: Show detailed health check results
- `--auto-refresh` / `-a`: Automatically refresh expired tokens
- `--help` / `-h`: Show help text

### Examples

**Default mode** (minimal output):

```bash
$ mcp-wizard session-start
✓ MCP Health: 4 MCPs authenticated
```

**Verbose mode** (detailed status):

```bash
$ mcp-wizard session-start --verbose

MCP Health Status:
Token: ✓ authenticated (expires in 2h 15m)
MCPs configured: GoogleDocs, Atlassian, Slack, Glean
Expiration: 2h 15m
```

**Auto-refresh mode** (automatic token refresh):

```bash
$ mcp-wizard session-start --auto-refresh
⠋ Checking MCP health...
⠋ Refreshing tokens...
✓ Token refreshed successfully
✓ MCP Health: 4 MCPs authenticated
```

## Output Formats

### Healthy Status (Green)

All tokens valid and not expiring soon:

```
✓ MCP Health: 4 MCPs authenticated
```

**Exit code**: 0

### Degraded Status (Yellow)

Token expiring soon (1-5 minutes remaining):

```
⚠ MCP Health: Token expiring soon (3m)
Run `mcp-wizard auth` to refresh
```

**Exit code**: 0 (warning, not critical)

### Unhealthy Status (Red)

Token expired or invalid:

```
✗ MCP Health: Token expired or invalid
Run `mcp-wizard auth` to re-authenticate
```

**Exit code**: 1 (requires action)

### No MCPs Configured (Yellow)

No MCP configuration found:

```
⚠️  No MCP configuration found. Run `mcp-wizard setup` to configure MCPs.
```

**Exit code**: 0 (not an error, graceful fallback)

## Shell Integration

### Bash (.bashrc)

Add to `~/.bashrc`:

```bash
# MCP health check at session startup
if command -v mcp-wizard &> /dev/null; then
  mcp-wizard session-start
fi
```

**Conditional integration** (only if mcp-wizard installed):

```bash
# Check MCP health if mcp-wizard is installed
if command -v mcp-wizard &> /dev/null; then
  mcp-wizard session-start 2>/dev/null || true
fi
```

### Zsh (.zshrc)

Add to `~/.zshrc`:

```bash
# MCP health check at session startup
if (( $+commands[mcp-wizard] )); then
  mcp-wizard session-start
fi
```

### Silent Mode (Suppress Warnings)

If you want to see errors only (no warnings):

```bash
# Show only critical errors, suppress warnings
mcp-wizard session-start 2>/dev/null || true
```

## How It Works

### Architecture

```
session-start
  ↓
1. Discover MCPs (from ~/.claude/.mcp.json and ~/.config/mcp-wizard/config.json)
  ↓
2. Run Health Checks (parallel execution, uses cache)
  ↓
3. Extract Token Health (covers all Okta-authenticated MCPs)
  ↓
4. Format Output (color-coded, actionable)
  ↓
5. Auto-Refresh (if --auto-refresh and token expired)
```

### MCP Discovery

The hook reads MCP configurations from two sources (in priority order):

1. **Claude Code config**: `~/.claude/.mcp.json`
2. **mcp-wizard config**: `~/.config/mcp-wizard/config.json`

**Filtering**: Only Okta-authenticated MCPs are checked (GoogleDocs, Atlassian, Slack, Glean).

### Health Check Integration

Uses the `health-checks` module from Phase 4-v2:

- **Token Health**: Validates Okta token (covers all Okta MCPs with single token)
- **Caching**: Uses 5-minute cache for performance (<500ms cached checks)
- **Parallel execution**: All checks run concurrently via `Promise.all()`

### Token Coverage

**Important**: All Okta-authenticated MCPs (GoogleDocs, Atlassian, Slack, Glean) share the same Google OAuth token. A single token health check covers all MCPs.

## Auto-Refresh

### Usage

```bash
mcp-wizard session-start --auto-refresh
```

### Behavior

1. Checks token health
2. If token expired or invalid:
   - Automatically calls `authenticate()` from auth.ts
   - Uses **Device Flow** for headless environments (SSH, Cloud Shell)
   - Uses **PKCE Flow** for interactive environments (local terminal)
3. Re-checks health after refresh
4. Shows success or error

### Device Flow (Headless)

In headless environments (SSH, tmux, Cloud Shell), auto-refresh uses Device Flow:

```bash
$ mcp-wizard session-start --auto-refresh
⠋ Refreshing tokens...
Headless environment detected, using device flow...

Please visit: https://okta.com/activate
And enter code: ABCD-1234

✓ Token refreshed successfully
✓ MCP Health: 4 MCPs authenticated
```

### Error Handling

If auto-refresh fails:

```bash
✗ Token refresh failed
Run `mcp-wizard auth` to re-authenticate manually
Error: Network timeout (check connectivity)
```

**Fallback**: Manual authentication via `mcp-wizard auth`

## Performance

### Execution Time

**Cached checks** (default):

- Config discovery: ~10ms
- Health checks (cached): ~50ms
- Output formatting: ~5ms
- **Total**: ~75ms (well under 500ms target)

**Fresh checks** (force or first run):

- Config discovery: ~10ms
- Health checks (fresh): ~1500ms (Okta API call)
- Output formatting: ~5ms
- **Total**: ~1.5s

### Cache Behavior

- **Cache TTL**: 5 minutes (300 seconds)
- **Cache key**: `mcp-health-Token Health`
- **Cache invalidation**: Automatic after TTL expires
- **Force fresh check**: Use `--force` flag (not available in session-start, use `mcp-wizard health --force`)

### Shell Startup Impact

**Recommendation**: <500ms is acceptable for shell startup

- If startup feels slow, use silent mode: `mcp-wizard session-start 2>/dev/null || true`
- Cached checks typically complete in <100ms (minimal impact)

## Security

### Token Safety

- **Never logs tokens**: Only shows status (healthy/degraded/unhealthy) and TTL
- **No token leakage**: Token values never appear in output
- **Secure storage**: Tokens stored in OS keychain (via existing auth.ts)

### Config File Permissions

**Recommendation**: `chmod 600` for config files

```bash
chmod 600 ~/.claude/.mcp.json
chmod 600 ~/.config/mcp-wizard/config.json
```

**Why**: Config files may contain client IDs and Okta domain info (though not sensitive like tokens, still good practice)

## Troubleshooting

### "No MCP configuration found"

**Symptom**:

```
⚠️  No MCP configuration found. Run `mcp-wizard setup` to configure MCPs.
```

**Solution**: Run `mcp-wizard setup` to configure MCPs and create config files.

### "Token expired or invalid"

**Symptom**:

```
✗ MCP Health: Token expired or invalid
Run `mcp-wizard auth` to re-authenticate
```

**Solution**:

1. **Manual auth**: `mcp-wizard auth`
2. **Auto-refresh**: `mcp-wizard session-start --auto-refresh`

### Health check fails with network error

**Symptom**:

```
⚠ MCP Health: Network error (check connection)
```

**Cause**: Okta API unreachable (network timeout, VPN issues)

**Solution**:

1. Check internet connectivity
2. Verify VPN connection (if required)
3. Retry: `mcp-wizard session-start --force` (bypasses cache)

### Slow shell startup

**Symptom**: Terminal takes >1 second to load

**Cause**: Fresh health checks (no cache) or network latency

**Solution**:

1. **Use silent mode**: `mcp-wizard session-start 2>/dev/null || true`
2. **Check cache**: Cached checks should be <100ms
3. **Investigate network**: Use `mcp-wizard health --verbose` to diagnose

### Auto-refresh fails in headless environment

**Symptom**:

```
✗ Token refresh failed
Error: Device Flow timeout
```

**Cause**: Device Flow requires user interaction (visit URL, enter code)

**Solution**:

1. **Interactive auth**: Exit headless env, run `mcp-wizard auth` locally
2. **Retry Device Flow**: Ensure you complete the flow within timeout (usually 5 minutes)

## CI/CD Usage

### Non-Interactive Environments

In CI/CD pipelines or automated scripts, use `--auto-refresh` with caution:

**Recommended**:

```bash
# Check health without interactive prompts
mcp-wizard session-start 2>/dev/null || echo "MCP auth required"
```

**Not recommended**:

```bash
# Auto-refresh may hang waiting for Device Flow input
mcp-wizard session-start --auto-refresh
```

### Exit Code Handling

Use exit codes to detect auth issues:

```bash
if ! mcp-wizard session-start; then
  echo "MCP authentication failed - manual intervention required"
  exit 1
fi
```

## Advanced Topics

### Multiple Config Sources

If both `~/.claude/.mcp.json` and `~/.config/mcp-wizard/config.json` exist:

- **Merge behavior**: Both files are read
- **Priority**: `~/.claude/.mcp.json` takes precedence
- **Deduplication**: MCPs with same name are merged (first source wins)

### Okta vs Non-Okta MCPs

**V1 behavior**: Only checks Okta-authenticated MCPs

- **Included**: GoogleDocs, Atlassian, Slack, Glean (auth.type === 'okta')
- **Excluded**: GitHub, custom MCPs with OAuth or API key auth

**Future (V2)**: May expand to check non-Okta auth types

### Cache Tuning

Cache TTL is currently hardcoded to 5 minutes. For advanced users:

**To clear cache**:

```bash
# No built-in clear command in V1 - wait for TTL expiry
# OR force fresh check via health command:
mcp-wizard health --force
```

## Related Commands

- `mcp-wizard setup` - Initial MCP configuration
- `mcp-wizard auth` - Manual token refresh
- `mcp-wizard health` - Detailed health diagnostics
- `mcp-wizard doctor` - Comprehensive system check

## Support

- **Documentation**: See `README.md` for full CLI reference
- **Troubleshooting**: See `TROUBLESHOOTING.md`
- **Issues**: Report bugs on GitHub or #vida-dev Slack

## Changelog

### v0.1.0 (2026-01-17)

- Initial release of session-start hook
- Support for `--verbose` and `--auto-refresh` flags
- Integration with health-checks module (oss-n1nq.4-v2)
- Dynamic Okta MCP discovery from config files
- <500ms cached performance target
- Part of Phase 4-v2 SessionStart Hook Integration (oss-n1nq.5-v2)
