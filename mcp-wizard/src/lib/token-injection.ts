/**
 * Token Injection Layer for MCP Processes
 *
 * Spawns MCP processes with OKTA_TOKEN environment variable.
 * Implements proactive token refresh at 50% TTL and re-authentication on expiry.
 *
 * Part of Phase 3-v2 Context Broker implementation (oss-n1nq.18)
 *
 * @module token-injection
 */

import { spawn, ChildProcess } from 'child_process';
import { getOktaToken, storeOktaToken, TokenResponse } from './token-storage';
import { authenticate, AuthConfig, detectEnvironment } from './auth';
import { retryWithBackoff, sanitizeError } from './errors';
import { TraceLogger, LogLevel } from './trace-logger';

/**
 * Token TTL threshold for proactive refresh
 * Refresh when remaining TTL < 50% of original TTL
 */
const REFRESH_THRESHOLD = 0.5;

/**
 * Default token TTL in milliseconds (1 hour)
 * Used when expires_at is not available
 */
const DEFAULT_TOKEN_TTL = 60 * 60 * 1000; // 1 hour

/**
 * Minimum token TTL in milliseconds (5 minutes)
 * Re-authenticate if token expires in less than this time
 */
const MIN_TOKEN_TTL = 5 * 60 * 1000; // 5 minutes

/**
 * Configuration for token injection
 */
export interface TokenInjectionConfig {
  oktaDomain: string;
  clientId: string;
  scopes: string[];
  clientSecret?: string;
}

/**
 * Token health status
 */
export interface TokenHealth {
  valid: boolean;
  expiresAt?: number;
  remainingTTL?: number;
  needsRefresh: boolean;
  isExpired: boolean;
}

/**
 * Check token health and determine if refresh is needed
 *
 * Evaluates token based on:
 * - Existence check
 * - Expiration time
 * - Remaining TTL vs refresh threshold (50%)
 *
 * @param token - Token to check (optional)
 * @returns Token health status
 *
 * @example
 * const token = await getOktaToken();
 * const health = checkTokenHealth(token);
 * if (health.needsRefresh) {
 *   // Refresh token
 * }
 */
export function checkTokenHealth(token?: TokenResponse): TokenHealth {
  const tracer = TraceLogger.getInstance();

  // No token = expired
  if (!token || !token.access_token) {
    const health = {
      valid: false,
      needsRefresh: true,
      isExpired: true,
    };
    tracer.log(LogLevel.DEBUG, 'token_health_check', {
      service: 'okta',
      valid: health.valid,
      is_expired: health.isExpired,
      needs_refresh: health.needsRefresh,
      reason: 'no_token',
    });
    return health;
  }

  // No expiry time = assume valid but needs refresh
  if (!token.expires_at) {
    const health = {
      valid: true,
      needsRefresh: true,
      isExpired: false,
    };
    tracer.log(LogLevel.DEBUG, 'token_health_check', {
      service: 'okta',
      valid: health.valid,
      is_expired: health.isExpired,
      needs_refresh: health.needsRefresh,
      reason: 'no_expiry',
    });
    return health;
  }

  const now = Date.now();
  const expiresAt = token.expires_at;
  const remainingTTL = expiresAt - now;

  // Token expired
  if (remainingTTL <= 0) {
    const health = {
      valid: false,
      expiresAt,
      remainingTTL: 0,
      needsRefresh: true,
      isExpired: true,
    };
    tracer.log(LogLevel.DEBUG, 'token_health_check', {
      service: 'okta',
      valid: health.valid,
      expires_at: health.expiresAt,
      remaining_ttl_ms: health.remainingTTL,
      is_expired: health.isExpired,
      needs_refresh: health.needsRefresh,
      reason: 'expired',
    });
    return health;
  }

  // Token expires soon (< 5 minutes)
  if (remainingTTL < MIN_TOKEN_TTL) {
    const health = {
      valid: false,
      expiresAt,
      remainingTTL,
      needsRefresh: true,
      isExpired: true,
    };
    tracer.log(LogLevel.DEBUG, 'token_health_check', {
      service: 'okta',
      valid: health.valid,
      expires_at: health.expiresAt,
      remaining_ttl_ms: health.remainingTTL,
      is_expired: health.isExpired,
      needs_refresh: health.needsRefresh,
      reason: 'expires_soon',
    });
    return health;
  }

  // Calculate original TTL (approximate)
  // If we don't know when token was issued, assume DEFAULT_TOKEN_TTL
  const estimatedTTL = DEFAULT_TOKEN_TTL;
  const refreshThreshold = estimatedTTL * REFRESH_THRESHOLD;

  // Proactive refresh: remaining TTL < 50% of estimated TTL
  const needsRefresh = remainingTTL < refreshThreshold;

  const health = {
    valid: true,
    expiresAt,
    remainingTTL,
    needsRefresh,
    isExpired: false,
  };

  tracer.log(LogLevel.DEBUG, 'token_health_check', {
    service: 'okta',
    valid: health.valid,
    expires_at: health.expiresAt,
    remaining_ttl_ms: health.remainingTTL,
    is_expired: health.isExpired,
    needs_refresh: health.needsRefresh,
    reason: needsRefresh ? 'needs_proactive_refresh' : 'healthy',
  });

  return health;
}

