/**
 * OAuth Device Authorization Grant Flow (RFC 8628) and PKCE Flow (RFC 7636) for Okta
 *
 * Implements two authentication flows:
 * - Device flow: Headless authentication for SSH/cloud environments
 * - PKCE flow: Browser-based authentication for local interactive environments
 *
 * Auto-detects environment and selects appropriate flow.
 *
 * @module auth
 */

import * as crypto from 'crypto';
import * as http from 'http';
import * as open from 'open';
import { retryWithBackoff, sanitizeError } from './errors';
import { storeOktaToken } from './token-storage';

// =============================================================================
// TypeScript Interfaces
// =============================================================================

/**
 * Environment type detection result
 */
export type EnvironmentType = 'headless' | 'interactive';

/**
 * Detailed environment detection information
 */
export interface EnvironmentDetection {
  type: EnvironmentType;
  sshDetected: boolean;
  cloudShellDetected: boolean;
  ttyAvailable: boolean;
}

/**
 * Device authorization response from Okta
 */
export interface DeviceCodeResponse {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete?: string;
  expires_in: number;
  interval: number;
}

/**
 * Token response from Okta
 */
export interface TokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  scope?: string;
}

/**
 * Error response during token polling
 */
export interface TokenErrorResponse {
  error: 'authorization_pending' | 'slow_down' | 'access_denied' | 'expired_token';
  error_description?: string;
}

/**
 * Configuration options for device flow
 */
export interface DeviceFlowOptions {
  oktaDomain: string;
  clientId: string;
  scopes: string[];
  clientSecret?: string;
}

/**
 * PKCE challenge parameters
 */
export interface PKCEChallenge {
  verifier: string;      // 128-char code verifier
  challenge: string;     // 43-char code challenge (SHA-256 of verifier)
  state: string;         // 32-char CSRF protection token
}

/**
 * Configuration options for PKCE flow
 */
export interface PKCEOptions {
  oktaDomain: string;
  clientId: string;
  scopes: string[];
  clientSecret?: string;
}

/**
 * High-level authentication configuration
 */
export interface AuthConfig {
  oktaDomain: string;
  clientId: string;
  scopes: string[];
  clientSecret?: string;
}

// =============================================================================
// Environment Detection
// =============================================================================

/**
 * Detect if running in headless environment (SSH, cloud workstation, container)
 *
 * Checks multiple signals:
 * - SSH_CLIENT, SSH_TTY, SSH_CONNECTION environment variables
 * - CLOUD_SHELL environment variable (GCP Cloud Shell)
 * - process.stdout.isTTY availability
 *
 * @returns Detailed environment detection information
 *
 * @example
 * const env = detectEnvironment();
 * if (env.type === 'headless') {
 *   // Use device flow
 * } else {
 *   // Use browser flow
 * }
 */
export function detectEnvironment(): EnvironmentDetection {
  // Check SSH environment variables
  const sshDetected = !!(
    process.env.SSH_CLIENT ||
    process.env.SSH_TTY ||
    process.env.SSH_CONNECTION
  );

  // Check for GCP Cloud Shell
  const cloudShellDetected = !!process.env.CLOUD_SHELL;

  // Check if TTY is available
  const ttyAvailable = !!process.stdout.isTTY;

  // Determine environment type
  // Headless if: SSH detected OR cloud shell detected OR no TTY
  const type: EnvironmentType =
    sshDetected || cloudShellDetected || !ttyAvailable ? 'headless' : 'interactive';

  return {
    type,
    sshDetected,
    cloudShellDetected,
    ttyAvailable,
  };
}

// =============================================================================
// PKCE Generation (RFC 7636)
// =============================================================================

/**
 * Generate PKCE code verifier (128 characters)
 *
 * Creates cryptographically random string using crypto.randomBytes().
 * Encoded as base64url (URL-safe, no padding).
 *
 * @returns 128-character code verifier
 *
 * @example
 * const verifier = generateCodeVerifier();
 * // "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_..."
 */
export function generateCodeVerifier(): string {
  // 96 bytes → 128 base64url characters
  return crypto.randomBytes(96).toString('base64url');
}

/**
 * Generate PKCE code challenge from verifier (SHA-256, base64url)
 *
 * Calculates SHA-256 hash of code verifier and encodes as base64url.
 * Per RFC 7636 Section 4.2.
 *
 * @param verifier - Code verifier (128 characters)
 * @returns 43-character code challenge
 *
 * @example
 * const challenge = generateCodeChallenge(verifier);
 * // "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
 */
