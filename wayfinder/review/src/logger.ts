/**
 * Centralized logging utility
 * Provides structured logging with levels and consistent formatting
 */

export enum LogLevel {
  DEBUG = 0,
  INFO = 1,
  WARN = 2,
  ERROR = 3,
  NONE = 4,
}

export interface LoggerConfig {
  level: LogLevel;
  prefix?: string;
  timestamp?: boolean;
}

/**
 * Simple structured logger
 */
export class Logger {
  private config: LoggerConfig;

  constructor(config: Partial<LoggerConfig> = {}) {
    this.config = {
      level: config.level ?? LogLevel.INFO,
      prefix: config.prefix ?? '',
      timestamp: config.timestamp ?? false,
    };
  }

  /**
   * Set the log level
   */
  setLevel(level: LogLevel): void {
    this.config.level = level;
  }

  /**
   * Format a log message with optional timestamp and prefix
   */
  private format(level: string, message: string): string {
    const parts: string[] = [];

    if (this.config.timestamp) {
      parts.push(`[${new Date().toISOString()}]`);
    }

    parts.push(`[${level}]`);

    if (this.config.prefix) {
      parts.push(`[${this.config.prefix}]`);
    }

    parts.push(message);

    return parts.join(' ');
  }

  /**
   * Log debug message (lowest priority)
   */
  debug(message: string, ...args: any[]): void {
    if (this.config.level <= LogLevel.DEBUG) {
      console.error(this.format('DEBUG', message), ...args);
    }
  }

  /**
   * Log info message
   */
  info(message: string, ...args: any[]): void {
    if (this.config.level <= LogLevel.INFO) {
      console.error(this.format('INFO', message), ...args);
    }
  }

  /**
   * Log warning message
   */
  warn(message: string, ...args: any[]): void {
    if (this.config.level <= LogLevel.WARN) {
      console.error(this.format('WARN', message), ...args);
    }
  }

  /**
   * Log error message (highest priority)
   */
  error(message: string, ...args: any[]): void {
    if (this.config.level <= LogLevel.ERROR) {
      console.error(this.format('ERROR', message), ...args);
    }
  }
}

/**
 * Default logger instance
 */
export const defaultLogger = new Logger();

/**
 * Create a logger with a specific prefix
 */
export function createLogger(prefix: string, level?: LogLevel): Logger {
  return new Logger({ prefix, level });
}