/**
 * Refresh Okta token using refresh_token grant
 *
 * Exchanges refresh_token for new access_token.
 * Updates token storage with new access_token and expires_at.
 *
 * @param config - Token injection configuration
 * @param currentToken - Current token with refresh_token
 * @returns Refreshed token
 * @throws Error if refresh fails (will trigger re-authentication)
 *
 * @example
 * const refreshed = await refreshOktaToken(config, currentToken);
 * // New access_token with updated expires_at
 */
export async function refreshOktaToken(
  config: TokenInjectionConfig,
  currentToken: TokenResponse
): Promise<TokenResponse> {
  const tracer = TraceLogger.getInstance();
  const endTimer = tracer.time('oauth_refresh');

  tracer.log(LogLevel.DEBUG, 'oauth_refresh_start', {
    service: 'okta',
    okta_domain: config.oktaDomain,
  });

  const { oktaDomain, clientId } = config;

  if (!currentToken.refresh_token) {
    const error = new Error('No refresh_token available for token refresh');
    tracer.log(LogLevel.ERROR, 'oauth_refresh_end', {
      service: 'okta',
      result: 'failure',
      error: error.message,
    });
    endTimer();
    throw error;
  }

  const url = `https://${oktaDomain}/oauth2/v1/token`;

  const params = new URLSearchParams({
    grant_type: 'refresh_token',
    refresh_token: currentToken.refresh_token,
    client_id: clientId,
    scope: config.scopes.join(' '),
  });

  // Add client_secret if available (confidential client)
  if (config.clientSecret) {
    params.append('client_secret', config.clientSecret);
  }

  const refreshWithRetry = async (): Promise<TokenResponse> => {
    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: params.toString(),
      });

      const data: any = await response.json();

      // Success case
      if (response.ok) {
        // Validate required fields
        if (!data.access_token || typeof data.expires_in !== 'number') {
          throw new Error('Invalid token refresh response from Okta');
        }

        // Build refreshed token (preserve existing fields, update access_token)
        const refreshedToken: TokenResponse = {
          ...currentToken,
          access_token: data.access_token,
          expires_at: Date.now() + data.expires_in * 1000,
        };

        // Update refresh_token if new one provided
        if (data.refresh_token) {
          refreshedToken.refresh_token = data.refresh_token;
        }

        // Store refreshed token
        await storeOktaToken(refreshedToken);

        tracer.log(LogLevel.DEBUG, 'oauth_refresh_end', {
          service: 'okta',
          result: 'success',
          token_expiry_seconds: refreshedToken.expires_at
            ? Math.floor((refreshedToken.expires_at - Date.now()) / 1000)
            : undefined,
        });
        endTimer();

        return refreshedToken;
      }

      // Error cases
      // HTTP 400/401: Don't retry (invalid refresh token, re-auth needed)
      if (response.status === 400 || response.status === 401) {
        const errorDesc = data.error_description || data.error || 'Unknown error';
        throw new Error(`Token refresh failed: ${errorDesc}`);
      }

      // HTTP 5xx: Will be retried by retryWithBackoff
      throw new Error(`Okta server error (HTTP ${response.status}): ${data.error || 'Unknown error'}`);
    } catch (error: any) {
      throw sanitizeError(error);
    }
  };

  try {
    return await retryWithBackoff(refreshWithRetry, 3, 1000);
  } catch (error: any) {
    tracer.log(LogLevel.ERROR, 'oauth_refresh_end', {
      service: 'okta',
      result: 'failure',
      error: error.message,
    });
    endTimer();
    throw sanitizeError(error);
  }
}

/**
 * Get valid Okta token (with proactive refresh)
 *
 * Algorithm:
 * 1. Retrieve token from keychain
 * 2. Check token health
 * 3. If expired: Re-authenticate
 * 4. If needs refresh: Proactively refresh
 * 5. Return valid token
 *
 * @param config - Token injection configuration
 * @returns Valid access token string
 * @throws Error if token retrieval/refresh fails
 *
 * @example
 * const token = await getValidOktaToken(config);
 * // Token guaranteed valid for at least MIN_TOKEN_TTL
 */