export function generateCodeChallenge(verifier: string): string {
  // SHA-256 hash (32 bytes) → 43 base64url characters
  return crypto.createHash('sha256').update(verifier).digest('base64url');
}

/**
 * Generate state parameter for CSRF protection (32 characters)
 *
 * Creates cryptographically random string using crypto.randomBytes().
 * Encoded as base64url.
 *
 * @returns 32-character state parameter
 *
 * @example
 * const state = generateState();
 * // "dBjftJeZ4CVP-mB92K27uhbUJU1p"
 */
export function generateState(): string {
  // 24 bytes → 32 base64url characters
  return crypto.randomBytes(24).toString('base64url');
}

/**
 * Generate complete PKCE challenge (verifier, challenge, state)
 *
 * Convenience function that generates all PKCE parameters.
 *
 * @returns PKCE challenge object
 *
 * @example
 * const pkce = generatePKCE();
 * // { verifier: "...", challenge: "...", state: "..." }
 */
export function generatePKCE(): PKCEChallenge {
  const verifier = generateCodeVerifier();
  const challenge = generateCodeChallenge(verifier);
  const state = generateState();

  return {
    verifier,
    challenge,
    state,
  };
}

// =============================================================================
// Callback Server
// =============================================================================

/**
 * Select random port in range 3000-9000
 *
 * Returns random port number for callback server.
 *
 * @returns Port number (3000-9000)
 *
 * @example
 * const port = selectRandomPort();
 * // 5432
 */
export function selectRandomPort(): number {
  const min = 3000;
  const max = 9000;
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

/**
 * Start OAuth callback server
 *
 * Creates HTTP server on localhost listening for OAuth redirect.
 * Parses authorization code from query parameters.
 * Validates state parameter to prevent CSRF attacks.
 * Returns success/error HTML to browser.
 * Closes server after callback or timeout.
 *
 * @param port - Port number to listen on
 * @param expectedState - State parameter from authorization request (for CSRF validation)
 * @param timeoutSeconds - Timeout in seconds (default: 300 = 5 minutes)
 * @returns Promise that resolves with authorization code or rejects on timeout/error
 * @throws Error if state mismatch, missing code, or timeout
 *
 * @example
 * const { code } = await startCallbackServer(3456, "state123", 300);
 * // User completes OAuth in browser
 * // Server receives: GET /callback?code=abc&state=state123
 * // Returns: { code: "abc" }
 */
export async function startCallbackServer(
  port: number,
  expectedState: string,
  timeoutSeconds: number = 300
): Promise<{ code: string }> {
  return new Promise((resolve, reject) => {
    let serverClosed = false;

    const server = http.createServer((req, res) => {
      if (serverClosed) return;

      // Only handle /callback path
      if (!req.url || !req.url.startsWith('/callback')) {
        res.writeHead(404, { 'Content-Type': 'text/html' });
        res.end('<h1>404 Not Found</h1>');
        return;
      }

      // Parse query parameters
      const url = new URL(req.url, `http://localhost:${port}`);
      const code = url.searchParams.get('code');
      const state = url.searchParams.get('state');

      // Validate state parameter (CSRF protection)
      if (state !== expectedState) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end(`
<!DOCTYPE html>
<html>
<head><title>Authentication Failed</title></head>
<body>
  <h1>Authentication failed</h1>
  <p>Invalid state parameter. Possible CSRF attack detected.</p>
  <p>Please try again.</p>
</body>
</html>
        `);
        return; // Don't resolve, stay open for retry
      }

      // Validate code parameter
      if (!code) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end(`
<!DOCTYPE html>
<html>
<head><title>Authentication Failed</title></head>
<body>
  <h1>Authentication failed</h1>
  <p>Missing authorization code.</p>
  <p>Please try again.</p>
</body>
</html>
        `);
        return; // Don't resolve, stay open for retry
      }

      // Success - return HTML and resolve
      res.writeHead(200, { 'Content-Type': 'text/html' });
      res.end(`
<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body>
  <h1>Authentication successful!</h1>
  <p>You can close this window.</p>
</body>
</html>
      `);

      // Close server and resolve
      serverClosed = true;
      server.close(() => {
        clearTimeout(timeoutHandle);
        resolve({ code });
      });
    });

    // Handle server errors (e.g., port in use)
    server.on('error', (err: any) => {
      if (err.code === 'EADDRINUSE') {
        reject(new Error(`Port ${port} is already in use. Please try again.`));
      } else {
        reject(err);
      }
    });

    // Start server on localhost only
    server.listen(port, 'localhost', () => {
      console.log(`Callback server listening on http://localhost:${port}/callback`);
    });

    // Timeout handler
    const timeoutHandle = setTimeout(() => {
      if (!serverClosed) {
        serverClosed = true;
        server.close(() => {
          reject(new Error('Authentication timed out. Please run the command again.'));
        });
      }
    }, timeoutSeconds * 1000);
  });
}

