# Global MCP Support

This document describes the global MCP support feature in mcp-wizard, which enables session-wide MCP availability through HTTP/SSE transport.

## Overview

Global MCP support allows MCP servers to be shared across multiple sessions using HTTP/SSE transport instead of stdio. This enables:

- **Centralized MCP Management**: Run MCPs as persistent services
- **Multi-Session Access**: Share MCP instances across Claude Code sessions
- **HTTP Transport**: MCPs accessible via HTTP/SSE instead of stdio
- **Temporal Workflows**: Integration with Temporal for workflow orchestration
- **Health Monitoring**: Built-in health checks for global MCP services

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ mcp-wizard                                                  │
│ ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐ │
│ │ Config      │  │ Health Check │  │ CLI Commands       │ │
│ │ Management  │  │ System       │  │ - enable-global-   │ │
│ │             │  │              │  │ - disable-global-  │ │
│ │             │  │              │  │ - status           │ │
│ └─────────────┘  └──────────────┘  └────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                         │
                         │ HTTP/SSE
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ Global MCP Server (http://localhost:8001)                  │
│ ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐ │
│ │ HTTP Server │  │ Discovery    │  │ Session Manager    │ │
│ │ Health      │  │ Endpoint     │  │                    │ │
│ │ /health     │  │ /discovery   │  │                    │ │
│ └─────────────┘  └──────────────┘  └────────────────────┘ │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐│
│ │ Temporal Integration                                    ││
│ │ - MCPServiceWorkflow                                    ││
│ │ - Workflow UI: http://localhost:8088                    ││
│ └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Configuration

Global MCP configuration is stored in `~/.config/mcp-wizard/config.json`:

```json
{
  "company": {
    "name": "Your Company",
    "glean_instance": "yourcompany",
    "okta_domain": "yourcompany.okta.com"
  },
  "globalMcps": {
    "enabled": true,
    "healthCheckUrl": "http://localhost:8001/health",
    "discoveryUrl": "http://localhost:8001/discovery",
    "temporalUrl": "http://localhost:7233"
  }
}
```

### Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable global MCP discovery |
| `healthCheckUrl` | string | `http://localhost:8001/health` | Health check endpoint |
| `discoveryUrl` | string | `http://localhost:8001/discovery` | Service discovery endpoint |
| `temporalUrl` | string | `http://localhost:7233` | Temporal server URL |

## CLI Commands

### Enable Global MCPs

```bash
mcp-wizard enable-global-mcps [options]
```

Enables global MCP discovery and HTTP transport.

**Options:**
- `--health-url <url>` - Health check URL (default: http://localhost:8001/health)
- `--discovery-url <url>` - Discovery URL (default: http://localhost:8001/discovery)
- `--temporal-url <url>` - Temporal server URL (default: http://localhost:7233)

**Examples:**

```bash
# Enable with default settings
mcp-wizard enable-global-mcps

# Enable with custom URLs
mcp-wizard enable-global-mcps \
  --health-url http://localhost:9000/health \
  --temporal-url http://localhost:9001
```

**Output:**

```
Enabling global MCP discovery...

✓ Global MCP discovery enabled

Configuration:
  Health check URL: http://localhost:8001/health
  Discovery URL:    http://localhost:8001/discovery
  Temporal URL:     http://localhost:7233
  Temporal UI:      http://localhost:8088

Config saved to:    ~/.config/mcp-wizard/config.json

Run 'mcp-wizard status' to verify global MCPs.
```

### Disable Global MCPs

```bash
mcp-wizard disable-global-mcps [options]
```

Disables global MCP discovery and reverts to stdio-only MCPs.

**Options:**
- `--silent` - Suppress success messages (for automation)

**Examples:**

```bash
# Disable with output
mcp-wizard disable-global-mcps

# Silent mode (for scripts)
mcp-wizard disable-global-mcps --silent
```

**Output:**

```
Disabling global MCP discovery...

✓ Global MCP discovery disabled
  Sessions will use stdio MCPs only.

Config saved to: ~/.config/mcp-wizard/config.json
```

### Check Status

```bash
mcp-wizard status
```

Shows current MCP setup status including global MCP health.

**Output (when enabled):**

```
Checking MCP setup status...

Environment:
  ✓ Work machine: hostname-w
  ✓ Node.js: v20.0.0

MCP Servers:
  Google Docs MCP:
    ✓ Installed: ~/mcp-servers/google-docs-mcp
    ✓ Credentials: present
    ✓ Authenticated: yes
    ✓ Configured: ~/.config/claude-code/mcp.json

  Atlassian MCP:
    ℹ  Remote MCP (authenticate on first use)

Global MCP Status:
  Enabled: ✓
  Health URL: http://localhost:8001/health
  Server Status: ✓ Healthy (uptime: 12345s)
  Active Sessions: 3
  Temporal UI: http://localhost:8088

✓ Overall: All systems operational
```

### Health Checks

```bash
mcp-wizard health [options]
```

Runs health checks including global MCP server status.

**Options:**
- `--silent` - Exit code only, no output
- `--json` - Output JSON format
- `--force` - Bypass cache, run fresh checks

**Examples:**

```bash
# Quick health check
mcp-wizard health

# JSON output for scripting
mcp-wizard health --json

# Force fresh check
mcp-wizard health --force
```

**Output:**

```
Overall Status: HEALTHY

Component Health:
  ✓ Token: healthy
     Google token valid (expires in 45 minutes)
  ✓ MCP: healthy
     2/2 processes alive
  ✓ Network: healthy
     All 3 endpoints reachable
  ✓ Intent Analyzer: healthy
     Confidence 85%, 0 mismatches
  ✓ Global MCP: healthy
     HTTP server healthy (uptime: 12345s)
```

**Exit Codes:**
- `0` - All checks healthy
- `1` - One or more warnings (degraded)
- `2` - One or more errors (unhealthy)

## Health Check Details

The global MCP health check performs the following:

1. **Configuration Check**: Verifies `globalMcps.enabled` in config
2. **HTTP Health Check**:
   - Makes GET request to `healthCheckUrl`
   - 5-second timeout
   - Expects JSON response with `{ uptime, sessionCount }`
3. **Status Evaluation**:
   - **Healthy**: Server responds with HTTP 200 OK
   - **Unhealthy**: Server unreachable or HTTP error status

### Health Check Response Format

Expected JSON response from health endpoint:

```json
{
  "uptime": 12345,
  "sessionCount": 3,
  "status": "healthy",
  "timestamp": "2026-02-15T10:30:00Z"
}
```

## Integration with mcp-server

Global MCP support is designed to work with the `mcp-server` component (separate package).

**Prerequisites:**
1. Install and configure `mcp-server`
2. Start the global MCP server:
   ```bash
   mcp-server start
   ```
3. Enable global MCPs in mcp-wizard:
   ```bash
   mcp-wizard enable-global-mcps
   ```

**Verification:**
```bash
# Check server is running
curl http://localhost:8001/health

# Check mcp-wizard can connect
mcp-wizard health
```

## Temporal Workflow Integration

Global MCPs integrate with Temporal for workflow orchestration:

1. **Workflow URL**: Configured via `temporalUrl` (default: http://localhost:7233)
2. **Web UI**: Accessible at http://localhost:8088
3. **Workflows**: MCPServiceWorkflow manages MCP lifecycle

**Access Temporal UI:**
```bash
# Open Temporal Web UI
open http://localhost:8088
```

## Troubleshooting

### Health Check Fails

**Symptom:** `mcp-wizard health` shows Global MCP as unhealthy

**Causes:**
- Global MCP server not running
- Incorrect health URL in config
- Network connectivity issues

**Solutions:**
```bash
# Check if server is running
curl http://localhost:8001/health

# Verify config
mcp-wizard config get globalMcps.healthCheckUrl

# Update health URL if needed
mcp-wizard enable-global-mcps --health-url http://correct-url/health

# Check server logs
journalctl -u mcp-server -f  # systemd
launchctl list | grep mcp-server  # launchd
```

### Status Shows "Global MCPs not enabled"

**Symptom:** `mcp-wizard status` doesn't show global MCP section

**Cause:** Global MCPs not enabled in config

**Solution:**
```bash
mcp-wizard enable-global-mcps
```

### Temporal UI Not Accessible

**Symptom:** Cannot access http://localhost:8088

**Causes:**
- Temporal server not running
- Incorrect Temporal URL in config

**Solutions:**
```bash
# Check Temporal server status
temporal server status

# Update Temporal URL
mcp-wizard enable-global-mcps --temporal-url http://correct-url:7233

# Start Temporal server (if not running)
temporal server start-dev
```

## API Reference

### TypeScript Interfaces

#### UserConfig

```typescript
interface UserConfig {
  company: {
    name: string;
    glean_instance: string;
    okta_domain: string;
  };
  globalMcps?: {
    enabled: boolean;
    discoveryUrl?: string;
    healthCheckUrl?: string;
    temporalUrl?: string;
  };
}
```

#### McpServer

```typescript
interface McpServer {
  command: string;
  args: string[];
  defer?: boolean;
  env?: Record<string, string>;
  global?: boolean;   // Mark as global (always available)
  httpUrl?: string;   // HTTP endpoint for global MCPs
}
```

#### HealthCheckResult

```typescript
interface HealthCheckResult {
  name: string;
  status: 'healthy' | 'degraded' | 'unhealthy';
  message: string;
  details?: Record<string, any>;
  last_check: Date;
}
```

### Functions

#### enableGlobalMcpsCommand

```typescript
async function enableGlobalMcpsCommand(
  options: EnableGlobalMcpsOptions
): Promise<void>
```

Enables global MCP discovery with optional custom URLs.

#### disableGlobalMcpsCommand

```typescript
async function disableGlobalMcpsCommand(
  options: DisableGlobalMcpsOptions
): Promise<void>
```

Disables global MCP discovery.

#### checkGlobalMCPHealth

```typescript
async function checkGlobalMCPHealth(
  options: HealthCheckOptions
): Promise<HealthCheckResult>
```

Checks health of global MCP server via HTTP endpoint.

## Best Practices

1. **Enable Only When Needed**: Global MCPs add complexity. Use stdio MCPs for single-session use cases.

2. **Monitor Health**: Regularly check global MCP health:
   ```bash
   mcp-wizard health --json | jq '.checks.globalMcp'
   ```

3. **Use Custom URLs for Non-Standard Setups**:
   ```bash
   mcp-wizard enable-global-mcps \
     --health-url http://custom-host:9000/health
   ```

4. **Disable When Not Using**:
   ```bash
   mcp-wizard disable-global-mcps
   ```

5. **Check Before Sessions**: Add to shell profile:
   ```bash
   # ~/.bashrc or ~/.zshrc
   alias claude-check='mcp-wizard health'
   ```

## Security Considerations

- **Network Exposure**: Global MCPs listen on network ports. Use firewall rules to restrict access.
- **Authentication**: Ensure MCP server implements authentication for production use.
- **Encryption**: Use HTTPS for production deployments (configure via custom URLs).

## Related Documentation

- [mcp-server Setup Guide](../../mcp-server/README.md)
- [Temporal Integration](./TEMPORAL-INTEGRATION.md)
- [Health Checks Reference](./HEALTH-CHECKS.md)
- [Troubleshooting](../TROUBLESHOOTING.md)

## Version History

- **v0.1.0** (2026-02-15): Initial implementation of global MCP support
  - CLI commands: `enable-global-mcps`, `disable-global-mcps`
  - Health check integration
  - Status command enhancement
  - Temporal workflow integration
