/**
 * Trace Sanitizer - Redact sensitive data from trace logs
 *
 * Provides utilities to sanitize tokens, passwords, and other credentials
 * before logging. Uses blocklist approach with comprehensive patterns.
 *
 * @module trace-sanitizer
 */

/**
 * Sanitization pattern for regex-based redaction
 */
interface SanitizationPattern {
  pattern: RegExp;
  replacement: string;
}

/**
 * Sensitive data patterns to redact from logs
 *
 * Covers: OAuth tokens, passwords, client secrets, Bearer tokens,
 * JWT tokens, URL parameters, environment variables
 */
const SENSITIVE_PATTERNS: SanitizationPattern[] = [
  // OAuth tokens (JSON format)
  {
    pattern: /"access_token"\s*:\s*"[^"]+"/gi,
    replacement: '"access_token": "[REDACTED]"',
  },
  {
    pattern: /"refresh_token"\s*:\s*"[^"]+"/gi,
    replacement: '"refresh_token": "[REDACTED]"',
  },
  // Passwords (JSON format)
  {
    pattern: /"password"\s*:\s*"[^"]+"/gi,
    replacement: '"password": "[REDACTED]"',
  },
  // Client secrets (JSON format)
  {
    pattern: /"client_secret"\s*:\s*"[^"]+"/gi,
    replacement: '"client_secret": "[REDACTED]"',
  },
  // Bearer tokens (HTTP header format)
  {
    pattern: /Bearer\s+[^\s]+/gi,
    replacement: 'Bearer [REDACTED]',
  },
  // JWT tokens (eyJ... format)
  {
    pattern: /eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+/g,
    replacement: '[JWT_REDACTED]',
  },
  // URL query parameters
  {
    pattern: /([?&](?:token|access_token|refresh_token|code)=)[^&\s]+/gi,
    replacement: '$1[REDACTED]',
  },
  // Environment variables
  {
    pattern: /(OKTA_TOKEN|ACCESS_TOKEN|BEARER_TOKEN|REFRESH_TOKEN)=[^\s]+/gi,
    replacement: '$1=[REDACTED]',
  },
];

/**
 * Sanitize string by applying all sensitive patterns
 *
 * @param str - String to sanitize
 * @returns Sanitized string with tokens/credentials redacted
 *
 * @example
 * ```typescript
 * const input = 'Authorization: Bearer ya29.secret';
 * const output = sanitizeString(input);
 * // output: 'Authorization: Bearer [REDACTED]'
 * ```
 */
export function sanitizeString(str: string): string {
  let sanitized = str;
  for (const { pattern, replacement } of SENSITIVE_PATTERNS) {
    sanitized = sanitized.replace(pattern, replacement);
  }
  return sanitized;
}

/**
 * Recursively sanitize any value (string, object, array)
 *
 * Handles nested objects and arrays to ensure comprehensive sanitization
 * throughout trace log data structures.
 *
 * @param value - Value to sanitize (any type)
 * @returns Sanitized value with same structure
 *
 * @example
 * ```typescript
 * const input = {
 *   headers: { Authorization: 'Bearer ya29.secret' },
 *   nested: {
 *     access_token: 'secret123',
 *     safe: 'value',
 *   },
 * };
 * const output = sanitizeValue(input);
 * // output.headers.Authorization: 'Bearer [REDACTED]'
 * // output.nested.access_token: '[REDACTED]'
 * // output.nested.safe: 'value'
 * ```
 */
export function sanitizeValue(value: any): any {
  if (typeof value === 'string') {
    return sanitizeString(value);
  } else if (Array.isArray(value)) {
    return value.map(sanitizeValue);
  } else if (value && typeof value === 'object') {
    const sanitized: any = {};
    for (const key in value) {
      if (Object.prototype.hasOwnProperty.call(value, key)) {
        sanitized[key] = sanitizeValue(value[key]);
      }
    }
    return sanitized;
  }
  return value;
}