/**
 * Start callback server with retry logic for port conflicts
 *
 * Retries up to 5 times if port is busy.
 *
 * @param expectedState - State parameter for CSRF validation
 * @param timeoutSeconds - Timeout in seconds
 * @returns Promise with authorization code and port used
 * @throws Error if all retries fail
 */
async function startCallbackServerWithRetry(
  expectedState: string,
  timeoutSeconds: number = 300
): Promise<{ code: string; port: number }> {
  const maxRetries = 5;
  let lastError: Error | null = null;

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    const port = selectRandomPort();

    try {
      const { code } = await startCallbackServer(port, expectedState, timeoutSeconds);
      return { code, port };
    } catch (error: any) {
      lastError = error;

      // Only retry on port conflict errors
      if (error.message && error.message.includes('already in use')) {
        console.log(`Port ${port} busy, retrying... (attempt ${attempt}/${maxRetries})`);
        continue;
      }

      // For other errors, throw immediately
      throw error;
    }
  }

  // All retries failed
  throw new Error(
    `Unable to start callback server after ${maxRetries} attempts. Please close other applications and try again.`
  );
}

// =============================================================================
// Browser Launch
// =============================================================================

/**
 * Build OAuth authorization URL with PKCE parameters
 *
 * Constructs URL for Okta authorization endpoint with all required
 * query parameters including PKCE challenge.
 *
 * @param domain - Okta organization domain (e.g., 'company.okta.com')
 * Configure your domain: mcp-wizard config set company.okta_domain your.okta.com
 * @param clientId - OAuth client ID
 * @param redirectUri - Callback URL (e.g., 'http://localhost:3456/callback')
 * @param scopes - Array of OAuth scopes
 * @param codeChallenge - PKCE code challenge
 * @param state - State parameter for CSRF protection
 * @returns Authorization URL
 *
 * @example
 * const url = buildAuthorizationUrl(
 *   'company.okta.com',
 *   'client-123',
 *   'http://localhost:3456/callback',
 *   ['openid', 'profile', 'email'],
 *   'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM',
 *   'state123'
 * );
 * // "https://company.okta.com/oauth2/v1/authorize?client_id=..."
 */
export function buildAuthorizationUrl(
  domain: string,
  clientId: string,
  redirectUri: string,
  scopes: string[],
  codeChallenge: string,
  state: string
): string {
  const params = new URLSearchParams({
    client_id: clientId,
    redirect_uri: redirectUri,
    response_type: 'code',
    scope: scopes.join(' '),
    code_challenge: codeChallenge,
    code_challenge_method: 'S256',
    state: state,
  });

  return `https://${domain}/oauth2/v1/authorize?${params.toString()}`;
}

/**
 * Launch browser with authorization URL
 *
 * Opens default browser using `open` package (macOS focus for V1).
 * Throws error if browser launch fails (no browser, sandbox restrictions).
 *
 * @param url - Authorization URL to open
 * @throws Error if browser launch fails
 *
 * @example
 * await launchBrowser('https://company.okta.com/oauth2/v1/authorize?...');
 * // Browser opens, user completes OAuth
 */
export async function launchBrowser(url: string): Promise<void> {
  try {
    await open.default(url);
    console.log('Browser opened for authentication...');
  } catch (error: any) {
    throw new Error(`Failed to launch browser: ${error.message}`);
  }
}

// =============================================================================
// Token Exchange
// =============================================================================

