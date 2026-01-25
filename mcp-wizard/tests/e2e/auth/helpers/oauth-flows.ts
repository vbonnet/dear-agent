/**
 * Shared OAuth flow test helpers (MCP-agnostic)
 */

import type { MCPOAuthConfig } from '../fixtures/mcp-configs';

/**
 * Test device flow for any MCP
 * Validates: device code request → token polling → success
 */
export async function testDeviceFlow(
  config: MCPOAuthConfig,
  deviceFlowAuth: Function
): Promise<void> {
  const result = await deviceFlowAuth({
    oktaDomain: 'localhost:8080',
    clientId: config.clientId,
    scopes: config.scopes
  });

  // Standard assertions (reusable across all MCPs)
  expect(result.access_token).toBeDefined();
  expect(result.refresh_token).toBeDefined();
  expect(result.expires_in).toBeGreaterThan(0);
}

/**
 * Test PKCE flow for any MCP
 * Validates: code_challenge → authorization code → code_verifier → tokens
 */
export async function testPKCEFlow(
  config: MCPOAuthConfig,
  pkceFlowAuth: Function
): Promise<void> {
  const result = await pkceFlowAuth({
    authEndpoint: config.authEndpoint,
    tokenEndpoint: config.tokenEndpoint,
    clientId: config.clientId,
    redirectUri: 'http://localhost:3000/callback',
    scopes: config.scopes
  });

  expect(result.access_token).toBeDefined();
  expect(result.refresh_token).toBeDefined();
}

/**
 * Test OAuth error scenario for any MCP
 * Validates: OAuth provider returns error → auth flow throws expected error
 */
export async function testOAuthError(
  config: MCPOAuthConfig,
  deviceFlowAuth: Function,
  errorCode: string,
  expectedMessage: string,
  mockServerSetup: Function
): Promise<void> {
  // Configure mock server to return error
  mockServerSetup(errorCode);

  await expect(
    deviceFlowAuth({
      oktaDomain: 'localhost:8080',
      clientId: config.clientId,
      scopes: config.scopes
    })
  ).rejects.toThrow(expectedMessage);
}

/**
 * Advance Jest fake timers for device flow polling
 */
export async function advancePolling(intervals: number): Promise<void> {
  for (let i = 0; i < intervals; i++) {
    await jest.advanceTimersByTimeAsync(1000); // 1s interval (fast for tests)
  }
}
