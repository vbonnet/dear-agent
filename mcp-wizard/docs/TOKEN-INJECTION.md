# Token Injection Layer

**Part of Phase 3-v2 Context Broker Architecture (oss-n1nq.18)**

## Overview

The Token Injection Layer provides automatic token management for MCP (Model Context Protocol) processes. It ensures MCP servers always have valid Okta tokens by:

1. **Proactive Refresh**: Automatically refreshes tokens at 50% TTL (30 minutes for 1-hour tokens)
2. **Re-authentication**: Triggers OAuth flow when tokens expire
3. **Environment Detection**: Uses Device Flow (SSH) or PKCE Flow (local browser)
4. **Transparent Integration**: Single function call handles all token complexity

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Token Injection Layer                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  spawnMCPWithToken(mcpCmd, config)                         │
│         │                                                   │
│         ├─> getValidOktaToken()                            │
│         │       │                                           │
│         │       ├─> Check token health                     │
│         │       │       - Valid? Return token              │
│         │       │       - Expired? Re-authenticate         │
│         │       │       - Needs refresh? Proactive refresh │
│         │       │                                           │
│         │       ├─> Refresh (if TTL < 50%)                 │
│         │       │       - POST /oauth2/v1/token            │
│         │       │       - Update keychain                  │
│         │       │                                           │
│         │       └─> Re-authenticate (if expired)           │
│         │               - Device Flow (headless)           │
│         │               - PKCE Flow (interactive)          │
│         │                                                   │
│         └─> spawn(mcpCmd, { env: { OKTA_TOKEN } })        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Token Lifecycle

```
Token Created (60 min TTL)
    │
    ├─ 0-30 min: ✓ Valid, no action
    │
    ├─ 30-55 min: ⚠ Proactive refresh (TTL < 50%)
    │     │
    │     ├─ Success: New token (60 min TTL)
    │     └─ Failure: Re-authenticate
    │
    └─ 55-60 min: ✗ Re-authenticate (TTL < 5 min)
```

## API Reference

### `spawnMCPWithToken(mcpCmd, config)`

Spawns MCP process with valid Okta token.

**Parameters:**
- `mcpCmd: string[]` - MCP command array (e.g., `['mcp-server-gdocs', '--port', '3000']`)
- `config: TokenInjectionConfig` - Token configuration

**Returns:** `Promise<ChildProcess>` - Child process handle

**Example:**
```typescript
import { spawnMCPWithToken } from 'mcp-wizard';

const mcp = await spawnMCPWithToken(
  ['mcp-server-gdocs', '--port', '3000'],
  {
    oktaDomain: '[REDACTED_EMPLOYER].okta.com',
    clientId: 'your-client-id',
    scopes: ['openid', 'profile', 'email'],
  }
);

console.log('MCP spawned with PID:', mcp.pid);
```

### `getValidOktaToken(config)`

Retrieves valid Okta token (handles refresh/re-auth).

**Parameters:**
- `config: TokenInjectionConfig` - Token configuration

**Returns:** `Promise<string>` - Valid access token

**Example:**
```typescript
const token = await getValidOktaToken({
  oktaDomain: '[REDACTED_EMPLOYER].okta.com',
  clientId: 'your-client-id',
  scopes: ['openid', 'profile', 'email'],
});

// Use token directly
fetch('https://api.example.com/data', {
  headers: { Authorization: `Bearer ${token}` },
});
```

### `checkTokenHealth(token)`

Evaluates token validity and refresh needs.

**Parameters:**
- `token: TokenResponse | undefined` - Token to check

**Returns:** `TokenHealth` - Health status object

**Example:**
```typescript
import { getOktaToken } from 'mcp-wizard/lib/token-storage';
import { checkTokenHealth } from 'mcp-wizard';

const token = await getOktaToken();
const health = checkTokenHealth(token);

if (health.isExpired) {
  console.log('Token expired, re-authentication required');
} else if (health.needsRefresh) {
  console.log(`Token expires in ${health.remainingTTL / 60000} minutes`);
}
```

### `needsTokenRefresh(config)`

Checks if token needs refresh (public API for health monitoring).

**Parameters:**
- `config: TokenInjectionConfig` - Token configuration

**Returns:** `Promise<boolean>` - True if refresh/re-auth needed