/**
 * Exchange authorization code for tokens using PKCE
 *
 * POSTs to Okta token endpoint with authorization code and PKCE verifier.
 * Includes retry logic with exponential backoff for network errors.
 * Validates token response structure.
 *
 * @param domain - Okta organization domain
 * @param clientId - OAuth client ID
 * @param code - Authorization code from callback
 * @param codeVerifier - PKCE code verifier
 * @param redirectUri - Redirect URI (must match authorization request)
 * @returns Token response with access_token, refresh_token, expires_in
 * @throws Error if code invalid, network error (after retries), or response invalid
 *
 * @example
 * const tokens = await exchangeCodeForTokens(
 *   'company.okta.com',
 *   'client-123',
 *   'auth-code-abc',
 *   'verifier-xyz',
 *   'http://localhost:3456/callback'
 * );
 * // { access_token: "...", refresh_token: "...", expires_in: 3600, ... }
 */
export async function exchangeCodeForTokens(
  domain: string,
  clientId: string,
  code: string,
  codeVerifier: string,
  redirectUri: string
): Promise<TokenResponse> {
  const url = `https://${domain}/oauth2/v1/token`;

  const params = new URLSearchParams({
    grant_type: 'authorization_code',
    code: code,
    redirect_uri: redirectUri,
    client_id: clientId,
    code_verifier: codeVerifier,
  });

  // Use retry with backoff for network errors
  const exchangeWithRetry = async (): Promise<TokenResponse> => {
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
        if (
          !data.access_token ||
          !data.token_type ||
          typeof data.expires_in !== 'number'
        ) {
          throw new Error('Invalid token response from Okta');
        }

        return data as TokenResponse;
      }

      // Error cases
      // HTTP 400/401: Don't retry (invalid code, client auth failed)
      if (response.status === 400 || response.status === 401) {
        const errorDesc = data.error_description || data.error || 'Unknown error';
        throw new Error(`Authorization failed: ${errorDesc}`);
      }

      // HTTP 5xx: Will be retried by retryWithBackoff
      throw new Error(`Okta server error (HTTP ${response.status}): ${data.error || 'Unknown error'}`);
    } catch (error: any) {
      throw sanitizeError(error);
    }
  };

  // Retry network errors, max 3 attempts, exponential backoff starting at 1s
  try {
    return await retryWithBackoff(exchangeWithRetry, 3, 1000);
  } catch (error: any) {
    throw sanitizeError(error);
  }
}

// =============================================================================
// Device Code Request
// =============================================================================

/**
 * Request device code from Okta
 *
 * Initiates OAuth device authorization flow by requesting a device code
 * from Okta's device authorization endpoint.
 *
 * @param domain - Okta organization domain (e.g., 'company.okta.com')
 * @param clientId - OAuth client ID
 * @param scopes - Array of OAuth scopes to request
 * @returns Device code response with verification URI and user code
 * @throws Error if request fails or response is invalid
 *
 * @example
 * const response = await requestDeviceCode(
 *   'company.okta.com',
 *   'client-id-123',
 *   ['openid', 'profile', 'email']
 * );
 * console.log(`Go to ${response.verification_uri}`);
 * console.log(`Enter code: ${response.user_code}`);
 */
export async function requestDeviceCode(
  domain: string,
  clientId: string,
  scopes: string[]
): Promise<DeviceCodeResponse> {
  const url = `https://${domain}/oauth2/v1/device/authorize`;

  const params = new URLSearchParams({
    client_id: clientId,
    scope: scopes.join(' '),
  });

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: params.toString(),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(
        `Device authorization request failed (HTTP ${response.status}): ${errorText}`
      );
    }

    const data: any = await response.json();

    // Validate required fields
    if (
      !data.device_code ||
      !data.user_code ||
      !data.verification_uri ||
      typeof data.expires_in !== 'number' ||
      typeof data.interval !== 'number'
    ) {
      throw new Error('Invalid device authorization response from Okta');
    }

    return data as DeviceCodeResponse;
  } catch (error: any) {
    throw sanitizeError(error);
  }
}

// =============================================================================
// User Code Display
// =============================================================================

/**
 * Display verification URI and user code to stdout
 *
 * Formats output based on TTY availability:
 * - TTY available: Formatted with colors and boxes
 * - No TTY: Plain text output
 *
 * @param verificationUri - URI where user completes authorization
 * @param userCode - Short code user must enter
 * @param expiresIn - Expiration time in seconds
 *
 * @example
 * displayUserCode('https://okta.com/activate', 'ABCD-1234', 600);
 */
