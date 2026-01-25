/**
 * Error classification for retry logic
 *
 * Classifies errors as transient (retry) or permanent (fail fast)
 * to enable intelligent retry behavior.
 *
 * @module observability/errors
 */

/**
 * Error classification types
 */
export type ErrorType = 'transient' | 'permanent';

/**
 * Classify error as transient or permanent
 *
 * Transient errors (should retry):
 * - Keychain service unavailable (libsecret not loaded)
 * - Connection refused (service restarting)
 * - Timeout errors (temporary network issues)
 *
 * Permanent errors (fail fast):
 * - Token not found (must re-authenticate)
 * - Invalid token structure (corrupted data)
 * - Corrupted token file (unrecoverable)
 *
 * @param error - Error object to classify
 * @returns 'transient' (retry) or 'permanent' (fail fast)
 *
 * @example
 * const errorType = classifyError(new Error('libsecret unavailable'));
 * // Returns: 'transient' (should retry)
 */
export function classifyError(error: any): ErrorType {
  // Transient errors (retry these)
  if (error.message?.includes('libsecret')) {
    return 'transient'; // Keychain service not loaded
  }
  if (error.code === 'ECONNREFUSED') {
    return 'transient'; // Connection refused (service restarting)
  }
  if (error.code === 'ETIMEDOUT') {
    return 'transient'; // Timeout (temporary network issue)
  }

  // Permanent errors (fail fast, no retry)
  if (error.message?.includes('No token found')) {
    return 'permanent'; // Token doesn't exist
  }
  if (error.message?.includes('Invalid token structure')) {
    return 'permanent'; // Corrupted token data
  }
  if (error.message?.includes('Corrupted token file')) {
    return 'permanent'; // Unrecoverable file corruption
  }

  // Default to transient (safe for retry, prevents permanent failures on unknown errors)
  return 'transient';
}
