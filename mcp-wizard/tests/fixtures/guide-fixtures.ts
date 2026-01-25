/**
 * OAuth credential fixtures for guide testing
 * Provides realistic mock credentials for GCP, Atlassian, and Slack setup guides
 */

/**
 * Mock GCP OAuth credentials (client application credentials)
 * Used for testing GCP setup guide OAuth flow
 */
export const MOCK_GCP_CREDENTIALS = {
  installed: {
    client_id: '123456789-abcdefghijklmnop.apps.googleusercontent.com',
    client_secret: 'GOCSPX-mock_client_secret_1234567890',
    redirect_uris: ['http://localhost:8080/', 'urn:ietf:wg:oauth:2.0:oob'],
    auth_uri: 'https://accounts.google.com/o/oauth2/auth',
    token_uri: 'https://oauth2.googleapis.com/token',
    auth_provider_x509_cert_url: 'https://www.googleapis.com/oauth2/v1/certs',
    project_id: 'mock-project-123456'
  }
};

/**
 * Mock Atlassian device code response
 * Used for testing Atlassian setup guide OAuth device flow
 */
export const MOCK_ATLASSIAN_DEVICE_CODE = {
  device_code: 'mock_device_code_abcdef1234567890',
  user_code: 'ABCD-1234',
  verification_uri: 'https://auth.atlassian.com/activate',
  verification_uri_complete: 'https://auth.atlassian.com/activate?user_code=ABCD-1234',
  expires_in: 600,  // 10 minutes
  interval: 5       // Poll every 5 seconds
};

/**
 * Mock Atlassian token response (successful authorization)
 * Used for testing token exchange after device authorization
 */
export const MOCK_ATLASSIAN_TOKEN = {
  access_token: 'mock_atlassian_access_token_xyz789',
  refresh_token: 'mock_atlassian_refresh_token_abc123',
  token_type: 'Bearer',
  expires_in: 3600,  // 1 hour
  scope: 'read:jira-work write:jira-work read:confluence-content.all'
};

/**
 * Mock GCP token response (successful authorization)
 * Used for testing GCP OAuth token exchange
 */
export const MOCK_GCP_TOKEN = {
  access_token: 'mock_gcp_access_token_xyz789',
  refresh_token: 'mock_gcp_refresh_token_abc123',
  token_type: 'Bearer',
  expires_in: 3599,
  scope: 'https://www.googleapis.com/auth/drive.readonly https://www.googleapis.com/auth/gmail.readonly',
  expiry_date: Date.now() + 3599000  // 1 hour from now
};

/**
 * Mock Slack tokens for different scenarios
 * Used for testing Slack setup guide token validation
 */
export const MOCK_SLACK_TOKEN = {
  /** Valid Slack bot token (xoxb- prefix) */
  valid: 'xoxb-1234567890-1234567890123-abcdefghijklmnopqrstuvwx',

  /** Invalid Slack token (wrong prefix) */
  invalid: 'invalid_token_format_xyz',

  /** Expired Slack token (valid format but expired) */
  expired: 'xoxb-0000000000-0000000000000-expiredtokenabcdefghijk'
};

/**
 * Mock OAuth error responses
 * Used for testing error handling in OAuth flows
 */
export const MOCK_OAUTH_ERRORS = {
  /** Authorization pending (user hasn't authorized yet) */
  authorization_pending: {
    error: 'authorization_pending',
    error_description: 'The authorization request is still pending as the end user has not yet completed the user interaction steps.'
  },

  /** Slow down (polling too fast) */
  slow_down: {
    error: 'slow_down',
    error_description: 'You are polling too frequently. Slow down the interval.'
  },

  /** Access denied (user rejected authorization) */
  access_denied: {
    error: 'access_denied',
    error_description: 'The resource owner or authorization server denied the request.'
  },

  /** Expired token */
  expired_token: {
    error: 'expired_token',
    error_description: 'The device code has expired. Please restart the authorization process.'
  },

  /** Invalid grant */
  invalid_grant: {
    error: 'invalid_grant',
    error_description: 'The provided authorization grant is invalid, expired, revoked, or does not match the authorization request.'
  }
};