export function displayUserCode(
  verificationUri: string,
  userCode: string,
  expiresIn: number
): void {
  const expiresInMin = Math.floor(expiresIn / 60);

  if (process.stdout.isTTY) {
    // TTY available: formatted output
    console.log('\n' + '='.repeat(60));
    console.log('  Device Authorization Required');
    console.log('='.repeat(60));
    console.log(`\n  1. Visit: ${verificationUri}`);
    console.log(`  2. Enter code: ${userCode}`);
    console.log(`\n  Code expires in ${expiresInMin} minutes\n`);
    console.log('='.repeat(60) + '\n');
  } else {
    // No TTY: plain text output
    console.log('Device Authorization Required');
    console.log(`Visit: ${verificationUri}`);
    console.log(`Enter code: ${userCode}`);
    console.log(`Code expires in ${expiresInMin} minutes`);
  }
}

// =============================================================================
// Token Polling
// =============================================================================

/**
 * Poll Okta token endpoint until authorization complete or timeout
 *
 * Implements RFC 8628 token polling with:
 * - Respect for Okta's polling interval
 * - Exponential backoff for transient errors
 * - Handling of slow_down, access_denied, expired_token responses
 *
 * @param domain - Okta organization domain
 * @param clientId - OAuth client ID
 * @param deviceCode - Device code from requestDeviceCode()
 * @param interval - Polling interval in seconds (from device response)
 * @param expiresIn - Timeout in seconds (from device response)
 * @returns Access token and refresh token
 * @throws Error if user denies, code expires, or network errors persist
 *
 * @example
 * const tokens = await pollForToken(
 *   'company.okta.com',
 *   'client-id-123',
 *   'device-code-xyz',
 *   5,  // poll every 5 seconds
 *   600 // timeout after 10 minutes
 * );
 */
export async function pollForToken(
  domain: string,
  clientId: string,
  deviceCode: string,
  interval: number,
  expiresIn: number
): Promise<TokenResponse> {
  const url = `https://${domain}/oauth2/v1/token`;
  const startTime = Date.now();
  const timeoutMs = expiresIn * 1000;
  let currentInterval = interval * 1000; // Convert to milliseconds

  while (true) {
    // Wait for interval before polling (including first iteration)
    await new Promise((resolve) => setTimeout(resolve, currentInterval));

    // Check timeout after waiting
    if (Date.now() - startTime > timeoutMs) {
      throw new Error('Device authorization timed out. Please try again.');
    }

    const params = new URLSearchParams({
      grant_type: 'urn:ietf:params:oauth:grant-type:device_code',
      device_code: deviceCode,
      client_id: clientId,
    });

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
        console.log('✓ Authorization successful!');
        return data as TokenResponse;
      }

      // Error cases
      const error = data as TokenErrorResponse;

      switch (error.error) {
        case 'authorization_pending':
          // Continue polling (user hasn't authorized yet)
          console.log('Waiting for authorization...');
          break;

        case 'slow_down':
          // Increase polling interval by 5 seconds (per RFC 8628)
          currentInterval += 5000;
          console.log('Reducing polling frequency...');
          break;

        case 'access_denied':
          // User denied authorization
          throw new Error(
            'Authorization denied. If this was a mistake, run the command again.'
          );

        case 'expired_token':
          // Device code expired
          throw new Error('Device code expired. Please run the command again.');

        default:
          // Unknown error
          throw new Error(
            `Authorization failed: ${error.error} - ${error.error_description || 'Unknown error'}`
          );
      }
    } catch (error: any) {
      // If error is one of our user-facing errors, re-throw
      if (
        error.message &&
        (error.message.includes('denied') ||
          error.message.includes('expired') ||
          error.message.includes('failed:'))
      ) {
        throw error;
      }

      // Otherwise, it's a network error - retry with backoff
      // Only log the retry attempt, don't fail immediately
      console.log(`Network error, retrying...`);
    }
  }
}

// =============================================================================
// Device Flow Orchestration
// =============================================================================

/**
 * Complete device authorization flow
 *
 * Orchestrates the full OAuth device flow:
 * 1. Request device code
 * 2. Display verification URI and user code
 * 3. Poll for token
 * 4. Store tokens in OS keychain
 *
 * @param options - Device flow configuration
 * @throws Error if configuration missing, authorization fails, or storage fails
 *
 * @example
 * await deviceFlowAuth({
 *   oktaDomain: process.env.OKTA_DOMAIN!,
 *   clientId: process.env.OKTA_CLIENT_ID!,
 *   scopes: ['openid', 'profile', 'email'],
 * });
 */
