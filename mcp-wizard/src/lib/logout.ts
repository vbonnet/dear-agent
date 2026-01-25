/**
 * Logout command - revoke OAuth tokens and clear local storage
 *
 * Implements OAuth 2.0 Token Revocation (RFC 7009) for secure logout.
 * Uses best-effort strategy: clears local keychain even if remote revocation fails.
 *
 * @module logout
 */

import { google } from 'googleapis';
import { getOktaToken, deleteOktaToken, TokenResponse } from './token-storage';

/**
 * Options for logout command
 */
export interface LogoutOptions {
  /** Suppress success messages (for automation) */
  silent?: boolean;
}

/**
 * Logout command - revoke tokens and clear local storage
 *
 * Implements OAuth 2.0 Token Revocation (RFC 7009)
 * Best-effort: Clears local storage even if revocation fails
 *
 * @param options - Command options
 * @throws Error if token retrieval fails critically
 *
 * @example
 * await logoutCommand();  // Interactive mode
 * await logoutCommand({ silent: true });  // Silent mode
 */
export async function logoutCommand(options: LogoutOptions = {}): Promise<void> {
  const { silent = false } = options;

  try {
    // Step 1: Retrieve tokens from keychain
    let token: TokenResponse;
    try {
      token = await getOktaToken();
    } catch (error: any) {
      // Check if error is "no token found" (already logged out)
      if (error.message && error.message.includes('No token found')) {
        if (!silent) {
          console.log('Already logged out');
        }
        return;
      }
      // Other errors (keychain unavailable, etc.) are critical
      throw error;
    }

    // Step 2: Create OAuth2 client for revocation
    const oauth2Client = new google.auth.OAuth2(
      token.client_id,
      token.client_secret,
      'http://localhost' // Redirect URI not needed for revocation
    );

    // Step 3: Revoke tokens at Okta (best-effort)
    let revocationSuccessful = false;
    try {
      // Revoke access token first (if present)
      if (token.access_token) {
        await revokeTokenAtOkta(token.access_token, 'access_token', oauth2Client);
      }

      // Then revoke refresh token (always present)
      await revokeTokenAtOkta(token.refresh_token, 'refresh_token', oauth2Client);

      revocationSuccessful = true;

      if (!silent) {
        console.log('✓ Tokens revoked at Google OAuth server');
      }
    } catch (error: any) {
      // Network or server error - warn but continue to local cleanup
      console.warn('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      console.warn(`  Error: ${error.message}`);
      console.warn('  Local credentials will still be cleared');
      console.warn('  Run \'mcp-wizard logout\' again when online to ensure revocation');
    }

    // Step 4: Clear local keychain (critical - must succeed)
    try {
      await deleteOktaToken();
      if (!silent) {
        console.log('✓ Local credentials cleared');
      }
    } catch (error: any) {
      // This is critical - keychain cleanup failed
      throw new Error(`Failed to clear local credentials: ${error.message}`);
    }

  } catch (error: any) {
    // Re-throw critical errors (not best-effort failures)
    if (!error.message?.includes('Failed to revoke tokens')) {
      throw error;
    }
  }
}

/**
 * Revoke a single token at Google OAuth server
 *
 * Uses OAuth 2.0 Token Revocation endpoint (RFC 7009)
 *
 * @param token - Token value to revoke
 * @param tokenTypeHint - Type of token (access_token or refresh_token)
 * @param oauth2Client - Configured OAuth2 client
 * @throws Error if revocation fails (network, server error, etc.)
 *
 * @internal
 */
async function revokeTokenAtOkta(
  token: string,
  tokenTypeHint: 'access_token' | 'refresh_token',
  oauth2Client: any
): Promise<void> {
  try {
    // Google OAuth2 client has a built-in revokeToken method
    // It calls: POST https://oauth2.googleapis.com/revoke
    // With body: token={token}
    await oauth2Client.revokeToken(token);
  } catch (error: any) {
    // Handle specific error cases
    if (error.message && error.message.includes('invalid_token')) {
      // Token already revoked or expired - this is OK (idempotent)
      return;
    }

    if (error.code === 'ENOTFOUND' || error.code === 'ETIMEDOUT' || error.code === 'ECONNREFUSED') {
      // Network error
      throw new Error(`Network unreachable: ${error.message}`);
    }

    // Server error or other issue
    throw new Error(`Revocation failed: ${error.message}`);
  }
}

/**
 * Check if a token exists in keychain (helper for testing)
 *
 * @returns true if token exists, false otherwise
 *
 * @internal
 */
export async function hasToken(): Promise<boolean> {
  try {
    await getOktaToken();
    return true;
  } catch (error) {
    return false;
  }
}
