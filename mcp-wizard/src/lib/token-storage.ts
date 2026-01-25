/**
 * Token storage using OS-native keychain
 *
 * Replaces plaintext file storage with secure OS keychain:
 * - macOS: Keychain Access (AES-256 encryption)
 * - Linux: Secret Service (libsecret/gnome-keyring)
 * - Windows: Credential Manager (untested, best-effort)
 *
 * @module token-storage
 */

import * as keytar from 'keytar';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { withObservability } from './observability/wrapper';
import { sanitizeToken } from './observability/sanitizer';
import { classifyError } from './observability/errors';

/**
 * OAuth token structure matching Google APIs format
 */
export interface TokenResponse {
  type: string;              // "authorized_user"
  client_id: string;         // OAuth client ID
  client_secret: string;     // OAuth client secret
  refresh_token: string;     // Long-lived refresh token
  access_token?: string;     // Short-lived access token (optional)
  expires_at?: number;       // Expiration timestamp (optional)
}

// Keychain service name (namespace for all token entries)
const SERVICE_NAME = 'mcp-wizard-google-oauth';

// Keychain account names (keys for individual token fields)
const ACCOUNT_TYPE = 'type';
const ACCOUNT_CLIENT_ID = 'client-id';
const ACCOUNT_CLIENT_SECRET = 'client-secret';
const ACCOUNT_REFRESH_TOKEN = 'refresh-token';
const ACCOUNT_ACCESS_TOKEN = 'access-token';
const ACCOUNT_EXPIRES_AT = 'expires-at';

/**
 * Store OAuth token in OS keychain
 *
 * @param token - Token data to store
 * @throws Error if keychain service unavailable
 *
 * @example
 * await storeOktaToken({
 *   type: 'authorized_user',
 *   client_id: '123.apps.googleusercontent.com',
 *   client_secret: 'GOCSPX-abc123',
 *   refresh_token: '1//refresh-token-here'
 * });
 */
export async function storeOktaToken(token: TokenResponse): Promise<void> {
  return withObservability(
    'storeToken',
    async () => {
      try {
        // Store required fields
        await keytar.setPassword(SERVICE_NAME, ACCOUNT_TYPE, token.type);
        await keytar.setPassword(SERVICE_NAME, ACCOUNT_CLIENT_ID, token.client_id);
        await keytar.setPassword(SERVICE_NAME, ACCOUNT_CLIENT_SECRET, token.client_secret);
        await keytar.setPassword(SERVICE_NAME, ACCOUNT_REFRESH_TOKEN, token.refresh_token);

        // Store optional fields if present
        if (token.access_token) {
          await keytar.setPassword(SERVICE_NAME, ACCOUNT_ACCESS_TOKEN, token.access_token);
        }
        if (token.expires_at !== undefined) {
          await keytar.setPassword(SERVICE_NAME, ACCOUNT_EXPIRES_AT, token.expires_at.toString());
        }
      } catch (error: any) {
        // Enhance error message for common issues
        if (error.message && error.message.includes('libsecret')) {
          throw new Error(
            `Keychain service unavailable. Install libsecret:\n` +
            `  Debian/Ubuntu: sudo apt-get install libsecret-1-dev\n` +
            `  Fedora/RHEL: sudo dnf install libsecret-devel`
          );
        }
        throw new Error(`Failed to store token in keychain: ${error.message}`);
      }
    },
    {
      retry: true,
      sanitize: () => ({ stored: true }),
    }
  );
}

/**
 * Retrieve OAuth token from OS keychain
 *
 * @returns Token data
 * @throws Error if token not found or keychain unavailable
 *
 * @example
 * const token = await getOktaToken();
 * console.log(token.refresh_token); // '1//refresh-token-here'
 */
export async function getOktaToken(): Promise<TokenResponse> {
  return withObservability(
    'getToken',
    async () => {
      try {
        // Retrieve required fields
        const type = await keytar.getPassword(SERVICE_NAME, ACCOUNT_TYPE);
        const clientId = await keytar.getPassword(SERVICE_NAME, ACCOUNT_CLIENT_ID);
        const clientSecret = await keytar.getPassword(SERVICE_NAME, ACCOUNT_CLIENT_SECRET);
        const refreshToken = await keytar.getPassword(SERVICE_NAME, ACCOUNT_REFRESH_TOKEN);

        // Check if required fields exist
        if (!type || !clientId || !clientSecret || !refreshToken) {
          throw new Error('No token found - run: mcp-wizard auth');
        }

        // Retrieve optional fields
        const accessToken = await keytar.getPassword(SERVICE_NAME, ACCOUNT_ACCESS_TOKEN);
        const expiresAtStr = await keytar.getPassword(SERVICE_NAME, ACCOUNT_EXPIRES_AT);

        const token: TokenResponse = {
          type,
          client_id: clientId,
          client_secret: clientSecret,
          refresh_token: refreshToken,
        };

        // Add optional fields if present
        if (accessToken) {
          token.access_token = accessToken;
        }
        if (expiresAtStr) {
          token.expires_at = parseInt(expiresAtStr, 10);
        }

        return token;
      } catch (error: any) {
        // Re-throw our custom error messages
        if (error.message && error.message.includes('No token found')) {
          throw error;
        }
        throw new Error(`Failed to retrieve token from keychain: ${error.message}`);
      }
    },
    {
      retry: true,
      sanitize: sanitizeToken,
    }
  );
}

