/**
 * Health Check Result Cache
 *
 * Implements 5-minute in-memory cache for health check results to avoid
 * excessive checking when running health/doctor commands repeatedly.
 *
 * Part of Phase 4-v2 Health and Doctor Commands (oss-n1nq.4-v2)
 *
 * @module health-cache
 */

import { HealthCheckResult } from './health-checks';

/**
 * Cache entry with result and timestamp
 */
interface CacheEntry {
  result: HealthCheckResult;
  timestamp: number;
}

/**
 * In-memory cache storage
 */
const cache = new Map<string, CacheEntry>();

/**
 * Cache TTL in milliseconds (5 minutes)
 */
const TTL = 300000;

/**
 * Get cached health check result if not expired
 *
 * @param checkName - Name of health check (e.g., "Token Health")
 * @param force - If true, bypass cache and return null
 * @returns Cached result if valid, null if expired or not found
 *
 * @example
 * const cached = getCached('Token Health', false);
 * if (cached) {
 *   return cached; // Use cached result
 * }
 * // Run fresh check
 */
export function getCached(checkName: string, force: boolean = false): HealthCheckResult | null {
  if (force) {
    return null;
  }

  const entry = cache.get(checkName);
  if (!entry) {
    return null;
  }

  const age = Date.now() - entry.timestamp;
  if (age > TTL) {
    cache.delete(checkName);
    return null;
  }

  return entry.result;
}

/**
 * Store health check result in cache
 *
 * @param checkName - Name of health check
 * @param result - Health check result to cache
 *
 * @example
 * const result = await checkTokenHealth();
 * setCached('Token Health', result);
 */
export function setCached(checkName: string, result: HealthCheckResult): void {
  cache.set(checkName, {
    result,
    timestamp: Date.now(),
  });
}

/**
 * Clear all cached results (for testing)
 */
export function clearCache(): void {
  cache.clear();
}
