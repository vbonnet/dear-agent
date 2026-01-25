/**
 * Observability wrapper for operations
 *
 * Provides centralized logging, timing, and retry logic
 * using the wrapper pattern.
 *
 * @module observability/wrapper
 */

import { backOff } from 'exponential-backoff';
import { createLogger } from './logger';
import { classifyError } from './errors';

/**
 * Options for observability wrapper
 */
export interface ObservabilityOptions {
  /**
   * Enable retry with exponential backoff
   * Default: false
   */
  retry?: boolean;

  /**
   * Function to sanitize result for logging
   * Use to remove sensitive data from log output
   */
  sanitize?: (result: any) => any;
}

/**
 * Wrap an async operation with observability
 *
 * Provides:
 * - Structured logging (success/failure)
 * - Timing metrics (duration_ms)
 * - Retry logic with exponential backoff (optional)
 * - Secret sanitization (optional)
 *
 * @param operation - Operation name for logging
 * @param fn - Async function to execute
 * @param options - Observability options (retry, sanitize)
 * @returns Result of fn()
 * @throws Original error after logging
 *
 * @example
 * // Simple operation (no retry)
 * await withObservability('deleteToken', async () => {
 *   await keytar.deletePassword('service', 'account');
 * });
 *
 * @example
 * // With retry and sanitization
 * const token = await withObservability('getToken', async () => {
 *   return await keytar.getPassword('service', 'account');
 * }, {
 *   retry: true,
 *   sanitize: (token) => ({ has_token: !!token })
 * });
 */
export async function withObservability<T>(
  operation: string,
  fn: () => Promise<T>,
  options?: ObservabilityOptions
): Promise<T> {
  const startTime = performance.now();
  const logger = createLogger();

  try {
    // Execute with or without retry
    const execute = options?.retry
      ? () =>
          backOff(fn, {
            numOfAttempts: 3,
            startingDelay: 100, // 100ms initial delay
            timeMultiple: 2, // Exponential backoff (2x each attempt)
            retry: (error) => classifyError(error) === 'transient',
          })
      : fn;

    const result = await execute();
    const duration = Math.round(performance.now() - startTime);

    // Log success
    const logData: any = {
      operation,
      status: 'success',
      duration_ms: duration,
    };

    if (options?.sanitize) {
      logData.result = options.sanitize(result);
    }

    logger.info(logData);
    return result;
  } catch (error: any) {
    // Log failure
    const duration = Math.round(performance.now() - startTime);
    logger.error({
      operation,
      status: 'failure',
      duration_ms: duration,
      error_type: classifyError(error),
      error_message: error.message,
      retry_attempted: options?.retry || false,
    });

    // Re-throw original error (preserve error propagation)
    throw error;
  }
}
