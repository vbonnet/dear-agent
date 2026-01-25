/**
 * Shared test fixtures for auth module tests
 */

import { DeviceCodeResponse, TokenResponse } from '../../src/lib/auth';

/**
 * Mock token responses for various scenarios
 */
export const MOCK_TOKENS = {
  /**
   * Valid access token with refresh token
   */
  valid: {
    access_token: 'mock-access-token-12345',
    token_type: 'Bearer',
    expires_in: 3600,
    refresh_token: 'mock-refresh-token-67890',
    scope: 'openid profile email',
  } as TokenResponse,

  /**
   * Expired token (expires_in = 0)
   */
  expired: {
    access_token: 'mock-expired-token',
    token_type: 'Bearer',
    expires_in: 0,
    refresh_token: 'mock-refresh-token',
    scope: 'openid profile email',
  } as TokenResponse,

  /**
   * Token without refresh token
   */
  noRefresh: {
    access_token: 'mock-access-token-no-refresh',
    token_type: 'Bearer',
    expires_in: 3600,
    scope: 'openid profile email',
  } as TokenResponse,

  /**
   * Invalid token response (missing required fields)
   */
  invalid: {
    access_token: 'mock-access-token',
    // Missing token_type and expires_in
  } as any,
};

/**
 * Mock Okta API responses for various scenarios
 */
export const MOCK_OKTA_RESPONSES = {
  /**
   * Successful device code authorization response
   */
  deviceCodeSuccess: {
    device_code: 'mock-device-code-abc123',
    user_code: 'ABCD-1234',
    verification_uri: 'https://[REDACTED_EMPLOYER].okta.com/activate',
    verification_uri_complete: 'https://[REDACTED_EMPLOYER].okta.com/activate?user_code=ABCD-1234',
    expires_in: 600,
    interval: 5,
  } as DeviceCodeResponse,

  /**
   * Successful token exchange response
   */
  tokenSuccess: {
    access_token: 'mock-access-token-success',
    token_type: 'Bearer',
    expires_in: 3600,
    refresh_token: 'mock-refresh-token-success',
    scope: 'openid profile email',
  } as TokenResponse,

  /**
   * Authorization pending error (user hasn't authorized yet)
   */
  authorizationPending: {
    error: 'authorization_pending',
    error_description: 'The authorization request is still pending',
  },

  /**
   * Slow down error (polling too fast)
   */
  slowDown: {
    error: 'slow_down',
    error_description: 'You are polling too frequently',
  },

  /**
   * Access denied error (user rejected)
   */
  accessDenied: {
    error: 'access_denied',
    error_description: 'The user denied the authorization request',
  },

  /**
   * Expired token error (device code expired)
   */
  expiredToken: {
    error: 'expired_token',
    error_description: 'The device code has expired',
  },

  /**
   * Invalid grant error (authorization code invalid/expired/already used)
   */
  invalidGrant: {
    error: 'invalid_grant',
    error_description: 'The authorization code is invalid or has expired',
  },

  /**
   * Invalid client error (client authentication failed)
   */
  invalidClient: {
    error: 'invalid_client',
    error_description: 'Client authentication failed',
  },

  /**
   * Server error (HTTP 500)
   */
  serverError: {
    error: 'server_error',
    error_description: 'Internal server error',
  },
};

/**
 * Mock PKCE parameters for testing
 */
export const MOCK_PKCE = {
  /**
   * Valid PKCE verifier (128 characters)
   */
  verifier: 'a'.repeat(128),

  /**
   * Valid PKCE challenge (43 characters, SHA-256 base64url of verifier)
   */
  challenge: 'GF-FVBzT1qXOxKb7t5GzpsdTJdnBs0HaGqQX4s4sQ_g',

  /**
   * Valid state parameter (32 characters)
   */
  state: 'mock-state-12345678901234567890',

  /**
   * Authorization code returned by Okta
   */
  authCode: 'mock-auth-code-xyz789',
};

/**
 * Mock environment variables for testing
 */
export const MOCK_ENV = {
  oktaDomain: '[REDACTED_EMPLOYER].okta.com',
  clientId: 'mock-client-id',
  clientSecret: 'mock-client-secret',
  scopes: ['openid', 'profile', 'email'],
};

/**
 * Mock HTTP server responses
 */
export const MOCK_HTML_RESPONSES = {
  success: `<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body>
  <h1>Authentication successful!</h1>
  <p>You can close this window.</p>
</body>
</html>`,

  stateMismatch: `<!DOCTYPE html>
<html>
<head><title>Authentication Failed</title></head>
<body>
  <h1>Authentication failed</h1>
  <p>Invalid state parameter. Possible CSRF attack detected.</p>
  <p>Please try again.</p>
</body>
</html>`,

  missingCode: `<!DOCTYPE html>
<html>
<head><title>Authentication Failed</title></head>
<body>
  <h1>Authentication failed</h1>
  <p>Missing authorization code.</p>
  <p>Please try again.</p>
</body>
</html>`,
};
