# Health Monitoring & Diagnostics

Comprehensive health monitoring system for MCP Wizard with fast health checks and detailed diagnostics.

**Part of Phase 4-v2 (oss-n1nq.4-v2)**

## Overview

MCP Wizard includes two commands for monitoring system health:

1. **`health`** - Fast health check (<5 seconds) with overall system status
2. **`doctor`** - Comprehensive diagnostics with actionable recommendations

Both commands support caching (5-minute TTL), JSON output, and force refresh.

## Health Command

### Quick Start

```bash
# Basic health check
mcp-wizard health

# JSON output for scripting
mcp-wizard health --json

# Force fresh check (bypass cache)
mcp-wizard health --force

# Silent mode (exit code only)
mcp-wizard health --silent
```

### Exit Codes

- `0` - All checks healthy
- `1` - One or more warnings (degraded)
- `2` - One or more errors (unhealthy)

### Example Output

```
Overall Status: HEALTHY

Component Health:
  ✓ Token: healthy (expires in 45 minutes)
  ✓ MCP: healthy (2/2 processes alive)
  ✓ Network: healthy (All 3 endpoints reachable)
  ✓ Intent Analyzer: healthy (Confidence 87%, 0 mismatches)
```

### What Gets Checked

1. **Token Health** - Google OAuth token validity and TTL
   - Healthy: Valid token, >5min TTL
   - Degraded: Valid token, 1-5min TTL
   - Unhealthy: Expired or missing token

2. **MCP Processes** - Process existence check via `pgrep`
   - Healthy: All configured MCPs running
   - Degraded: Some MCPs running
   - Unhealthy: No MCPs running

3. **Network Connectivity** - HEAD requests to OAuth/API endpoints
   - Healthy: All endpoints reachable
   - Degraded: 1-2 endpoints unreachable
   - Unhealthy: 3+ endpoints unreachable

4. **Intent Analyzer** - Keyword matching accuracy test
   - Healthy: ≥70% avg confidence, 0 mismatches
   - Degraded: ≥50% avg confidence, ≤1 mismatch
   - Unhealthy: <50% avg confidence or 2+ mismatches

## Doctor Command

### Quick Start

```bash
# Full diagnostics
mcp-wizard doctor

# JSON output
mcp-wizard doctor --json

# Force fresh diagnostics
mcp-wizard doctor --force
```

### Exit Codes

- `0` - All checks healthy
- `1` - One or more warnings
- `2` - One or more errors

### Example Output

```
System Diagnostics Report
========================

Findings:
  ✓ [OK] Token Health: Google token valid (expires in 45 minutes)
  ✓ [OK] MCP Processes: 2/2 processes alive
  ✓ [OK] Network Connectivity: All 3 endpoints reachable
  ⚠ [WARNING] Intent Analyzer: Confidence 62%, 1 mismatch
  ✓ [OK] Configuration: Config file valid

Recommendations:
  1. Intent analyzer accuracy is below optimal. Consider updating to latest version.

Overall Status: WARNING
```

### What Gets Diagnosed

All health checks plus:

5. **Configuration Validation** - Config file schema and completeness
   - Checks required fields (company.name, company.okta_domain, broker.downstream_mcps)
   - Validates MCP server configurations (command, args)
   - Reports missing or invalid settings

### Recommendations Engine

Doctor provides actionable recommendations based on findings:

| Issue | Recommendations |
|-------|----------------|
| Token expired | Re-authenticate: `mcp-wizard setup --mcps=googledocs` |
| Token expires soon | Re-authenticate proactively to avoid disruption |
| MCP processes down | Restart with AI agent, check config in `~/.config/mcp-wizard/config.json` |
| Some MCPs down | Check which failed and restart them |
| Network unreachable | Check internet connection, firewall/VPN settings |
| Network degraded | Check proxy settings if behind corporate proxy |
| Intent analyzer failing | Reinstall: `npm install -g mcp-wizard` |
| Intent analyzer degraded | Update to latest version |
| Config missing/invalid | Run `mcp-wizard setup` to recreate |
| Config incomplete | Run `mcp-wizard config init` to add missing settings |

## Caching

Both commands use a 5-minute in-memory cache to avoid excessive checking:

- First run: Performs all checks (~2-5 seconds)
- Subsequent runs (within 5min): Returns cached results (<100ms)
- Force bypass: Use `--force` flag to ignore cache

### When Cache Is Cleared

- Manual: `--force` flag
- Automatic: 5 minutes after last check
- Process restart: Cache is in-memory only

## Integration with AI Agents

### Session Start Hook

Health checks run automatically when starting MCP Wizard session:

```typescript
import { onSessionStart } from 'mcp-wizard/hooks/session-start';

// Called at session initialization
await onSessionStart();
```

If health status is not healthy, a warning is logged.

### Programmatic Usage

```typescript
import { checkHealth, formatHealthOutput } from 'mcp-wizard/commands/health';
import { runDiagnostics, formatDiagnosticReport } from 'mcp-wizard/commands/doctor';

// Run health check
const healthStatus = await checkHealth({ force: false });
console.log(formatHealthOutput(healthStatus));

// Run diagnostics
const diagnostics = await runDiagnostics({ force: false });
console.log(formatDiagnosticReport(diagnostics));
```