/**
 * Delete OAuth token from OS keychain
 *
 * Idempotent: Does not throw if token doesn't exist
 *
 * @example
 * await deleteOktaToken();
 * console.log('Token deleted');
 */
export async function deleteOktaToken(): Promise<void> {
  return withObservability('deleteToken', async () => {
    try {
      // Delete all fields (ignore return values - we don't care if they didn't exist)
      await keytar.deletePassword(SERVICE_NAME, ACCOUNT_TYPE);
      await keytar.deletePassword(SERVICE_NAME, ACCOUNT_CLIENT_ID);
      await keytar.deletePassword(SERVICE_NAME, ACCOUNT_CLIENT_SECRET);
      await keytar.deletePassword(SERVICE_NAME, ACCOUNT_REFRESH_TOKEN);
      await keytar.deletePassword(SERVICE_NAME, ACCOUNT_ACCESS_TOKEN);
      await keytar.deletePassword(SERVICE_NAME, ACCOUNT_EXPIRES_AT);
    } catch (error: any) {
      // Ignore errors on delete (idempotent operation)
      // Most common error: service doesn't exist, which is fine
    }
  });
  // No retry for delete (idempotent operation)
}

/**
 * Migrate plaintext token to keychain (one-time operation)
 *
 * Automatically called during first token storage/retrieval
 *
 * @returns true if migration occurred, false if no plaintext file found
 * @throws Error if plaintext file corrupted or migration fails
 *
 * @example
 * const migrated = await migrateTokensToKeychain();
 * if (migrated) {
 *   console.log('Token migrated from plaintext to keychain');
 * }
 */
export async function migrateTokensToKeychain(): Promise<boolean> {
  return withObservability(
    'migrateTokens',
    async () => {
      // Path to legacy plaintext token file
      const oldTokenPath = path.join(os.homedir(), '.config', 'mcp-wizard', 'okta-token.json');

      try {
        // Check if plaintext file exists
        await fs.access(oldTokenPath);
      } catch (error) {
        // File doesn't exist - no migration needed
        return false;
      }

      try {
        // Read plaintext file
        const plaintextContent = await fs.readFile(oldTokenPath, 'utf-8');

        // Parse JSON
        let plaintextToken: any;
        try {
          plaintextToken = JSON.parse(plaintextContent);
        } catch (parseError) {
          throw new Error('Corrupted token file. Please re-authenticate: mcp-wizard auth');
        }

        // Validate required fields
        if (!plaintextToken.refresh_token || !plaintextToken.client_id || !plaintextToken.client_secret) {
          throw new Error('Invalid token structure. Please re-authenticate: mcp-wizard auth');
        }

        // Default type if not present
        const tokenData: TokenResponse = {
          type: plaintextToken.type || 'authorized_user',
          client_id: plaintextToken.client_id,
          client_secret: plaintextToken.client_secret,
          refresh_token: plaintextToken.refresh_token,
        };

        // Include optional fields if present
        if (plaintextToken.access_token) {
          tokenData.access_token = plaintextToken.access_token;
        }
        if (plaintextToken.expires_at) {
          tokenData.expires_at = plaintextToken.expires_at;
        }

        // Store in keychain
        await storeOktaToken(tokenData);

        // Delete plaintext file (only after successful keychain storage)
        await fs.unlink(oldTokenPath);

        return true; // Migration successful
      } catch (error: any) {
        // If error is from validation/parsing, re-throw with original message
        if (error.message && (error.message.includes('Corrupted') || error.message.includes('Invalid'))) {
          throw error;
        }
        // Otherwise, wrap error
        throw new Error(`Migration failed: ${error.message}`);
      }
    },
    {
      retry: true,
      sanitize: (migrated) => ({ migrated }),
    }
  );
}

/**
 * Health status for token and keychain
 *
 * Returned by checkTokenHealth() function.
 */
export interface HealthStatus {
  /** Overall health (true if token valid and accessible) */
  healthy: boolean;
  /** Token exists in keychain */
  token_exists: boolean;
  /** All required token fields present */
  has_required_fields: boolean;
  /** Keychain service accessible */
  keychain_accessible: boolean;
  /** ISO8601 timestamp of health check */
  checked_at: string;
  /** Error classification (if unhealthy) */
  error_type?: 'transient' | 'permanent';
}

/**
 * Check token and keychain health
 *
 * Non-throwing function that verifies token validity and keychain accessibility.
 * Returns structured status for monitoring and health checks.
 *
 * @returns Health status with detailed diagnostics
 *
 * @example
 * const health = await checkTokenHealth();
 * if (health.healthy) {
 *   console.log('Token is valid');
 * } else {
 *   console.log(`Token unhealthy: ${health.error_type}`);
 * }
 */
export async function checkTokenHealth(): Promise<HealthStatus> {
  return withObservability('checkHealth', async () => {
    try {
      const token = await getOktaToken();
      return {
        healthy: true,
        token_exists: true,
        has_required_fields: !!(
          token.type &&
          token.client_id &&
          token.client_secret &&
          token.refresh_token
        ),
        keychain_accessible: true,
        checked_at: new Date().toISOString(),
      };
    } catch (error: any) {
      const errorType = classifyError(error);
      return {
        healthy: false,
        token_exists: !error.message?.includes('No token found'),
        has_required_fields: false,
        keychain_accessible: errorType !== 'transient',
        checked_at: new Date().toISOString(),
        error_type: errorType,
      };
    }
  });
}