**Example:**
```typescript
if (await needsTokenRefresh(config)) {
  console.log('⚠ Token health check: needs refresh');
}
```

### `refreshOktaToken(config, currentToken)`

Refreshes Okta token using refresh_token grant.

**Parameters:**
- `config: TokenInjectionConfig` - Token configuration
- `currentToken: TokenResponse` - Current token with refresh_token

**Returns:** `Promise<TokenResponse>` - Refreshed token

**Example:**
```typescript
const currentToken = await getOktaToken();
const refreshedToken = await refreshOktaToken(config, currentToken);
console.log('Token refreshed, new expiry:', refreshedToken.expires_at);
```

## Configuration

### `TokenInjectionConfig`

```typescript
interface TokenInjectionConfig {
  oktaDomain: string;        // e.g., '[REDACTED_EMPLOYER].okta.com'
  clientId: string;          // OAuth client ID
  scopes: string[];          // e.g., ['openid', 'profile', 'email']
  clientSecret?: string;     // Optional: for confidential clients
}
```

### `TokenHealth`

```typescript
interface TokenHealth {
  valid: boolean;            // Token exists and not expired
  expiresAt?: number;        // Expiration timestamp (ms)
  remainingTTL?: number;     // Time until expiry (ms)
  needsRefresh: boolean;     // True if TTL < 50% or expired
  isExpired: boolean;        // True if TTL < 5 min or expired
}
```

## Token Refresh Strategy

### Thresholds

| Condition | Action | TTL Remaining |
|-----------|--------|---------------|
| Fresh token | No action | > 50% (> 30 min for 1hr token) |
| Needs refresh | Proactive refresh | < 50%, > 5 min |
| Expired | Re-authenticate | < 5 min |

### Refresh Algorithm

```typescript
async function getValidOktaToken(config) {
  // 1. Retrieve token from keychain
  let token = await getOktaToken();

  // 2. Check health
  const health = checkTokenHealth(token);

  // 3. Handle expired (< 5 min)
  if (health.isExpired) {
    await authenticate(config);  // Device Flow or PKCE
    token = await getOktaToken();
    return token.access_token;
  }

  // 4. Proactive refresh (< 50% TTL)
  if (health.needsRefresh) {
    try {
      token = await refreshOktaToken(config, token);
    } catch (error) {
      // Fall back to re-authentication
      await authenticate(config);
      token = await getOktaToken();
    }
  }

  // 5. Return valid token
  return token.access_token;
}
```

## Integration Examples

### Context Broker Integration

```typescript
import { spawnMCPWithToken } from 'mcp-wizard';

class MCPManager {
  private mcpProcesses: Map<string, ChildProcess> = new Map();

  async spawnMCP(mcpName: string, port: number) {
    const config = {
      oktaDomain: process.env.OKTA_DOMAIN!,
      clientId: process.env.OKTA_CLIENT_ID!,
      scopes: ['openid', 'profile', 'email'],
    };

    const process = await spawnMCPWithToken(
      [`mcp-server-${mcpName}`, '--port', port.toString()],
      config
    );

    this.mcpProcesses.set(mcpName, process);
    return process;
  }

  async spawnAllMCPs() {
    await Promise.all([
      this.spawnMCP('gdocs', 3000),
      this.spawnMCP('atlassian', 3001),
      this.spawnMCP('slack', 3002),
    ]);
  }
}
```

### Health Monitoring

```typescript
import { needsTokenRefresh } from 'mcp-wizard';

class HealthMonitor {
  async checkOktaToken() {
    const config = {
      oktaDomain: process.env.OKTA_DOMAIN!,
      clientId: process.env.OKTA_CLIENT_ID!,
      scopes: ['openid', 'profile', 'email'],
    };

    const needsRefresh = await needsTokenRefresh(config);

    return {
      name: 'Okta Token',
      status: needsRefresh ? 'warning' : 'healthy',
      message: needsRefresh
        ? 'Token expires soon, will refresh on next MCP spawn'
        : 'Token is valid',
    };
  }
}
```

## Error Handling

### Re-authentication Triggers

The layer automatically re-authenticates when:

1. **No token found**: `getOktaToken()` throws error
2. **Token expired**: `expires_at < Date.now()`
3. **Token expiring soon**: `remainingTTL < 5 minutes`
4. **Refresh fails**: HTTP 400/401 from Okta

### Fallback Strategy

