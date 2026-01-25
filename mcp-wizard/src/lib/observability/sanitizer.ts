/**
 * Secret sanitization for logging
 *
 * Provides utilities to redact sensitive data from log output.
 * Defense-in-depth approach: use with Pino's built-in redaction.
 *
 * @module observability/sanitizer
 */

import { TokenResponse } from '../token-storage';

/**
 * Sanitized token data (safe for logging)
 *
 * Contains only non-sensitive metadata.
 * NEVER includes: client_id, client_secret, refresh_token, access_token
 */
export interface SanitizedToken {
  /** Token type (e.g., 'authorized_user') */
  type: string;
  /** Whether refresh token exists (boolean, not actual token) */
  has_refresh_token: boolean;
  /** Whether access token exists (boolean, not actual token) */
  has_access_token: boolean;
  /** Token expiration timestamp (safe to log) */
  expires_at?: number;
}

/**
 * Sanitize TokenResponse for safe logging
 *
 * Removes all sensitive data (tokens, client secrets) and returns
 * only non-sensitive metadata.
 *
 * @param token - Token data to sanitize
 * @returns Sanitized token (safe for logging)
 *
 * @example
 * const token = await getOktaToken();
 * const safe = sanitizeToken(token);
 * logger.info({ result: safe }); // Safe: no secrets logged
 */
export function sanitizeToken(token: TokenResponse): SanitizedToken {
  return {
    type: token.type,
    has_refresh_token: !!token.refresh_token,
    has_access_token: !!token.access_token,
    expires_at: token.expires_at,
  };
}
