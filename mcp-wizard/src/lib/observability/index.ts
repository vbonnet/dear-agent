/**
 * Observability infrastructure
 *
 * Exports logging, error classification, sanitization, and wrapper utilities.
 *
 * @module observability
 */

export { createLogger } from './logger';
export { classifyError, type ErrorType } from './errors';
export { sanitizeToken, type SanitizedToken } from './sanitizer';
export { withObservability, type ObservabilityOptions } from './wrapper';