```
1. Try to refresh token
   ├─ Success → Return new token
   └─ Failure → Re-authenticate

2. Re-authenticate
   ├─ Device Flow (headless: SSH, Cloud Shell)
   └─ PKCE Flow (interactive: local browser)

3. Verify new token
   ├─ Valid → Return token
   └─ Still expired → Throw error
```

## Testing

### Unit Tests

```bash
npm test -- token-injection.test.ts
```

**Coverage:** 96.62% (30 test cases)

Test categories:
- Token health checking (8 tests)
- Token refresh logic (7 tests)
- Valid token retrieval (6 tests)
- MCP spawning (3 tests)
- Health monitoring (4 tests)
- Integration scenarios (2 tests)

### Test Scenarios

| Scenario | Expected Behavior |
|----------|-------------------|
| Fresh token (55 min) | No action, return token |
| Token at 40% TTL | Proactive refresh |
| Token at 60% TTL | No action, return token |
| Expired token | Re-authenticate |
| No token found | Re-authenticate |
| Refresh fails | Fall back to re-auth |
| Spawn with valid token | Pass token to MCP |
| Spawn with expired token | Re-auth then spawn |

## Dependencies

- `~/src/ws/[REDACTED_EMPLOYER]/repos/vida/pr-extraction/libraries/[REDACTED_EMPLOYER]-mcp/src/lib/token-storage.ts` - Token storage/retrieval
- `~/src/ws/[REDACTED_EMPLOYER]/repos/vida/pr-extraction/libraries/[REDACTED_EMPLOYER]-mcp/src/lib/auth.ts` - OAuth flows (Device/PKCE)
- `~/src/ws/[REDACTED_EMPLOYER]/repos/vida/pr-extraction/libraries/[REDACTED_EMPLOYER]-mcp/src/lib/errors.ts` - Retry logic, error sanitization

## Security Considerations

1. **Token Storage**: Uses OS keychain (macOS Keychain, Linux Secret Service)
2. **Environment Variables**: OKTA_TOKEN only visible to spawned MCP processes
3. **Error Sanitization**: Tokens redacted from error messages
4. **No Plaintext**: Never stores tokens in plaintext files

## Performance

- **Spawn Time**: ~100ms (cached token, no refresh)
- **Refresh Time**: ~500ms (Okta API call)
- **Re-auth Time**: 10-30s (user interaction required)
- **Memory**: Minimal (<1MB per spawned process)

## Troubleshooting

### Token Refresh Fails

**Symptoms:** `Token refresh failed: invalid_grant`

**Causes:**
- Refresh token expired (> 90 days unused)
- Refresh token revoked by admin
- Client ID/secret mismatch

**Solution:** Run `mcp-wizard auth` to re-authenticate

### Re-authentication Loop

**Symptoms:** Repeated authentication prompts

**Causes:**
- System clock incorrect (token expires_at calculation off)
- Okta token TTL < 5 minutes (misconfiguration)

**Solution:**
1. Check system clock: `date`
2. Verify Okta token policy: Min TTL = 60 minutes

### MCP Spawn Fails

**Symptoms:** `Failed to spawn MCP process: ENOENT`

**Causes:**
- MCP binary not installed
- MCP binary not in PATH

**Solution:**
1. Install MCP: `npm install -g @modelcontextprotocol/server-gdocs`
2. Verify: `which mcp-server-gdocs`

## Related Documentation

- [ARCHITECTURE.md](~/src/ws/[REDACTED_EMPLOYER]/wf/context-broker-architecture-design/ARCHITECTURE.md) - Phase 3-v2 Context Broker Architecture
- [auth.ts](~/src/ws/[REDACTED_EMPLOYER]/repos/vida/pr-extraction/libraries/[REDACTED_EMPLOYER]-mcp/src/lib/auth.ts) - OAuth Device Flow & PKCE
- [token-storage.ts](~/src/ws/[REDACTED_EMPLOYER]/repos/vida/pr-extraction/libraries/[REDACTED_EMPLOYER]-mcp/src/lib/token-storage.ts) - Keychain token storage

## Support

- **Issues**: File in VIDA repository (vida/pr-extraction/libraries/[REDACTED_EMPLOYER]-mcp)
- **Slack**: #vida-dev
- **Email**: vida-dev@[REDACTED_DOMAIN]