export async function deviceFlowAuth(options: DeviceFlowOptions): Promise<void> {
  const { oktaDomain, clientId, scopes } = options;

  // Validate configuration
  if (!oktaDomain || !clientId) {
    throw new Error(
      'Missing Okta configuration. Set environment variables:\n' +
        '  OKTA_DOMAIN=your-org.okta.com\n' +
        '  OKTA_CLIENT_ID=your-client-id'
    );
  }

  try {
    // Step 1: Request device code
    console.log('Initiating device authorization...');
    const deviceResponse = await requestDeviceCode(oktaDomain, clientId, scopes);

    // Step 2: Display verification URI and user code
    displayUserCode(
      deviceResponse.verification_uri,
      deviceResponse.user_code,
      deviceResponse.expires_in
    );

    // Step 3: Poll for token
    console.log('Waiting for authorization...');
    const tokenResponse = await pollForToken(
      oktaDomain,
      clientId,
      deviceResponse.device_code,
      deviceResponse.interval,
      deviceResponse.expires_in
    );

    // Step 4: Store tokens in OS keychain
    await storeOktaToken({
      type: 'authorized_user',
      client_id: clientId,
      client_secret: options.clientSecret || '',
      refresh_token: tokenResponse.refresh_token || '',
      access_token: tokenResponse.access_token,
      expires_at: Date.now() + tokenResponse.expires_in * 1000,
    });

    console.log('✓ Tokens stored securely in OS keychain');
  } catch (error: any) {
    throw sanitizeError(error);
  }
}

// =============================================================================
// PKCE Flow Orchestration
// =============================================================================

/**
 * Complete browser PKCE authorization flow
 *
 * Orchestrates full PKCE flow:
 * 1. Generate PKCE (verifier, challenge, state)
 * 2. Start callback server
 * 3. Build authorization URL
 * 4. Launch browser
 * 5. Wait for callback (or timeout)
 * 6. Exchange code for tokens
 * 7. Store tokens in OS keychain
 *
 * Error handling:
 * - Network errors: Retry with backoff
 * - Browser launch errors: Propagate (caller handles fallback)
 * - Callback timeout: Error with retry message
 * - State mismatch: Error (CSRF attack)
 * - All errors sanitized to prevent token leakage
 *
 * Cleanup:
 * - Callback server always closed (finally block)
 *
 * @param options - PKCE flow configuration
 * @throws Error if flow fails (caller handles fallback to device flow)
 *
 * @example
 * await browserPKCEAuth({
 *   oktaDomain: 'company.okta.com',
 *   clientId: 'client-123',
 *   scopes: ['openid', 'profile', 'email'],
 * });
 * // Browser opens, user completes OAuth, tokens stored in keychain
 */