export async function getValidOktaToken(config: TokenInjectionConfig): Promise<string> {
  let token: TokenResponse | undefined;

  // Step 1: Retrieve token from keychain
  try {
    token = await getOktaToken();
  } catch (error: any) {
    // No token found - trigger authentication
    console.log('No token found, initiating authentication...');
    await authenticate(config);
    token = await getOktaToken();
  }

  // Step 2: Check token health
  const health = checkTokenHealth(token);

  // Step 3: Handle expired token (re-authenticate)
  if (health.isExpired) {
    console.log('Token expired, re-authenticating...');
    await authenticate(config);
    token = await getOktaToken();

    // Verify new token is valid
    const newHealth = checkTokenHealth(token);
    if (newHealth.isExpired) {
      throw new Error('Failed to obtain valid token after re-authentication');
    }

    return token.access_token!;
  }

  // Step 4: Handle token needing refresh (proactive)
  if (health.needsRefresh) {
    const remainingMin = health.remainingTTL ? Math.floor(health.remainingTTL / 60000) : 0;
    console.log(`Token expires in ${remainingMin} minutes, refreshing proactively...`);

    try {
      token = await refreshOktaToken(config, token);
    } catch (error: any) {
      // If refresh fails, fall back to re-authentication
      console.log('Token refresh failed, re-authenticating...');
      await authenticate(config);
      token = await getOktaToken();
    }
  }

  // Step 5: Return valid token
  if (!token.access_token) {
    throw new Error('No access_token available after token validation');
  }

  return token.access_token;
}

/**
 * Spawn MCP process with OKTA_TOKEN environment variable
 *
 * Core function for token injection layer.
 * Ensures token is valid before spawning MCP process.
 *
 * Algorithm (from ARCHITECTURE.md:168-177):
 * 1. Get valid Okta token (from keychain, refresh if needed)
 * 2. Spawn MCP process with OKTA_TOKEN env var
 * 3. Return child process handle
 *
 * @param mcpCmd - MCP command array (e.g., ['mcp-server-gdocs', '--port', '3000'])
 * @param config - Token injection configuration
 * @returns Child process handle
 * @throws Error if token retrieval fails or spawn fails
 *
 * @example
 * const mcp = await spawnMCPWithToken(
 *   ['mcp-server-gdocs', '--port', '3000'],
 *   {
 *     oktaDomain: 'company.okta.com',
 *     clientId: 'client-123',
 *     scopes: ['openid', 'profile', 'email'],
 *   }
 * );
 * // MCP process now has OKTA_TOKEN env var
 */
export async function spawnMCPWithToken(
  mcpCmd: string[],
  config: TokenInjectionConfig
): Promise<ChildProcess> {
  const tracer = TraceLogger.getInstance();
  const mcpName = mcpCmd[0]?.split('/').pop() || 'unknown';

  // Log spawn start (command will be sanitized by tracer)
  tracer.log(LogLevel.DEBUG, 'mcp_spawn_start', {
    mcp_name: mcpName,
    command: mcpCmd[0],
    args: mcpCmd.slice(1), // Will be sanitized if contains OKTA_TOKEN
  });

  // Step 1: Get valid Okta token (handles refresh/re-auth)
  const token = await getValidOktaToken(config);

  // Log token injection
  tracer.log(LogLevel.DEBUG, 'token_inject', {
    service: 'okta',
    mcp_name: mcpName,
    token_length: token.length,
  });

  // Step 2: Spawn MCP process with OKTA_TOKEN env var
  const childProcess = spawn(mcpCmd[0], mcpCmd.slice(1), {
    env: { ...process.env, OKTA_TOKEN: token },
    stdio: ['pipe', 'pipe', 'pipe'],
  });

  // Log successful spawn
  if (childProcess.pid) {
    tracer.log(LogLevel.DEBUG, 'mcp_spawn_success', {
      mcp_name: mcpName,
      pid: childProcess.pid,
    });
  }

  // Capture stderr for error logging
  let stderrData = '';
  childProcess.stderr?.on('data', (data) => {
    stderrData += data.toString();
  });

  // Handle spawn errors
  childProcess.on('error', (error) => {
    console.error(`Failed to spawn MCP process: ${error.message}`);
    tracer.log(LogLevel.ERROR, 'mcp_spawn_error', {
      mcp_name: mcpName,
      pid: childProcess.pid,
      error: error.message,
    });
  });

  // Log process exit
  childProcess.on('exit', (code, signal) => {
    tracer.log(code === 0 ? LogLevel.INFO : LogLevel.ERROR, 'mcp_exit', {
      mcp_name: mcpName,
      pid: childProcess.pid,
      exit_code: code,
      signal: signal,
      stderr: code !== 0 ? stderrData.slice(0, 1000) : undefined, // First 1000 chars on error
    });
  });

  // Step 3: Return child process
  return childProcess;
}

/**
 * Check if token needs refresh (public API)
 *
 * Utility function for health monitoring.
 *
 * @param config - Token injection configuration
 * @returns True if token needs refresh or re-authentication
 *
 * @example
 * if (await needsTokenRefresh(config)) {
 *   console.log('Token health check: needs refresh');
 * }
 */
export async function needsTokenRefresh(config: TokenInjectionConfig): Promise<boolean> {
  try {
    const token = await getOktaToken();
    const health = checkTokenHealth(token);
    return health.needsRefresh || health.isExpired;
  } catch (error) {
    // No token found = needs refresh
    return true;
  }
}
