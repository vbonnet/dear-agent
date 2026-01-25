/**
 * Structured logging with Pino
 *
 * Provides JSON-formatted logs with built-in secret redaction
 * for production observability.
 *
 * @module observability/logger
 */

import pino from 'pino';

/**
 * Create a configured Pino logger instance
 *
 * Features:
 * - JSON structured logging
 * - Secret redaction (tokens, client secrets, refresh tokens)
 * - Configurable log level via LOG_LEVEL environment variable
 * - ISO8601 timestamps
 *
 * @returns Configured Pino logger
 *
 * @example
 * const logger = createLogger();
 * logger.info({ operation: 'getToken', duration_ms: 45 });
 */
export function createLogger(): pino.Logger {
  return pino({
    level: process.env.LOG_LEVEL || 'info',
    redact: {
      paths: ['token', 'client_secret', 'refresh_token', 'access_token'],
      remove: true, // Completely remove sensitive fields (not replace with [Redacted])
    },
    formatters: {
      level: (label) => ({ level: label }),
    },
    timestamp: pino.stdTimeFunctions.isoTime,
  });
}