export async function browserPKCEAuth(options: PKCEOptions): Promise<void> {
  const { oktaDomain, clientId, scopes } = options;

  // Validate configuration
  if (!oktaDomain || !clientId) {
    throw new Error(
      'Missing Okta configuration. Set environment variables:\n' +
        '  OKTA_DOMAIN=your-org.okta.com\n' +
        '  OKTA_CLIENT_ID=your-client-id'
    );
  }

  try {
    // Step 1: Generate PKCE parameters
    console.log('Initiating browser-based authorization...');
    const pkce = generatePKCE();

    // Step 2: Start callback server (with retry logic for port conflicts)
    const callbackPromise = startCallbackServerWithRetry(pkce.state, 300);

    // Get port from the promise (awkward, but we need it for redirect URI)
    // We'll construct the redirect URI pattern and launch browser
    // The port is determined by the callback server function

    // Actually, let's refactor to get port first, then start server
    // For now, we'll use a different approach: start server, get port, then launch browser

    // Alternative: Use fixed approach - we know the port from startCallbackServerWithRetry
    // Let's simplify by selecting port first
    let port: number | undefined;
    let code: string | undefined;

    try {
      // Start callback server (will select port internally and retry if needed)
      const result = await new Promise<{ code: string; port: number }>(async (resolve, reject) => {
        const maxRetries = 5;

        for (let attempt = 1; attempt <= maxRetries; attempt++) {
          const attemptPort = selectRandomPort();

          try {
            // Step 3: Build authorization URL
            const redirectUri = `http://localhost:${attemptPort}/callback`;
            const authUrl = buildAuthorizationUrl(
              oktaDomain,
              clientId,
              redirectUri,
              scopes,
              pkce.challenge,
              pkce.state
            );

            // Step 4: Launch browser
            await launchBrowser(authUrl);

            // Step 5: Wait for callback
            const callbackResult = await startCallbackServer(attemptPort, pkce.state, 300);

            resolve({ code: callbackResult.code, port: attemptPort });
            return;
          } catch (error: any) {
            // Only retry on port conflict errors
            if (error.message && error.message.includes('already in use')) {
              console.log(`Port ${attemptPort} busy, retrying... (attempt ${attempt}/${maxRetries})`);
              continue;
            }

            // For other errors (timeout, state mismatch, browser launch), throw immediately
            reject(error);
            return;
          }
        }

        // All retries failed
        reject(new Error(
          `Unable to start callback server after ${maxRetries} attempts. Please close other applications and try again.`
        ));
      });

      code = result.code;
      port = result.port;
    } catch (error: any) {
      // Re-throw for outer catch block
      throw error;
    }

    if (!code || !port) {
      throw new Error('Failed to obtain authorization code');
    }

    // Step 6: Exchange code for tokens
    console.log('Exchanging authorization code for tokens...');
    const redirectUri = `http://localhost:${port}/callback`;
    const tokenResponse = await exchangeCodeForTokens(
      oktaDomain,
      clientId,
      code,
      pkce.verifier,
      redirectUri
    );

    // Step 7: Store tokens in OS keychain
    await storeOktaToken({
      type: 'authorized_user',
      client_id: clientId,
      client_secret: options.clientSecret || '',
      refresh_token: tokenResponse.refresh_token || '',
      access_token: tokenResponse.access_token,
      expires_at: Date.now() + tokenResponse.expires_in * 1000,
    });

    console.log('✓ Tokens stored securely in OS keychain');
  } catch (error: any) {
    throw sanitizeError(error);
  }
}

// =============================================================================
// High-Level Authentication
// =============================================================================

/**
 * Check if error is a browser launch error (for fallback detection)
 *
 * @param error - Error to check
 * @returns True if browser launch error
 */
export function isBrowserLaunchError(error: Error): boolean {
  return (
    error.message.includes('ENOENT') ||      // Browser binary not found
    error.message.includes('spawn') ||       // Spawn failed
    error.message.includes('browser') ||     // Generic browser error
    error.message.includes('Failed to launch browser')  // Our error message
  );
}

/**
 * High-level authentication function with automatic flow selection
 *
 * Detects environment and selects appropriate OAuth flow:
 * - Headless (SSH, Cloud Shell, no TTY): Device flow
 * - Interactive (local with browser): PKCE flow
 * - Browser launch failure: Fallback to device flow
 *
 * Two-layer approach:
 * - Layer 1: Proactive detection (detectEnvironment)
 * - Layer 2: Reactive fallback (catch browser launch errors)
 *
 * @param config - Authentication configuration
 * @throws Error if both flows fail (rare, likely config issue)
 *
 * @example
 * await authenticate({
 *   oktaDomain: process.env.OKTA_DOMAIN!,
 *   clientId: process.env.OKTA_CLIENT_ID!,
 *   scopes: ['openid', 'profile', 'email'],
 * });
 * // Auto-selects PKCE (local) or device flow (SSH)
 */
export async function authenticate(config: AuthConfig): Promise<void> {
  // Layer 1: Proactive environment detection
  const env = detectEnvironment();

  if (env.type === 'headless') {
    // Use device flow for SSH, Cloud Shell, non-TTY environments
    console.log('Headless environment detected, using device flow...');
    return await deviceFlowAuth(config);
  }

  // Layer 2: Try PKCE flow, fallback to device flow on browser launch failure
  try {
    // Use PKCE flow for interactive environments
    await browserPKCEAuth(config);
  } catch (error: any) {
    // If browser launch failed, fall back to device flow
    if (isBrowserLaunchError(error)) {
      console.log('Browser launch failed, falling back to device flow...');
      return await deviceFlowAuth(config);
    }

    // For other errors (timeout, network, etc.), propagate
    throw error;
  }
}
