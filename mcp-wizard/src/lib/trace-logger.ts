/**
 * Trace Logger - Structured logging for debug mode
 *
 * Provides JSON Lines format logging with correlation IDs, performance timing,
 * and automatic sanitization of sensitive data.
 *
 * @module trace-logger
 */

import { AsyncLocalStorage } from 'async_hooks';
import { createWriteStream, WriteStream } from 'fs';
import { randomUUID } from 'crypto';
import { sanitizeValue } from './trace-sanitizer';

/**
 * Log level enumeration
 */
export enum LogLevel {
  DEBUG = 'DEBUG',
  INFO = 'INFO',
  WARN = 'WARN',
  ERROR = 'ERROR',
}

/**
 * Trace logger configuration
 */
export interface TraceConfig {
  /** Enable trace logging (default: false) */
  enabled: boolean;
  /** Optional log file path for persistent logging */
  logFile?: string;
}

/**
 * Trace log entry structure
 */
export interface TraceLogEntry {
  /** ISO 8601 timestamp */
  timestamp: string;
  /** Log level */
  level: LogLevel;
  /** Operation identifier (e.g., 'oauth_refresh_start') */
  operation: string;
  /** UUID v4 correlation ID linking related operations */
  correlation_id: string;
  /** Operation-specific fields */
  [key: string]: any;
}

/**
 * Singleton trace logger with correlation ID support
 *
 * Features:
 * - JSON Lines output format
 * - Correlation IDs via AsyncLocalStorage
 * - Performance timing utilities
 * - Automatic sanitization
 * - Stderr + optional file output
 *
 * @example
 * ```typescript
 * const tracer = TraceLogger.getInstance();
 * tracer.configure({ enabled: true, logFile: 'debug.log' });
 *
 * tracer.withCorrelationId(async () => {
 *   tracer.log(LogLevel.DEBUG, 'operation_start', { foo: 'bar' });
 *   await someAsyncWork();
 *   tracer.log(LogLevel.DEBUG, 'operation_end', { result: 'success' });
 * });
 * ```
 */
export class TraceLogger {
  private static instance: TraceLogger;
  private config: TraceConfig = { enabled: false };
  private correlationStorage = new AsyncLocalStorage<string>();
  private logStream?: WriteStream;

  /**
   * Private constructor for singleton pattern
   */
  private constructor() {}

  /**
   * Get singleton instance
   *
   * @returns TraceLogger instance
   */
  static getInstance(): TraceLogger {
    if (!TraceLogger.instance) {
      TraceLogger.instance = new TraceLogger();
    }
    return TraceLogger.instance;
  }

  /**
   * Configure trace logger
   *
   * Call once at CLI startup to enable tracing and optional file logging.
   *
   * @param config - Trace configuration
   *
   * @example
   * ```typescript
   * tracer.configure({ enabled: true, logFile: '/tmp/debug.log' });
   * ```
   */
  configure(config: TraceConfig): void {
    this.config = config;

    // Open log file if specified
    if (config.logFile && config.enabled) {
      try {
        this.logStream = createWriteStream(config.logFile, {
          mode: 0o600, // User read/write only (rw-------)
          flags: 'a',  // Append mode (don't truncate)
        });

        // Handle file I/O errors gracefully
        this.logStream.on('error', (err) => {
          process.stderr.write(
            `Warning: Trace log file error: ${err.message}\n`
          );
          process.stderr.write('Trace logs will only be written to stderr.\n');
          this.logStream = undefined; // Fallback to stderr only
        });
      } catch (err: any) {
        process.stderr.write(
          `Warning: Failed to open trace log file: ${err.message}\n`
        );
        process.stderr.write('Trace logs will only be written to stderr.\n');
      }
    }
  }

  /**
   * Wrap operation with correlation ID
   *
   * Generates new UUID v4 correlation ID and propagates it through
   * all async operations via AsyncLocalStorage. All log entries within
   * the wrapped function share the same correlation ID.
   *
   * @param fn - Function to execute with correlation ID context
   * @returns Result of function execution
   *
   * @example
   * ```typescript
   * await tracer.withCorrelationId(async () => {
   *   tracer.log('DEBUG', 'step1', {});
   *   await asyncOperation();
   *   tracer.log('DEBUG', 'step2', {});
   *   // Both logs have same correlation_id
   * });
   * ```
   */
  withCorrelationId<T>(fn: () => T | Promise<T>): T | Promise<T> {
    if (!this.config.enabled) {
      return fn(); // Skip correlation if tracing disabled
    }
    const correlationId = randomUUID();
    return this.correlationStorage.run(correlationId, fn);
  }

  /**
   * Get current correlation ID from async context
   *
   * @returns Correlation ID or undefined if not in context
   */
  getCorrelationId(): string | undefined {
    return this.correlationStorage.getStore();
  }

  /**
   * Log trace event
   *
   * Writes JSON Lines format log entry to stderr and optionally to file.
   * Automatically includes correlation ID and sanitizes sensitive data.
   *
   * @param level - Log level
   * @param operation - Operation identifier (e.g., 'oauth_refresh_start')
   * @param data - Operation-specific data (will be sanitized)
   *
   * @example
   * ```typescript
   * tracer.log(LogLevel.DEBUG, 'oauth_refresh_start', {
   *   service: 'googledocs',
   *   okta_domain: 'company.okta.com',
   * });
   * ```
   */
  log(level: LogLevel, operation: string, data: object = {}): void {
    if (!this.config.enabled) return;

    // Build log entry
    const entry: TraceLogEntry = {
      timestamp: new Date().toISOString(),
      level,
      operation,
      correlation_id: this.getCorrelationId() || randomUUID(),
      ...sanitizeValue(data),
    };

    const jsonLine = JSON.stringify(entry) + '\n';

    // Always write to stderr
    process.stderr.write(jsonLine);

    // Optionally write to file
    if (this.logStream) {
      this.logStream.write(jsonLine);
    }
  }

  /**
   * Start timing an operation
   *
   * Returns end function that logs duration when called.
   * Useful for measuring operation timing.
   *
   * @param operation - Operation identifier
   * @returns End function that logs duration_ms
   *
   * @example
   * ```typescript
   * const endTimer = tracer.time('oauth_refresh');
   * await refreshToken();
   * endTimer(); // Logs { operation: 'oauth_refresh_end', duration_ms: 234 }
   * ```
   */
  time(operation: string): () => void {
    const start = Date.now();
    return () => {
      const duration_ms = Date.now() - start;
      this.log(LogLevel.DEBUG, `${operation}_end`, { duration_ms });
    };
  }

  /**
   * Sanitize data (for testing/manual use)
   *
   * @param value - Value to sanitize
   * @returns Sanitized value
   */
  sanitize(value: any): any {
    return sanitizeValue(value);
  }

  /**
   * Close log file stream (cleanup)
   *
   * Called automatically on process exit, but can be called manually
   * for cleanup in tests or long-running processes.
   */
  close(): void {
    if (this.logStream) {
      this.logStream.end();
      this.logStream = undefined;
    }
  }
}