## JSON Output Format

### Health Command JSON

```json
{
  "overall_status": "healthy",
  "checks": {
    "token": {
      "status": "healthy",
      "details": "Google token valid (expires in 45 minutes)"
    },
    "mcp": {
      "status": "healthy",
      "details": "2/2 processes alive"
    },
    "network": {
      "status": "healthy",
      "details": "All 3 endpoints reachable"
    },
    "intentAnalyzer": {
      "status": "healthy",
      "details": "Confidence 87%, 0 mismatches"
    }
  },
  "warnings": [],
  "errors": [],
  "timestamp": "2025-01-17T12:34:56.789Z"
}
```

### Doctor Command JSON

```json
{
  "overall_status": "healthy",
  "findings": [
    {
      "component": "Token Health",
      "status": "ok",
      "message": "Google token valid (expires in 45 minutes)",
      "details": "{\"expiresAt\":\"2025-01-17T13:19:56.789Z\",\"remainingTTL\":2700000,\"ttlMinutes\":45}"
    },
    {
      "component": "MCP Processes",
      "status": "ok",
      "message": "2/2 processes alive",
      "details": "{\"processes\":[{\"name\":\"googledocs\",\"alive\":true,\"pid\":12345}]}"
    }
  ],
  "recommendations": [],
  "timestamp": "2025-01-17T12:34:56.789Z"
}
```

## Troubleshooting

### Health check hangs or times out

- Network check uses 3s timeout per endpoint
- If hanging, check firewall/VPN blocking network requests
- Use `--force` to bypass potentially corrupted cache

### "Token not found" error

Run setup to authenticate:
```bash
mcp-wizard setup --mcps=googledocs
```

### "No MCP processes running"

1. Check if MCPs are configured: `mcp-wizard status`
2. Start your AI agent (Claude Code, Cursor, etc.) which launches MCPs
3. Verify config: `cat ~/.config/mcp-wizard/config.json`

### "Config file error"

Run setup wizard to recreate:
```bash
mcp-wizard setup
```

Or manually create with:
```bash
mcp-wizard config init
```

## Architecture

### File Structure

```
src/
  commands/
    health.ts          # Health command implementation (220 lines)
    doctor.ts          # Doctor command implementation (300 lines)
  lib/
    health-cache.ts    # 5-minute cache (88 lines)
    health-checks.ts   # Core health checks (509 lines)
  hooks/
    session-start.ts   # Session initialization hook (44 lines)
```

### Design Decisions

**V1 Limitations (intentional):**
- Token Health: Only checks Google OAuth (Atlassian uses mcp-remote auth)
- MCP Processes: Existence check only (no stdio ping due to process ownership)
- Network: HEAD requests only (no deep API validation)
- Intent Analyzer: Keyword matching only (no ML/AI validation)

**Why caching?**
- Avoid excessive health checks during active development sessions
- Reduce latency for repeated checks (100ms vs 2-5s)
- TTL of 5min balances freshness with performance

**Why separate health vs doctor?**
- `health`: Fast check for quick feedback loops
- `doctor`: Comprehensive diagnostics when issues arise
- Different use cases, different verbosity levels

## Testing

### Coverage

- `health-cache.ts`: 100% coverage
- `health-checks.ts`: 94% coverage
- Overall: 90%+ coverage target met

### Test Files

```
tests/
  unit/
    lib/
      health-cache.test.ts      # Cache TTL, bypass, expiration (13 tests)
      health-checks.test.ts     # All 4 health checks + config (24 tests)
    commands/
      health.test.ts            # Command integration (16 tests)
      doctor.test.ts            # Diagnostics & recommendations (8 tests)
  integration/
    health-diagnostics.test.ts  # End-to-end workflows (10 tests)
  e2e/
    health/
      health-workflow.test.ts   # User-facing scenarios (6 tests)
```

### Running Tests

```bash
# All health tests
npm test -- --testPathPattern=health

# With coverage
npm test -- --testPathPattern=health --coverage

# Specific file
npm test tests/unit/lib/health-cache.test.ts
```

## Future Enhancements (Post-V1)

Potential improvements for future phases:

1. **Deeper MCP validation** - Stdio ping to verify MCP responsiveness
2. **Token refresh** - Auto-refresh tokens before expiration
3. **Atlassian token health** - Check Atlassian OAuth in addition to Google
4. **Historical tracking** - Store health check history for trending
5. **Alert thresholds** - Configurable warning/error thresholds
6. **Auto-remediation** - Attempt fixes (e.g., token refresh, MCP restart)
7. **Metrics export** - Prometheus/Grafana integration
8. **Web dashboard** - Real-time health monitoring UI

## See Also

- [TROUBLESHOOTING.md](../TROUBLESHOOTING.md) - General troubleshooting guide
- [README.md](../README.md) - MCP Wizard overview
- [ATLASSIAN-MCP.md](ATLASSIAN-MCP.md) - Atlassian OAuth setup
